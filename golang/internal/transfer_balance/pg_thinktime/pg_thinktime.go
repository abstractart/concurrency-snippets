// Package pg_thinktime показывает сценарий, в котором оптимистичная блокировка
// не просто быстрее — она единственный работающий вариант.
//
// Вводная: между чтением и записью проходит «время раздумий» — пользователь
// смотрит на форму, внешний сервис отвечает, воркер считает. Это время мы
// не контролируем, и оно на порядки больше самой работы с БД.
//
//	GET  /accounts/42/edit   → читаем баланс, отдаём форму
//	                         ... 30 секунд пользователь думает ...
//	POST /accounts/42        → записываем
//
// Пессимистичная стратегия требует держать транзакцию открытой всё это время:
// критическая секция начинается на чтении и заканчивается на записи. А открытая
// транзакция — это занятое соединение из пула. N думающих пользователей
// занимают N соединений, ничего при этом не делая.
//
// Оптимистичная стратегия разрывает эту связь: фаза чтения и фаза записи —
// две независимые короткие операции, между ними не удерживается ничего.
// Согласованность обеспечивает не блокировка, а номер версии, который
// пережидает раздумья на стороне клиента (скрытое поле формы, ETag, поле в очереди).
//
// Схема та же, что в pg_optimistic / pg_pessimistic:
//
//	CREATE TABLE accounts (
//	    id      BIGSERIAL PRIMARY KEY,
//	    name    TEXT NOT NULL,
//	    balance BIGINT NOT NULL CHECK (balance >= 0),
//	    version BIGINT NOT NULL DEFAULT 0
//	);
package pg_thinktime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrConflict — за время раздумий кто-то изменил один из счетов.
	// Прочитанные данные устарели, решение принято на основе неактуального состояния.
	ErrConflict = errors.New("conflict: account was modified concurrently")
)

// ---------------------------------------------------------------------------
// Оптимистичная стратегия: две независимые фазы
// ---------------------------------------------------------------------------

// Intent — намерение выполнить перевод, зафиксированное на фазе чтения.
// Переживает раздумья на стороне клиента: сериализуется в скрытое поле формы,
// в ETag, в сообщение очереди. Соединение с БД при этом не удерживается.
type Intent struct {
	FromID      int64
	ToID        int64
	Amount      int64
	FromVersion int64
	ToVersion   int64
}

// Prepare — фаза 1: читаем состояние и версии, сразу отпускаем соединение.
func Prepare(ctx context.Context, db *pgxpool.Pool, fromID, toID, amount int64) (Intent, error) {
	var intent Intent

	rows, err := db.Query(ctx,
		"SELECT id, balance, version FROM accounts WHERE id = ANY($1)",
		[]int64{fromID, toID})
	if err != nil {
		return intent, err
	}

	type state struct{ balance, version int64 }
	found := make(map[int64]state, 2)
	for rows.Next() {
		var id int64
		var s state
		if err := rows.Scan(&id, &s.balance, &s.version); err != nil {
			rows.Close()
			return intent, err
		}
		found[id] = s
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return intent, err
	}

	from, ok := found[fromID]
	if !ok {
		return intent, fmt.Errorf("account %d not found", fromID)
	}
	to, ok := found[toID]
	if !ok {
		return intent, fmt.Errorf("account %d not found", toID)
	}
	if from.balance < amount {
		return intent, ErrInsufficientFunds
	}

	return Intent{
		FromID:      fromID,
		ToID:        toID,
		Amount:      amount,
		FromVersion: from.version,
		ToVersion:   to.version,
	}, nil
}

// Commit — фаза 2: применяем намерение, если версии не изменились.
// Возвращает ErrConflict, если за время раздумий состояние ушло вперёд.
func Commit(ctx context.Context, db *pgxpool.Pool, intent Intent) error {
	// Порядок по id обязателен: UPDATE держит блокировку строки до конца
	// транзакции, поэтому встречные переводы без общего порядка дают дедлок
	// (40P01). Отсутствие явного SELECT FOR UPDATE от этого не спасает.
	updates := [2]struct {
		id      int64
		delta   int64
		version int64
	}{
		{id: intent.FromID, delta: -intent.Amount, version: intent.FromVersion},
		{id: intent.ToID, delta: +intent.Amount, version: intent.ToVersion},
	}
	if updates[0].id > updates[1].id {
		updates[0], updates[1] = updates[1], updates[0]
	}

	// Уровень изоляции фиксируем явно: на Repeatable Read и Serializable
	// конкурирующее обновление даёт 40001 вместо RowsAffected()==0.
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, u := range updates {
		// Проверка версии — аналог CAS. Если версия уехала, RowsAffected == 0.
		tag, err := tx.Exec(ctx, `
			UPDATE accounts
			SET balance = balance + $1, version = version + 1
			WHERE id = $2 AND version = $3`,
			u.delta, u.id, u.version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrConflict
		}
	}

	return tx.Commit(ctx)
}

// TransferOptimistic — полный цикл: чтение, раздумья, запись.
// think вызывается МЕЖДУ фазами, когда соединение уже возвращено в пул.
func TransferOptimistic(
	ctx context.Context, db *pgxpool.Pool,
	fromID, toID, amount int64,
	think func(),
) error {
	intent, err := Prepare(ctx, db, fromID, toID, amount)
	if err != nil {
		return err
	}

	// Соединение свободно: пул обслуживает других, пока этот клиент думает.
	if think != nil {
		think()
	}

	return Commit(ctx, db, intent)
}

// ---------------------------------------------------------------------------
// Пессимистичная стратегия: одна неразрывная транзакция
// ---------------------------------------------------------------------------

// TransferPessimistic — тот же сценарий через SELECT FOR UPDATE.
//
// Разделить на фазы невозможно: блокировка живёт только внутри транзакции,
// поэтому think вызывается ВНУТРИ неё. Всё время раздумий удерживаются
// и блокировки строк, и соединение из пула.
func TransferPessimistic(
	ctx context.Context, db *pgxpool.Pool,
	fromID, toID, amount int64,
	think func(),
) error {
	return pgx.BeginTxFunc(ctx, db, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		// Порядок по id обязателен — иначе встречные переводы дают deadlock.
		rows, err := tx.Query(ctx, `
			SELECT id, balance FROM accounts
			WHERE id = ANY($1)
			ORDER BY id
			FOR UPDATE`, []int64{fromID, toID})
		if err != nil {
			return err
		}
		balances := make(map[int64]int64, 2)
		for rows.Next() {
			var id, bal int64
			if err := rows.Scan(&id, &bal); err != nil {
				rows.Close()
				return err
			}
			balances[id] = bal
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if balances[fromID] < amount {
			return ErrInsufficientFunds
		}

		// Раздумья внутри транзакции: строки заблокированы, соединение занято.
		if think != nil {
			think()
		}

		if _, err := tx.Exec(ctx,
			"UPDATE accounts SET balance = balance - $1, version = version + 1 WHERE id = $2",
			amount, fromID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			"UPDATE accounts SET balance = balance + $1, version = version + 1 WHERE id = $2",
			amount, toID)
		return err
	})
}

// ---------------------------------------------------------------------------
// Вспомогательное
// ---------------------------------------------------------------------------

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

func CreateAccount(ctx context.Context, db *pgxpool.Pool, name string, initialBalance int64) (int64, error) {
	var id int64
	err := db.QueryRow(ctx,
		"INSERT INTO accounts (name, balance) VALUES ($1, $2) RETURNING id",
		name, initialBalance).Scan(&id)
	return id, err
}

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

	alice, err := CreateAccount(ctx, db, "alice", 100_000)
	if err != nil {
		return err
	}
	bob, err := CreateAccount(ctx, db, "bob", 100_000)
	if err != nil {
		return err
	}

	const thinkTime = 200 * time.Millisecond
	think := func() { time.Sleep(thinkTime) }

	fmt.Printf("pool max conns: %d, think time: %v\n\n", db.Config().MaxConns, thinkTime)

	// Демонстрация обнаружения конфликта: два клиента читают одно состояние,
	// думают, затем пытаются записать. Второй получает ErrConflict.
	first, err := Prepare(ctx, db, alice, bob, 100)
	if err != nil {
		return err
	}
	second, err := Prepare(ctx, db, alice, bob, 100)
	if err != nil {
		return err
	}
	think()

	fmt.Printf("first  commit: %v\n", Commit(ctx, db, first))
	fmt.Printf("second commit: %v  ← устаревшая версия отвергнута\n", Commit(ctx, db, second))
	return nil
}
