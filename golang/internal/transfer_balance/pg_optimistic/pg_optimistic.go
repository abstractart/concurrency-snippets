// Package pg_optimistic демонстрирует перевод баланса через оптимистичную блокировку.
//
// Паттерн: читаем без блокировок, пишем с проверкой версии (аналог CAS).
// Если версия изменилась пока мы думали — кто-то нас опередил, повторяем.
//
// Аналогия с MCAS: вместо atomic.CompareAndSwap — UPDATE WHERE version = $expected.
// PostgreSQL атомарно проверяет и обновляет; если строк затронуто 0 — конфликт.
//
// Преимущество перед пессимистичной: не нужно держать транзакцию открытой
// между чтением и записью (см. пакет pg_thinktime).
//
// Недостаток: при высокой конкуренции много повторных попыток.
//
// ВАЖНО, вопреки расхожему мнению: дедлоки здесь ВОЗМОЖНЫ. UPDATE удерживает
// блокировку строки до конца транзакции, поэтому перевод, затрагивающий две
// строки, обязан обновлять их в общем порядке (по id) — иначе встречные
// переводы дают 40P01, ровно как в пессимистичной версии. «Оптимистичная
// блокировка избавляет от дедлоков» справедливо только для однострочных операций.
//
// Реализация рассчитана на Read Committed и фиксирует этот уровень явно.
// На Repeatable Read и Serializable конкурирующее обновление возвращает
// ошибку 40001, а не RowsAffected()==0, и цикл повторов её не обрабатывает.
//
// Схема:
//
//	CREATE TABLE accounts (
//	    id      BIGSERIAL PRIMARY KEY,
//	    name    TEXT NOT NULL,
//	    balance BIGINT NOT NULL CHECK (balance >= 0),
//	    version BIGINT NOT NULL DEFAULT 0
//	);
package pg_optimistic

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrTooManyConflicts — перевод не удалось провести за maxAttempts попыток.
	// Отдельная ошибка нужна, чтобы вызывающий отличал перегрузку от отказа
	// по существу и мог ответить 503, а не 400.
	ErrTooManyConflicts = errors.New("too many conflicts")
)

// Параметры повторов.
//
// Без задержки цикл превращается в busy-wait: каждая итерация — это два SELECT
// и транзакция с двумя UPDATE, то есть ~4 обращения к СУБД. Чем выше нагрузка,
// тем больше конфликтов, тем больше повторов, тем выше нагрузка — положительная
// обратная связь. Задержка разрывает её, а джиттер разводит участников,
// которые иначе просыпались бы синхронно и конфликтовали снова.
//
// Потолок попыток обязателен: без него отдельный перевод может голодать
// неограниченно, удерживая соединение из пула.
const (
	maxAttempts    = 50
	backoffBase    = time.Millisecond
	backoffCeiling = 50 * time.Millisecond
)

// backoff — экспоненциальная задержка с полным джиттером: спим случайное
// время из [0, предел). Полный джиттер, а не «половина фиксированная,
// половина случайная», потому что он сильнее всего размазывает момент
// пробуждения конкурентов.
func backoff(ctx context.Context, attempt int) error {
	limit := backoffBase << min(attempt, 16)
	if limit > backoffCeiling {
		limit = backoffCeiling
	}

	timer := time.NewTimer(rand.N(limit))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type accountSnapshot struct {
	balance int64
	version int64
}

func readAccount(ctx context.Context, db *pgxpool.Pool, id int64) (accountSnapshot, error) {
	var s accountSnapshot
	err := db.QueryRow(ctx,
		"SELECT balance, version FROM accounts WHERE id = $1", id).
		Scan(&s.balance, &s.version)
	return s, err
}

// Transfer переводит amount со счёта fromID на toID.
// Читает без блокировок, обновляет с проверкой версии, при конфликте
// повторяет с экспоненциальной задержкой — не более maxAttempts раз.
func Transfer(ctx context.Context, db *pgxpool.Pool, fromID, toID int64, amount int64) error {
	for attempt := range maxAttempts {
		if attempt > 0 {
			if err := backoff(ctx, attempt-1); err != nil {
				return err
			}
		}

		from, err := readAccount(ctx, db, fromID)
		if err != nil {
			return err
		}
		if from.balance < amount {
			return ErrInsufficientFunds
		}
		to, err := readAccount(ctx, db, toID)
		if err != nil {
			return err
		}

		// Атомарно обновляем оба счёта в транзакции с проверкой версий.
		// UPDATE WHERE version = $expected — аналог CAS: если версия изменилась,
		// affected rows == 0, значит конфликт — начинаем сначала.
		committed, err := tryCommit(ctx, db, fromID, from, toID, to, amount)
		if err != nil {
			return err
		}
		if committed {
			return nil
		}
		// Конфликт: кто-то изменил один из счётов — читаем снова
	}
	return fmt.Errorf("%w: gave up after %d attempts", ErrTooManyConflicts, maxAttempts)
}

// versionedUpdate — одно обновление счёта с проверкой версии.
type versionedUpdate struct {
	id      int64
	delta   int64
	version int64
}

func tryCommit(
	ctx context.Context, db *pgxpool.Pool,
	fromID int64, from accountSnapshot,
	toID int64, to accountSnapshot,
	amount int64,
) (bool, error) {
	// Порядок по id обязателен, хотя блокировок мы явно не берём.
	// UPDATE удерживает блокировку строки до конца транзакции, поэтому две
	// транзакции, обновляющие те же две строки в разном порядке, встают
	// в дедлок (40P01) — ровно как в пессимистичной версии.
	// «Оптимистичная блокировка избавляет от дедлоков» верно только для
	// однострочных операций; здесь строк две.
	updates := [2]versionedUpdate{
		{id: fromID, delta: -amount, version: from.version},
		{id: toID, delta: +amount, version: to.version},
	}
	if updates[0].id > updates[1].id {
		updates[0], updates[1] = updates[1], updates[0]
	}

	// Уровень изоляции фиксируем явно: на Repeatable Read и Serializable
	// конкурирующее обновление даёт ошибку 40001 вместо RowsAffected()==0,
	// и цикл повторов выше её не обрабатывает.
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, u := range updates {
		// Проверка версии — аналог CAS: если версия уехала, RowsAffected()==0.
		tag, err := tx.Exec(ctx, `
			UPDATE accounts
			SET balance = balance + $1, version = version + 1
			WHERE id = $2 AND version = $3`,
			u.delta, u.id, u.version)
		if err != nil {
			return false, err
		}
		if tag.RowsAffected() == 0 {
			return false, nil // конфликт: состояние ушло вперёд
		}
	}

	return true, tx.Commit(ctx)
}

// SetupSchema создаёт таблицу accounts (идемпотентно).
func SetupSchema(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS accounts (
			id      BIGSERIAL PRIMARY KEY,
			name    TEXT NOT NULL,
			balance BIGINT NOT NULL CHECK (balance >= 0),
			version BIGINT NOT NULL DEFAULT 0
		)`)
	return err
}

// CreateAccount создаёт счёт и возвращает его ID.
func CreateAccount(ctx context.Context, db *pgxpool.Pool, name string, initialBalance int64) (int64, error) {
	var id int64
	err := db.QueryRow(ctx,
		"INSERT INTO accounts (name, balance) VALUES ($1, $2) RETURNING id",
		name, initialBalance).Scan(&id)
	return id, err
}

// TotalBalance возвращает сумму всех балансов — должна оставаться константой.
func TotalBalance(ctx context.Context, db *pgxpool.Pool) (int64, error) {
	var total int64
	err := db.QueryRow(ctx, "SELECT COALESCE(SUM(balance), 0) FROM accounts").Scan(&total)
	return total, err
}

func Run(ctx context.Context, db *pgxpool.Pool) error {
	if err := SetupSchema(ctx, db); err != nil {
		return fmt.Errorf("schema: %w", err)
	}

	if _, err := db.Exec(ctx, "DELETE FROM accounts"); err != nil {
		return err
	}

	alice, err := CreateAccount(ctx, db, "alice", 1000)
	if err != nil {
		return err
	}
	bob, err := CreateAccount(ctx, db, "bob", 1000)
	if err != nil {
		return err
	}

	totalBefore, _ := TotalBalance(ctx, db)
	fmt.Printf("before: total=%d\n", totalBefore)

	// Встречные переводы: сортировка по id внутри tryCommit защищает от дедлока
	done := make(chan error, 2)
	go func() { done <- Transfer(ctx, db, alice, bob, 300) }()
	go func() { done <- Transfer(ctx, db, bob, alice, 200) }()

	for range 2 {
		if err := <-done; err != nil {
			return fmt.Errorf("transfer: %w", err)
		}
	}

	totalAfter, _ := TotalBalance(ctx, db)
	fmt.Printf("after:  total=%d (conservation: %v)\n", totalAfter, totalBefore == totalAfter)
	return nil
}
