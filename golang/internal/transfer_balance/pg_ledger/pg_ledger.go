// Package pg_ledger хранит деньги журналом двойной записи, а не изменяемым полем.
//
// Остальные примеры (pg_pessimistic, pg_optimistic, pg_thinktime) решают задачу
// «есть поле balance, надо безопасно его изменить». Здесь такой задачи нет:
// хранится журнал неизменяемых записей, а баланс — это SUM(amount) по счёту.
// Поле balance остаётся лишь кэшем.
//
// Что меняется для конкурентности — главное в этом примере:
//
// Дописывание записей ни с кем не конфликтует. Синхронизация нужна ровно
// в одном месте — проверке достаточности средств, и она делается одним
// атомарным UPDATE с условием, без предварительного чтения:
//
//	UPDATE ... SET balance = balance + $delta WHERE id = $id AND balance + $delta >= 0
//
// Ни блокировок, ни версий, ни цикла повторов — CAS, выраженный на SQL.
// Порядок обновления счетов по id при этом всё равно обязателен: UPDATE
// держит блокировку строки, и встречные переводы иначе дают deadlock.
//
// Сохранение денег гарантирует схема: отложенный триггер не даст закоммитить
// проводку, записи которой не дают в сумме ноль.
//
// Чего здесь нет: авторизационные холды (pending/posted), мультивалютные
// проводки через FX-счета, партиционирование журнала, снапшоты балансов
// и повторы на 40001 для Repeatable Read / Serializable.
package pg_ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Run(ctx context.Context, db *pgxpool.Pool) error {
	if err := ResetSchema(ctx, db); err != nil {
		return fmt.Errorf("schema: %w", err)
	}

	system, err := CreateAccount(ctx, db, "system:issuance", "USD", true)
	if err != nil {
		return err
	}
	alice, err := CreateAccount(ctx, db, "alice", "USD", false)
	if err != nil {
		return err
	}
	bob, err := CreateAccount(ctx, db, "bob", "USD", false)
	if err != nil {
		return err
	}

	// Пополнение — перевод с системного счёта: деньги не берутся из воздуха,
	// поэтому сумма журнала остаётся нулевой.
	if _, err := Transfer(ctx, db, TransferRequest{
		IdempotencyKey: "mint-alice",
		FromID:         system, ToID: alice,
		Amount: 100_00, Currency: "USD", Description: "mint",
	}); err != nil {
		return err
	}

	payment := TransferRequest{
		IdempotencyKey: "transfer-1",
		FromID:         alice, ToID: bob,
		Amount: 30_00, Currency: "USD", Description: "payment for coffee",
	}
	transfer, err := Transfer(ctx, db, payment)
	if err != nil {
		return err
	}
	replay, err := Transfer(ctx, db, payment) // клиент не получил ответ и ретраит
	if err != nil {
		return err
	}
	fmt.Printf("перевод: tx=%d replayed=%v\n", transfer.TransactionID, transfer.Replayed)
	fmt.Printf("повтор:  tx=%d replayed=%v  ← списания не было\n\n", replay.TransactionID, replay.Replayed)

	_, err = Transfer(ctx, db, TransferRequest{
		IdempotencyKey: "overdraft",
		FromID:         bob, ToID: alice,
		Amount: 999_00, Currency: "USD", Description: "overdraft attempt",
	})
	fmt.Printf("овердрафт отвергнут: %v\n\n", errors.Is(err, ErrInsufficientFunds))

	if _, err := Reverse(ctx, db, "reverse-1", transfer.TransactionID); err != nil {
		return err
	}

	for _, acc := range []struct {
		label string
		id    int64
	}{{"alice", alice}, {"bob", bob}, {"system", system}} {
		if err := printAccount(ctx, db, acc.label, acc.id); err != nil {
			return err
		}
	}

	total, err := LedgerSum(ctx, db)
	if err != nil {
		return err
	}
	discrepancies, err := Reconcile(ctx, db)
	if err != nil {
		return err
	}
	fmt.Printf("сумма записей журнала: %d (всегда 0), расхождений кэша: %d\n",
		total, len(discrepancies))
	return nil
}

func printAccount(ctx context.Context, db *pgxpool.Pool, label string, id int64) error {
	cached, err := CachedBalance(ctx, db, id)
	if err != nil {
		return err
	}
	derived, err := DerivedBalance(ctx, db, id)
	if err != nil {
		return err
	}
	lines, err := Statement(ctx, db, id, 0, 100)
	if err != nil {
		return err
	}

	fmt.Printf("%s: кэш=%d журнал=%d\n", label, cached, derived)
	for _, l := range lines {
		fmt.Printf("    №%d %+8d → %8d  %s\n", l.Seq, l.Amount, l.Balance, l.Description)
	}
	fmt.Println()
	return nil
}
