package pg_optimistic_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/concurrency-examples/golang/internal/transfer_balance/pg_optimistic"
)

// TestNoDeadlockOnOpposingTransfers — регрессия на реальный баг.
//
// Оптимистичная блокировка не берёт явных блокировок, но UPDATE удерживает
// блокировку строки до конца транзакции. Пока перевод обновлял счета в порядке
// «сначала отправитель, потом получатель», встречные переводы A→B и B→A
// захватывали те же две строки в противоположном порядке и вставали в дедлок:
// PostgreSQL выбирал жертву и возвращал 40P01.
//
// Баг долго не замечался, потому что тесты выбрасывали ошибку через `_ =`:
// сохранность денег не нарушалась (жертва откатывалась целиком), просто часть
// переводов молча не выполнялась. А ждать детектор дедлоков по deadlock_timeout
// (1 секунда по умолчанию на каждую жертву) выглядело как «оптимистичная
// блокировка медленная под конкуренцией».
//
// Лечится сортировкой обновлений по id — тем же приёмом, что в pg_pessimistic
// и в сортировке операций по адресу в mcas.
func TestNoDeadlockOnOpposingTransfers(t *testing.T) {
	ctx := context.Background()

	if _, err := db.Exec(ctx, "DELETE FROM accounts"); err != nil {
		t.Fatal(err)
	}
	alice, err := pg_optimistic.CreateAccount(ctx, db, "alice", 100_000)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := pg_optimistic.CreateAccount(ctx, db, "bob", 100_000)
	if err != nil {
		t.Fatal(err)
	}

	const pairs = 25
	var (
		wg        sync.WaitGroup
		deadlocks atomic.Int64
		serFails  atomic.Int64
	)
	classify := func(err error) {
		if err == nil {
			return
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Errorf("неожиданная ошибка: %v", err)
			return
		}
		switch pgErr.Code {
		case "40P01":
			deadlocks.Add(1)
		case "40001":
			serFails.Add(1)
		default:
			t.Errorf("неожиданная ошибка СУБД %s: %v", pgErr.Code, err)
		}
	}

	wg.Add(pairs * 2)
	for range pairs {
		go func() {
			defer wg.Done()
			classify(pg_optimistic.Transfer(ctx, db, alice, bob, 10))
		}()
		go func() {
			defer wg.Done()
			classify(pg_optimistic.Transfer(ctx, db, bob, alice, 10))
		}()
	}
	wg.Wait()

	if got := deadlocks.Load(); got != 0 {
		t.Errorf("дедлоков (40P01): %d, ожидалось 0 — обновления не упорядочены по id", got)
	}
	// Уровень изоляции зафиксирован как Read Committed, поэтому конкурирующее
	// обновление обязано приводить к RowsAffected()==0 и повтору,
	// а не к ошибке сериализации.
	if got := serFails.Load(); got != 0 {
		t.Errorf("ошибок сериализации (40001): %d, ожидалось 0 на Read Committed", got)
	}

	total, err := pg_optimistic.TotalBalance(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(200_000); total != want {
		t.Errorf("сумма балансов = %d, ожидалась %d", total, want)
	}
}
