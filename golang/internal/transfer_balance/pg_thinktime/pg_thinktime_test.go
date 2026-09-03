package pg_thinktime_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/concurrency-examples/golang/internal/transfer_balance/pg_thinktime"
)

// Параметры сценария: пул намеренно узкий, раздумья намеренно долгие —
// так соотношение «время в БД / время раздумий» приближено к реальному вебу.
const (
	maxConns  = 10
	users     = 50
	thinkTime = 200 * time.Millisecond
)

var connStr string

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

	connStr, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("connection string: " + err.Error())
	}

	db, err := newPool(ctx)
	if err != nil {
		panic("connect: " + err.Error())
	}
	if err := pg_thinktime.SetupSchema(ctx, db); err != nil {
		panic("setup schema: " + err.Error())
	}
	db.Close()

	m.Run()
}

// newPool создаёт пул с фиксированным числом соединений — одинаковым
// для обеих стратегий, чтобы сравнение было честным.
func newPool(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = maxConns
	return pgxpool.NewWithConfig(ctx, cfg)
}

// seedAccounts очищает таблицу и создаёт n счетов с одинаковым балансом.
func seedAccounts(t *testing.T, db *pgxpool.Pool, n int) []int64 {
	t.Helper()
	ctx := context.Background()

	if _, err := db.Exec(ctx, "DELETE FROM accounts"); err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, n)
	rows, err := db.Query(ctx, `
		INSERT INTO accounts (name, balance)
		SELECT 'account-' || i, 1000000
		FROM generate_series(1, $1) AS i
		RETURNING id`, n)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

// runConcurrentUsers запускает users горутин, каждая переводит между
// собственной парой счетов (конкуренции за строки нет — узкое место только пул).
func runConcurrentUsers(t *testing.T, db *pgxpool.Pool, ids []int64,
	transfer func(ctx context.Context, db *pgxpool.Pool, from, to, amount int64, think func()) error,
) time.Duration {
	t.Helper()
	ctx := context.Background()
	think := func() { time.Sleep(thinkTime) }

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(users)
	for i := range users {
		go func() {
			defer wg.Done()
			from, to := ids[i*2], ids[i*2+1]
			if err := transfer(ctx, db, from, to, 10, think); err != nil {
				t.Errorf("transfer: %v", err)
			}
		}()
	}
	wg.Wait()
	return time.Since(start)
}

// TestConnectionPoolUnderThinkTime — главный замер.
//
// У каждого пользователя своя пара счетов, поэтому за строки никто не борется.
// Единственный дефицитный ресурс — соединения в пуле. Пессимистичная стратегия
// держит соединение все 200ms раздумий, поэтому 50 пользователей проходят
// волнами по 10; оптимистичная отпускает соединение на время раздумий,
// и все 50 думают одновременно.
func TestConnectionPoolUnderThinkTime(t *testing.T) {
	ctx := context.Background()

	dbOpt, err := newPool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer dbOpt.Close()

	ids := seedAccounts(t, dbOpt, users*2)
	optimistic := runConcurrentUsers(t, dbOpt, ids, pg_thinktime.TransferOptimistic)

	dbPess, err := newPool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer dbPess.Close()

	ids = seedAccounts(t, dbPess, users*2)
	pessimistic := runConcurrentUsers(t, dbPess, ids, pg_thinktime.TransferPessimistic)

	// Нижняя граница для пессимистичной: пользователи проходят волнами
	// по maxConns штук, каждая волна длится не меньше thinkTime.
	waves := (users + maxConns - 1) / maxConns
	expectedFloor := time.Duration(waves) * thinkTime

	t.Logf("users=%d  maxConns=%d  thinkTime=%v", users, maxConns, thinkTime)
	t.Logf("optimistic:  %v  (соединение отпущено на время раздумий)", optimistic.Round(time.Millisecond))
	t.Logf("pessimistic: %v  (соединение занято всё время раздумий, ~%d волн)",
		pessimistic.Round(time.Millisecond), waves)
	t.Logf("speedup:     %.1fx", float64(pessimistic)/float64(optimistic))

	if pessimistic < expectedFloor {
		t.Errorf("пессимистичная прошла за %v — быстрее теоретического минимума %v; "+
			"похоже, соединения не удерживались как ожидалось", pessimistic, expectedFloor)
	}
	if optimistic >= expectedFloor {
		t.Errorf("оптимистичная заняла %v — не быстрее пессимистичного минимума %v; "+
			"преимущество не воспроизвелось", optimistic, expectedFloor)
	}
}

// TestStaleWriteRejected — корректность, а не скорость.
//
// Два клиента читают одно и то же состояние, оба «думают», затем пишут.
// Без проверки версии второй записал бы поверх первого (lost update).
func TestStaleWriteRejected(t *testing.T) {
	ctx := context.Background()

	db, err := newPool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := seedAccounts(t, db, 2)
	from, to := ids[0], ids[1]

	totalBefore, err := pg_thinktime.TotalBalance(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	// Обе фазы чтения происходят до любой записи — клиенты видят одну версию.
	first, err := pg_thinktime.Prepare(ctx, db, from, to, 100)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pg_thinktime.Prepare(ctx, db, from, to, 100)
	if err != nil {
		t.Fatal(err)
	}

	if err := pg_thinktime.Commit(ctx, db, first); err != nil {
		t.Fatalf("первый коммит должен пройти: %v", err)
	}
	if err := pg_thinktime.Commit(ctx, db, second); !errors.Is(err, pg_thinktime.ErrConflict) {
		t.Fatalf("второй коммит должен вернуть ErrConflict, получено: %v", err)
	}

	// Применился ровно один перевод — второй отвергнут целиком.
	var balance int64
	if err := db.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = $1", from).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if want := int64(1000000 - 100); balance != want {
		t.Errorf("баланс отправителя = %d, ожидался %d (примениться должен ровно один перевод)", balance, want)
	}

	totalAfter, err := pg_thinktime.TotalBalance(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if totalBefore != totalAfter {
		t.Errorf("conservation violated: before=%d after=%d", totalBefore, totalAfter)
	}
}
