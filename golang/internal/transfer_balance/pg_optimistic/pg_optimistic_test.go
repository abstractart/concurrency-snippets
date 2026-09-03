package pg_optimistic_test

import (
	"context"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/concurrency-examples/golang/internal/transfer_balance/pg_optimistic"
)

// db — пул соединений, инициализированный один раз в TestMain для всего пакета.
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

	if err := pg_optimistic.SetupSchema(ctx, db); err != nil {
		panic("setup schema: " + err.Error())
	}

	m.Run()
}

func TestTransferConservation(t *testing.T) {
	ctx := context.Background()

	if _, err := db.Exec(ctx, "DELETE FROM accounts"); err != nil {
		t.Fatal(err)
	}

	alice, _ := pg_optimistic.CreateAccount(ctx, db, "alice", 1000)
	bob, _ := pg_optimistic.CreateAccount(ctx, db, "bob", 1000)

	totalBefore, _ := pg_optimistic.TotalBalance(ctx, db)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for range goroutines {
		go func() {
			defer wg.Done()
			if err := pg_optimistic.Transfer(ctx, db, alice, bob, 10); err != nil {
				t.Errorf("transfer alice→bob: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := pg_optimistic.Transfer(ctx, db, bob, alice, 10); err != nil {
				t.Errorf("transfer bob→alice: %v", err)
			}
		}()
	}
	wg.Wait()

	totalAfter, _ := pg_optimistic.TotalBalance(ctx, db)
	if totalBefore != totalAfter {
		t.Errorf("conservation violated: before=%d after=%d", totalBefore, totalAfter)
	}
}

// TestTransferManyAccounts — низкая конкуренция: 100 аккаунтов, случайные пары.
// Конфликты редки → почти нет повторных попыток, блокировки не нужны.
func TestTransferManyAccounts(t *testing.T) {
	ctx := context.Background()

	if _, err := db.Exec(ctx, "DELETE FROM accounts"); err != nil {
		t.Fatal(err)
	}

	const numAccounts = 10_000
	ids := make([]int64, numAccounts)
	rows, err := db.Query(ctx, `
		INSERT INTO accounts (name, balance)
		SELECT 'account-' || i, 10000
		FROM generate_series(1, $1) AS i
		RETURNING id`, numAccounts)
	if err != nil {
		t.Fatal(err)
	}
	i := 0
	for rows.Next() {
		if err := rows.Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
		i++
	}
	rows.Close()

	totalBefore, _ := pg_optimistic.TotalBalance(ctx, db)

	const goroutines = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			from := ids[rand.IntN(numAccounts)]
			to := ids[rand.IntN(numAccounts)]
			if from == to {
				return
			}
			if err := pg_optimistic.Transfer(ctx, db, from, to, 1); err != nil {
				t.Errorf("transfer: %v", err)
			}
		}()
	}
	wg.Wait()

	totalAfter, _ := pg_optimistic.TotalBalance(ctx, db)
	if totalBefore != totalAfter {
		t.Errorf("conservation violated: before=%d after=%d", totalBefore, totalAfter)
	}
}
