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

		if uintptr(unsafe.Pointer(from)) > uintptr(unsafe.Pointer(to)) {
			t.firstOp, t.secondOp = t.secondOp, t.firstOp
		}

		if t.commit(); t.succeeded() {
			return
		}
	}
}
