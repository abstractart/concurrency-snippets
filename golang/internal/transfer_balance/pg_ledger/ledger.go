package pg_ledger

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrCurrencyMismatch  = errors.New("currency mismatch")
	ErrAccountNotFound   = errors.New("account not found")
	ErrInvalidAmount     = errors.New("amount must be positive")
	ErrSameAccount       = errors.New("source and destination must differ")
)

// TransferRequest — запрос на перевод.
//
// IdempotencyKey задаёт клиент и повторяет при ретрае: по содержимому
// «повтори тот же перевод» и «сделай ещё один такой же» неразличимы.
type TransferRequest struct {
	IdempotencyKey string
	FromID         int64
	ToID           int64
	Amount         int64 // минорные единицы, строго > 0
	Currency       string
	Description    string
}

// TransferResult — итог перевода. Replayed означает, что проводка с таким
// ключом уже существовала и ничего не изменилось.
type TransferResult struct {
	TransactionID int64
	Replayed      bool
}

// entry — одна запись проводки. Отрицательная сумма — списание.
type entry struct {
	accountID int64
	amount    int64
}

// Transfer проводит перевод как проводку двойной записи.
func Transfer(ctx context.Context, db *pgxpool.Pool, req TransferRequest) (TransferResult, error) {
	if req.Amount <= 0 {
		return TransferResult{}, ErrInvalidAmount
	}
	if req.FromID == req.ToID {
		return TransferResult{}, ErrSameAccount
	}
	return post(ctx, db, req.IdempotencyKey, req.Description, req.Currency, []entry{
		{accountID: req.FromID, amount: -req.Amount},
		{accountID: req.ToID, amount: +req.Amount},
	})
}

// Reverse сторнирует проводку, добавляя обратную. Исходная остаётся
// в журнале: в бухгалтерии отменяют компенсацией, а не удалением.
func Reverse(ctx context.Context, db *pgxpool.Pool, idempotencyKey string, transactionID int64) (TransferResult, error) {
	rows, err := db.Query(ctx, `
		SELECT account_id, -amount, currency
		FROM ledger_entries WHERE transaction_id = $1 ORDER BY id`, transactionID)
	if err != nil {
		return TransferResult{}, err
	}
	defer rows.Close()

	var (
		entries  []entry
		currency string
	)
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.accountID, &e.amount, &currency); err != nil {
			return TransferResult{}, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return TransferResult{}, err
	}
	if len(entries) == 0 {
		return TransferResult{}, fmt.Errorf("transaction %d not found", transactionID)
	}

	return post(ctx, db, idempotencyKey,
		fmt.Sprintf("reversal of transaction %d", transactionID), currency, entries)
}

// idempotencyConstraint — имя UNIQUE-ограничения на ключе идемпотентности.
// По нему отличается гонка двух запросов с одним ключом от любого другого
// нарушения уникальности (например, номера операции в ledger_entries).
const idempotencyConstraint = "ledger_transactions_idempotency_key"

// post записывает сбалансированный набор записей одной проводкой.
//
// Повтор с уже проведённым ключом — no-op. Обрабатывается в три шага,
// каждый из которых опирается только на документированное поведение:
//
//  1. поиск проводки по ключу — дешёвый путь для ретраев, их большинство;
//  2. вставка и применение в одной транзакции;
//  3. если параллельный запрос с тем же ключом успел раньше, INSERT падает
//     с 23505 unique_violation, наша транзакция откатывается целиком,
//     и мы возвращаем результат победителя.
//
// Шаг 3 корректен и когда конкурент ещё в полёте: INSERT ждёт на уникальном
// индексе, пока тот не закоммитится (тогда 23505) или не откатится
// (тогда вставка проходит и работу делаем мы).
//
// Уровень изоляции зафиксирован: на Repeatable Read и Serializable
// конкурирующее обновление вернуло бы 40001, а цикла повторов здесь нет.
func post(
	ctx context.Context, db *pgxpool.Pool,
	idempotencyKey, description, currency string,
	entries []entry,
) (TransferResult, error) {
	if id, found, err := findTransaction(ctx, db, idempotencyKey); err != nil {
		return TransferResult{}, err
	} else if found {
		return TransferResult{TransactionID: id, Replayed: true}, nil
	}

	result, err := apply(ctx, db, idempotencyKey, description, currency, entries)
	if !isIdempotencyConflict(err) {
		return result, err
	}

	id, found, err := findTransaction(ctx, db, idempotencyKey)
	if err != nil {
		return TransferResult{}, err
	}
	if !found {
		return TransferResult{}, fmt.Errorf(
			"idempotency key %q conflicted but no transaction found", idempotencyKey)
	}
	return TransferResult{TransactionID: id, Replayed: true}, nil
}

func findTransaction(ctx context.Context, db *pgxpool.Pool, key string) (int64, bool, error) {
	var id int64
	err := db.QueryRow(ctx,
		"SELECT id FROM ledger_transactions WHERE idempotency_key = $1", key).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, err
	default:
		return id, true, nil
	}
}

func isIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" && // unique_violation
		pgErr.ConstraintName == idempotencyConstraint
}

// apply создаёт проводку и применяет её. Вся работа в одной транзакции,
// поэтому отказ на любом шаге не оставляет следов — включая заявку ключа.
func apply(
	ctx context.Context, db *pgxpool.Pool,
	idempotencyKey, description, currency string,
	entries []entry,
) (TransferResult, error) {
	var result TransferResult

	err := pgx.BeginTxFunc(ctx, db, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		// Сначала балансы, потом записи: номера записей выдаёт тот же UPDATE,
		// что берёт блокировку счёта. Проверка средств тоже там, поэтому
		// при отказе записи даже не создаются.
		txID, seqs, err := claimAndApply(ctx, tx, idempotencyKey, description, currency, entries)
		if err != nil {
			return err
		}
		result.TransactionID = txID
		return appendEntries(ctx, tx, txID, entries, currency, seqs)
	})
	if err != nil {
		return TransferResult{}, err
	}
	return result, nil
}

// claimAndApply создаёт проводку и применяет её к балансам ОДНИМ пакетом,
// возвращая id проводки и выданный каждому счёту номер операции.
//
// Пакет здесь не ради латентности, а ради пропускной способности. Блокировку
// строки счёта берёт первый же UPDATE и держит до COMMIT, поэтому каждое лишнее
// обращение к СУБД внутри транзакции продлевает удержание и напрямую снижает
// потолок операций по горячему счёту. Было три обращения под блокировкой,
// стало одно.
//
// Порядок по account_id сохраняется: запросы пакета выполняются последовательно
// в порядке очереди, значит и блокировки берутся в этом порядке. Без общего
// порядка встречные переводы дают deadlock — та же причина, по которой
// в mcas операции сортируются по адресу.
// applied — что случилось со счётом в этой проводке.
type applied struct {
	seq     int64 // выданный номер операции
	balance int64 // баланс счёта после неё
}

func claimAndApply(
	ctx context.Context, tx pgx.Tx,
	idempotencyKey, description, currency string, entries []entry,
) (int64, map[int64]applied, error) {
	ordered := slices.Clone(entries)
	slices.SortFunc(ordered, func(a, b entry) int { return cmp.Compare(a.accountID, b.accountID) })
	for i := 1; i < len(ordered); i++ {
		if ordered[i].accountID == ordered[i-1].accountID {
			return 0, nil, fmt.Errorf("account %d appears twice in one posting", ordered[i].accountID)
		}
	}

	batch := &pgx.Batch{}
	batch.Queue(`
		INSERT INTO ledger_transactions (idempotency_key, description)
		VALUES ($1, $2) RETURNING id`, idempotencyKey, description)
	for _, e := range ordered {
		// Один UPDATE под одной блокировкой строки делает три вещи:
		// проверяет средства, двигает баланс, выдаёт номер операции.
		//
		// Проверка средств встроена в условие, а не сделана отдельным SELECT:
		// между чтением и записью баланс может изменить кто угодно. СУБД
		// проверяет и применяет атомарно — это CAS, выраженный на SQL.
		// RETURNING отдаёт и номер операции, и новый баланс — оба нужны
		// записи журнала, и оба уже посчитаны этим же UPDATE.
		batch.Queue(`
			UPDATE ledger_accounts
			SET balance  = balance + $1,
			    last_seq = last_seq + 1
			WHERE id = $2
			  AND currency = $3
			  AND (allow_negative OR balance + $1 >= 0)
			RETURNING last_seq, balance`, e.amount, e.accountID, currency)
	}

	results := tx.SendBatch(ctx, batch)

	var txID int64
	err := results.QueryRow().Scan(&txID)

	applies := make(map[int64]applied, len(ordered))
	rejected := -1
	for i := range ordered {
		if err != nil {
			break
		}
		var a applied
		switch scanErr := results.QueryRow().Scan(&a.seq, &a.balance); {
		case errors.Is(scanErr, pgx.ErrNoRows):
			if rejected < 0 {
				rejected = i // счёт не прошёл условие
			}
		case scanErr != nil:
			err = scanErr
		default:
			applies[ordered[i].accountID] = a
		}
	}

	// Закрываем пакет до любых других запросов: пока он открыт,
	// выполнять что-либо ещё в этой транзакции нельзя.
	if closeErr := results.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, nil, err
	}
	if rejected >= 0 {
		return 0, nil, classifyUpdateFailure(ctx, tx, ordered[rejected], currency)
	}
	return txID, applies, nil
}

// classifyUpdateFailure выясняет, почему UPDATE не затронул строку.
// Лишний запрос на ошибочном пути не важен, зато вызывающий получает
// причину вместо «0 rows affected».
func classifyUpdateFailure(ctx context.Context, tx pgx.Tx, e entry, currency string) error {
	var (
		actualCurrency string
		balance        int64
	)
	err := tx.QueryRow(ctx,
		"SELECT currency, balance FROM ledger_accounts WHERE id = $1",
		e.accountID).Scan(&actualCurrency, &balance)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("%w: id=%d", ErrAccountNotFound, e.accountID)
	case err != nil:
		return err
	case actualCurrency != currency:
		return fmt.Errorf("%w: account %d holds %s, transfer is in %s",
			ErrCurrencyMismatch, e.accountID, actualCurrency, currency)
	default:
		return fmt.Errorf("%w: account %d has %d, needs %d",
			ErrInsufficientFunds, e.accountID, balance, -e.amount)
	}
}

// appendEntries дописывает записи в журнал. Только INSERT — никогда UPDATE.
func appendEntries(
	ctx context.Context, tx pgx.Tx,
	txID int64, entries []entry, currency string, applies map[int64]applied,
) error {
	batch := &pgx.Batch{}
	for _, e := range entries {
		a := applies[e.accountID]
		batch.Queue(`
			INSERT INTO ledger_entries
				(transaction_id, account_id, account_seq, amount, currency, balance_after)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			txID, e.accountID, a.seq, e.amount, currency, a.balance)
	}
	return tx.SendBatch(ctx, batch).Close()
}
