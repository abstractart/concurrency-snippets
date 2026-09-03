// Package pg_pessimistic демонстрирует перевод баланса через пессимистичную блокировку.
//
// Паттерн: SELECT ... FOR UPDATE захватывает строки до завершения транзакции.
// Никто другой не может изменить заблокированные строки — конкурент ждёт.
//
// Проблема deadlock: если A блокирует счёт 1, B блокирует счёт 2, затем
// A хочет счёт 2, а B — счёт 1, оба застывают навсегда.
// Решение то же, что в MCAS: всегда блокировать в порядке возрастания ID.
//
// Схема:
//
//	CREATE TABLE accounts (
//	    id      BIGSERIAL PRIMARY KEY,
//	    name    TEXT NOT NULL,
//	    balance BIGINT NOT NULL CHECK (balance >= 0),
//	    version BIGINT NOT NULL DEFAULT 0
//	);
package pg_pessimistic

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInsufficientFunds = errors.New("insufficient funds")

// Transfer переводит amount со счёта fromID на toID.
// Использует SELECT FOR UPDATE — строки блокируются на время транзакции.
func Transfer(ctx context.Context, db *pgxpool.Pool, fromID, toID int64, amount int64) error {
	return pgx.BeginTxFunc(ctx, db, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		// Блокируем оба счёта в порядке возрастания ID — иначе возможен deadlock.
		// Та же логика, что при сортировке операций по адресу в MCAS.
		lockQuery := `
			SELECT id, balance FROM accounts
			WHERE id = ANY($1)
			ORDER BY id
			FOR UPDATE`

		rows, err := tx.Query(ctx, lockQuery, []int64{fromID, toID})
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

		if _, err := tx.Exec(ctx,
			"UPDATE accounts SET balance = balance - $1 WHERE id = $2",
			amount, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			"UPDATE accounts SET balance = balance + $1 WHERE id = $2",
			amount, toID); err != nil {
			return err
		}
		return nil
	})
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

	// Очищаем таблицу перед демонстрацией
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

	// Встречные переводы — без сортировки по ID был бы deadlock
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
