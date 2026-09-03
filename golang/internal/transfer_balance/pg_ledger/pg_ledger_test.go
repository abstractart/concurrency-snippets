package pg_ledger_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/concurrency-examples/golang/internal/transfer_balance/pg_ledger"
)

var db *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("concurrency"),
		postgres.WithUsername("concurrency"),
		postgres.WithPassword("concurrency"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		panic("start postgres container: " + err.Error())
	}
	defer container.Terminate(ctx) //nolint:errcheck

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("connection string: " + err.Error())
	}
	db, err = pgxpool.New(ctx, connStr)
	if err != nil {
		panic("connect: " + err.Error())
	}
	defer db.Close()

	m.Run()
}

// reset пересоздаёт схему и возвращает id системного счёта.
func reset(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	if err := pg_ledger.ResetSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	system, err := pg_ledger.CreateAccount(ctx, db, "system", "USD", true)
	if err != nil {
		t.Fatal(err)
	}
	return system
}

// fundedAccount создаёт счёт и пополняет его с системного.
func fundedAccount(t *testing.T, system int64, name string, amount int64) int64 {
	t.Helper()
	ctx := context.Background()
	id, err := pg_ledger.CreateAccount(ctx, db, name, "USD", false)
	if err != nil {
		t.Fatal(err)
	}
	if amount > 0 {
		if _, err := pg_ledger.Transfer(ctx, db, pg_ledger.TransferRequest{
			IdempotencyKey: "mint-" + name,
			FromID:         system, ToID: id,
			Amount: amount, Currency: "USD", Description: "mint",
		}); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func transfer(key string, from, to, amount int64) pg_ledger.TransferRequest {
	return pg_ledger.TransferRequest{
		IdempotencyKey: key,
		FromID:         from, ToID: to,
		Amount: amount, Currency: "USD", Description: key,
	}
}

// assertReconciled проверяет, что кэш каждого счёта сходится с журналом.
func assertReconciled(t *testing.T) {
	t.Helper()
	discrepancies, err := pg_ledger.Reconcile(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range discrepancies {
		t.Errorf("расхождение: %s", d)
	}
}

// TestLedgerSumsToZeroUnderConcurrency — главный инвариант двойной записи:
// сколько бы переводов ни прошло, сумма всех записей журнала равна нулю.
func TestLedgerSumsToZeroUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	system := reset(t)

	const numAccounts = 20
	ids := make([]int64, numAccounts)
	for i := range ids {
		ids[i] = fundedAccount(t, system, fmt.Sprintf("acc-%d", i), 10_000)
	}

	const goroutines = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			from, to := ids[rand.IntN(numAccounts)], ids[rand.IntN(numAccounts)]
			if from == to {
				return
			}
			_, err := pg_ledger.Transfer(ctx, db, transfer(fmt.Sprintf("tx-%d", i), from, to, 100))
			if err != nil && !errors.Is(err, pg_ledger.ErrInsufficientFunds) {
				t.Errorf("transfer: %v", err)
			}
		}()
	}
	wg.Wait()

	total, err := pg_ledger.LedgerSum(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("сумма журнала = %d, должна быть 0", total)
	}
	assertReconciled(t)
}

// TestNoOverdraftUnderConcurrency — проверка средств встроена в UPDATE,
// поэтому гонка «оба прочитали достаточный баланс» невозможна.
// 50 горутин списывают по 100 со счёта, где лежит 1000: пройдут ровно 10.
func TestNoOverdraftUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	alice := fundedAccount(t, system, "alice", 1_000)
	bob := fundedAccount(t, system, "bob", 0)

	const attempts = 50
	var (
		wg        sync.WaitGroup
		succeeded atomic.Int64
	)
	wg.Add(attempts)
	for i := range attempts {
		go func() {
			defer wg.Done()
			_, err := pg_ledger.Transfer(ctx, db, transfer(fmt.Sprintf("w-%d", i), alice, bob, 100))
			switch {
			case err == nil:
				succeeded.Add(1)
			case errors.Is(err, pg_ledger.ErrInsufficientFunds): // ожидаемо
			default:
				t.Errorf("transfer: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := succeeded.Load(); got != 10 {
		t.Errorf("успешных списаний: %d, ожидалось ровно 10", got)
	}
	balance, err := pg_ledger.CachedBalance(ctx, db, alice)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 0 {
		t.Errorf("баланс alice = %d, ожидался 0", balance)
	}
}

// TestIdempotentConcurrentRetry — повтор с тем же ключом не списывает дважды.
// Запросы идут конкурентно, как настоящие ретраи по таймауту.
func TestIdempotentConcurrentRetry(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	alice := fundedAccount(t, system, "alice", 1_000)
	bob := fundedAccount(t, system, "bob", 0)

	const retries = 30
	var (
		wg      sync.WaitGroup
		applied atomic.Int64
		txIDs   sync.Map
	)
	wg.Add(retries)
	for range retries {
		go func() {
			defer wg.Done()
			res, err := pg_ledger.Transfer(ctx, db, transfer("same-key", alice, bob, 500))
			if err != nil {
				t.Errorf("transfer: %v", err)
				return
			}
			txIDs.Store(res.TransactionID, struct{}{})
			if !res.Replayed {
				applied.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := applied.Load(); got != 1 {
		t.Errorf("применено проводок: %d, ожидалась 1", got)
	}
	var distinct int
	txIDs.Range(func(_, _ any) bool { distinct++; return true })
	if distinct != 1 {
		t.Errorf("разных transaction_id: %d, ожидался 1", distinct)
	}
	balance, err := pg_ledger.CachedBalance(ctx, db, alice)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 500 {
		t.Errorf("баланс alice = %d, ожидался 500 (одно списание из %d попыток)", balance, retries)
	}
}

// TestIdempotencyRaceResolvesToWinner — проверка пути через unique_violation.
//
// Быстрый SELECT перехватывает почти все повторы, поэтому ветку с 23505
// надо вводить намеренно: занимаем ключ в незавершённой транзакции,
// пока параллельный перевод уже прошёл проверку и упирается в уникальный
// индекс. После коммита конкурента он обязан вернуть его результат,
// а не списать деньги второй раз.
func TestIdempotencyRaceResolvesToWinner(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	alice := fundedAccount(t, system, "alice", 1_000)
	bob := fundedAccount(t, system, "bob", 0)

	winner, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer winner.Rollback(ctx) //nolint:errcheck

	var winnerID int64
	if err := winner.QueryRow(ctx, `
		INSERT INTO ledger_transactions (idempotency_key, description)
		VALUES ('raced', 'winner') RETURNING id`).Scan(&winnerID); err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		res pg_ledger.TransferResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := pg_ledger.Transfer(ctx, db, transfer("raced", alice, bob, 100))
		done <- outcome{res, err}
	}()

	// Перевод должен ждать на уникальном индексе, пока конкурент не решится.
	select {
	case got := <-done:
		t.Fatalf("перевод завершился до коммита конкурента: %+v", got)
	case <-time.After(200 * time.Millisecond):
	}

	if err := winner.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.err != nil {
		t.Fatalf("перевод должен был вернуть результат победителя: %v", got.err)
	}
	if !got.res.Replayed {
		t.Error("перевод не помечен как повтор, хотя ключ занял конкурент")
	}
	if got.res.TransactionID != winnerID {
		t.Errorf("вернулся tx=%d, ожидался tx=%d победителя", got.res.TransactionID, winnerID)
	}

	// Наша транзакция откатилась целиком: деньги не двигались.
	balance, err := pg_ledger.CachedBalance(ctx, db, alice)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 1_000 {
		t.Errorf("баланс alice = %d, ожидался 1000 — списания быть не должно", balance)
	}
}

// TestFailedTransferDoesNotConsumeKey — неуспешный перевод не «сжигает» ключ:
// транзакция откатывается целиком, включая заявку ключа.
func TestFailedTransferDoesNotConsumeKey(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	alice := fundedAccount(t, system, "alice", 100)
	bob := fundedAccount(t, system, "bob", 0)

	req := transfer("retry-after-failure", alice, bob, 500) // больше, чем есть
	if _, err := pg_ledger.Transfer(ctx, db, req); !errors.Is(err, pg_ledger.ErrInsufficientFunds) {
		t.Fatalf("ожидался ErrInsufficientFunds, получено: %v", err)
	}

	if _, err := pg_ledger.Transfer(ctx, db, transfer("top-up", system, alice, 1_000)); err != nil {
		t.Fatal(err)
	}
	res, err := pg_ledger.Transfer(ctx, db, req)
	if err != nil {
		t.Fatalf("после пополнения перевод должен пройти: %v", err)
	}
	if res.Replayed {
		t.Error("перевод помечен как повтор, хотя первая попытка не удалась")
	}
}

// TestSequenceContiguousUnderConcurrency — нумерация операций счёта сплошная
// (1, 2, 3, ...), last_seq равен их количеству, и выписка идёт в этом порядке.
//
// Номера выдаются под блокировкой строки счёта тем же UPDATE, что меняет
// баланс, поэтому конкуренция их не разрежает. По глобальному id из BIGSERIAL
// так не получилось бы: он выдаётся до взятия блокировки.
func TestSequenceContiguousUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	hot := fundedAccount(t, system, "hot", 100_000)
	other := fundedAccount(t, system, "other", 100_000)

	const goroutines = 60
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			from, to := hot, other
			if i%2 == 0 {
				from, to = other, hot
			}
			if _, err := pg_ledger.Transfer(ctx, db, transfer(fmt.Sprintf("c-%d", i), from, to, 10)); err != nil {
				t.Errorf("transfer: %v", err)
			}
		}()
	}
	wg.Wait()

	// Каждый перевод задевает оба счёта: 1 пополнение + goroutines переводов.
	const wantLines = 1 + goroutines
	for _, acc := range []struct {
		name string
		id   int64
	}{{"hot", hot}, {"other", other}} {
		lines, err := pg_ledger.Statement(ctx, db, acc.id, 0, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) != wantLines {
			t.Errorf("%s: строк выписки %d, ожидалось %d", acc.name, len(lines), wantLines)
			continue
		}
		for i, l := range lines {
			if want := int64(i + 1); l.Seq != want {
				t.Errorf("%s: строка %d имеет номер %d, ожидался %d", acc.name, i, l.Seq, want)
				break
			}
		}
		balance, err := pg_ledger.CachedBalance(ctx, db, acc.id)
		if err != nil {
			t.Fatal(err)
		}
		if last := lines[len(lines)-1].Balance; last != balance {
			t.Errorf("%s: итог выписки %d не совпал с балансом %d", acc.name, last, balance)
		}
	}
	assertReconciled(t)
}

// TestJournalChainIsSelfVerifying — balance_after образует цепочку контрольных
// точек: баланс из журнала читается одним обращением и совпадает с кэшем,
// а разрыв цепочки обнаруживается.
func TestJournalChainIsSelfVerifying(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	alice := fundedAccount(t, system, "alice", 10_000)
	bob := fundedAccount(t, system, "bob", 10_000)

	const goroutines = 40
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			from, to := alice, bob
			if i%2 == 0 {
				from, to = bob, alice
			}
			if _, err := pg_ledger.Transfer(ctx, db, transfer(fmt.Sprintf("chain-%d", i), from, to, 10)); err != nil {
				t.Errorf("transfer: %v", err)
			}
		}()
	}
	wg.Wait()

	for _, id := range []int64{alice, bob} {
		cached, err := pg_ledger.CachedBalance(ctx, db, id)
		if err != nil {
			t.Fatal(err)
		}
		derived, err := pg_ledger.DerivedBalance(ctx, db, id)
		if err != nil {
			t.Fatal(err)
		}
		if cached != derived {
			t.Errorf("account %d: кэш=%d, журнал=%d", id, cached, derived)
		}
		if err := pg_ledger.AuditChain(ctx, db, id); err != nil {
			t.Errorf("account %d: %v", id, err)
		}
	}

	// Ломаем цепочку в обход триггера — так выглядела бы порча извне.
	if _, err := db.Exec(ctx,
		"ALTER TABLE ledger_entries DISABLE TRIGGER ledger_entries_no_mutation"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE ledger_entries SET balance_after = balance_after + 1
		WHERE account_id = $1 AND account_seq = 2`, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx,
		"ALTER TABLE ledger_entries ENABLE TRIGGER ledger_entries_no_mutation"); err != nil {
		t.Fatal(err)
	}
	if err := pg_ledger.AuditChain(ctx, db, alice); err == nil {
		t.Error("разрыв цепочки не обнаружен")
	}
}

// TestStatementPaginates — страницы идут по ключу, итог берётся из журнала.
func TestStatementPaginates(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	alice := fundedAccount(t, system, "alice", 10_000)
	bob := fundedAccount(t, system, "bob", 0)

	for i := range 10 {
		if _, err := pg_ledger.Transfer(ctx, db, transfer(fmt.Sprintf("p-%d", i), alice, bob, 100)); err != nil {
			t.Fatal(err)
		}
	}

	var (
		seen   []pg_ledger.StatementLine
		cursor int64
	)
	for {
		page, err := pg_ledger.Statement(ctx, db, alice, cursor, 4)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		seen = append(seen, page...)
		cursor = page[len(page)-1].Seq
	}

	if len(seen) != 11 { // пополнение + 10 переводов
		t.Fatalf("строк выписки: %d, ожидалось 11", len(seen))
	}
	for i, l := range seen {
		if want := int64(i + 1); l.Seq != want {
			t.Errorf("строка %d: номер %d, ожидался %d", i, l.Seq, want)
		}
	}
	balance, err := pg_ledger.CachedBalance(ctx, db, alice)
	if err != nil {
		t.Fatal(err)
	}
	if last := seen[len(seen)-1].Balance; last != balance {
		t.Errorf("итог выписки %d не совпал с балансом %d", last, balance)
	}
}

// TestSchemaRejectsUnbalancedTransaction — деньги нельзя создать из воздуха
// даже в обход кода пакета: пишем в журнал напрямую одну запись без парной.
func TestSchemaRejectsUnbalancedTransaction(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	alice := fundedAccount(t, system, "alice", 0)

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var txID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO ledger_transactions (idempotency_key, description)
		VALUES ('forged', 'money from nothing') RETURNING id`).Scan(&txID); err != nil {
		t.Fatal(err)
	}
	// Вставка проходит: проверка отложена до COMMIT.
	// Номер и баланс берём правдоподобные, иначе тест упрётся в NOT NULL,
	// а не в проверку сбалансированности.
	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries
			(transaction_id, account_id, account_seq, amount, currency, balance_after)
		SELECT $1, a.id, a.last_seq + 1, 100, 'USD', a.balance + 100
		FROM ledger_accounts a WHERE a.id = $2`,
		txID, alice); err != nil {
		t.Fatalf("вставка должна пройти, проверка отложена до COMMIT: %v", err)
	}

	err = tx.Commit(ctx)
	if err == nil {
		t.Fatal("COMMIT несбалансированной проводки должен был упасть")
	}
	if !strings.Contains(err.Error(), "unbalanced transaction") {
		t.Errorf("ожидалась ошибка про unbalanced transaction, получено: %v", err)
	}
}

// TestJournalIsAppendOnly — журнал только дописывается.
func TestJournalIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	alice := fundedAccount(t, system, "alice", 1_000)

	for _, tc := range []struct{ name, sql string }{
		{"UPDATE", "UPDATE ledger_entries SET amount = 999999 WHERE account_id = $1"},
		{"DELETE", "DELETE FROM ledger_entries WHERE account_id = $1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.Exec(ctx, tc.sql, alice)
			if err == nil {
				t.Fatalf("%s по журналу должен быть запрещён", tc.name)
			}
			if !strings.Contains(err.Error(), "append-only") {
				t.Errorf("ожидалась ошибка про append-only, получено: %v", err)
			}
		})
	}
}

// TestReverseKeepsHistory — сторно возвращает баланс, но не стирает историю.
func TestReverseKeepsHistory(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	alice := fundedAccount(t, system, "alice", 1_000)
	bob := fundedAccount(t, system, "bob", 0)

	tx, err := pg_ledger.Transfer(ctx, db, transfer("mistake", alice, bob, 300))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pg_ledger.Reverse(ctx, db, "reverse-mistake", tx.TransactionID); err != nil {
		t.Fatal(err)
	}

	balance, err := pg_ledger.CachedBalance(ctx, db, alice)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 1_000 {
		t.Errorf("баланс alice = %d, ожидался 1000", balance)
	}
	lines, err := pg_ledger.Statement(ctx, db, alice, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Errorf("строк выписки: %d, ожидалось 3 (пополнение, ошибка, сторно)", len(lines))
	}
	assertReconciled(t)
}

// TestTransferRejections — отказы возвращают конкретную причину,
// а не безымянную ошибку.
func TestTransferRejections(t *testing.T) {
	ctx := context.Background()
	system := reset(t)
	alice := fundedAccount(t, system, "alice", 1_000)
	euro, err := pg_ledger.CreateAccount(ctx, db, "euro", "EUR", false)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		req  pg_ledger.TransferRequest
		want error
	}{
		{"валюта не совпадает", transfer("cross-currency", alice, euro, 100), pg_ledger.ErrCurrencyMismatch},
		{"недостаточно средств", transfer("too-much", alice, system, 999_999), pg_ledger.ErrInsufficientFunds},
		{"счёта нет", transfer("missing", alice, 999_999, 100), pg_ledger.ErrAccountNotFound},
		{"нулевая сумма", transfer("zero", alice, system, 0), pg_ledger.ErrInvalidAmount},
		{"перевод себе", transfer("self", alice, alice, 100), pg_ledger.ErrSameAccount},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pg_ledger.Transfer(ctx, db, tc.req); !errors.Is(err, tc.want) {
				t.Errorf("ожидалась %v, получено: %v", tc.want, err)
			}
		})
	}
	assertReconciled(t)
}
