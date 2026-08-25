package mcas

import (
	"sync/atomic"
	"unsafe"
)

type Account struct {
	balance atomic.Pointer[balance]
}

func NewAccount(amount int64) *Account {
	a := &Account{}
	a.balance.Store(&balance{amount: amount})
	return a
}

func (a *Account) Balance() int64 {
	return a.readBalance().amount
}

// readBalance возвращает реальное значение ячейки,
// помогая завершить чужую транзакцию если ячейка захвачена.
func (a *Account) readBalance() *balance {
	for {
		b := a.balance.Load()
		if !b.isInFlight() {
			return b
		}
		b.tx.commit()
	}
}

// Transfer — lock-free перевод через Multi-word CAS.
func (from *Account) Transfer(to *Account, amount int64) {
	for {
		t := &tx{}
		t.firstOp = newOperation(from, t, -amount)
		t.secondOp = newOperation(to, t, +amount)

		// Фиксируем порядок операций по адресу, чтобы граф cooperative helping
		// был ациклическим: без порядка A помогает B, B помогает A → бесконечная рекурсия.
		if uintptr(unsafe.Pointer(from)) > uintptr(unsafe.Pointer(to)) {
			t.firstOp, t.secondOp = t.secondOp, t.firstOp
		}

		if t.commit(); t.succeeded() {
			return
		}
	}
}

// TransferWithoutOrdering — то же что Transfer, но без сортировки операций по адресу.
// При встречных переводах (A→B и B→A одновременно) cooperative helping может войти
// в цикл: A помогает B, B помогает A, A помогает B... → бесконечная рекурсия.
// Используется только для демонстрации проблемы в тестах.
func (from *Account) TransferWithoutOrdering(to *Account, amount int64) {
	for {
		t := &tx{}
		t.firstOp = newOperation(from, t, -amount)
		t.secondOp = newOperation(to, t, +amount)
		// ordering swap намеренно отсутствует
		if t.commit(); t.succeeded() {
			return
		}
	}
}
