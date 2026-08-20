package main

import (
	"sync/atomic"
	"unsafe"
)

// Каждая ячейка счёта хранит либо реальный баланс (tx==nil),
// либо маркер захваченной транзакции (tx!=nil).
type balance struct {
	amount int64
	tx     *tx
}

type MCASAccount struct {
	balance atomic.Pointer[balance]
}

// tx — дескриптор атомарной операции над двумя счетами.
// Публикуется в обе ячейки до того, как значения изменятся.
type tx struct {
	done     atomic.Int32 // 0=pending, 1=ok, 2=fail
	firstOp  operation
	secondOp operation
}

type operation struct {
	acc    *MCASAccount
	before *balance // что было
	after  *balance // маркер: новый баланс + указатель на tx
}

func newMCASAccount(amount int64) *MCASAccount {
	a := &MCASAccount{}
	a.balance.Store(&balance{amount: amount})
	return a
}

func readMCASBalance(a *MCASAccount) int64 { return readBalance(a).amount }

// readBalance возвращает реальное значение ячейки,
// помогая завершить чужую транзакцию если ячейка захвачена.
func readBalance(a *MCASAccount) *balance {
	for {
		b := a.balance.Load()
		if b.tx == nil {
			return b
		}
		commit(b.tx)
	}
}

// commit пытается завершить транзакцию (может вызываться несколькими горутинами).
func commit(t *tx) {
	if t.done.Load() == 0 {
		var status int32 = 1

		if ok := prepare(&t.firstOp) && prepare(&t.secondOp); !ok {
			status = 2
		}

		t.done.CompareAndSwap(0, status)
	}

	if t.done.Load() == 1 {
		firstOpUpdatedBalance := &balance{amount: t.firstOp.after.amount}
		t.firstOp.acc.balance.CompareAndSwap(t.firstOp.after, firstOpUpdatedBalance)

		secondOpUpdatedBalance := &balance{amount: t.secondOp.after.amount}
		t.secondOp.acc.balance.CompareAndSwap(t.secondOp.after, secondOpUpdatedBalance)
	} else {
		t.firstOp.acc.balance.CompareAndSwap(t.firstOp.after, t.firstOp.before)
		t.secondOp.acc.balance.CompareAndSwap(t.secondOp.after, t.secondOp.before)
	}
}

// prepare захватывает ячейку для транзакции op.after.tx.
// Если ячейка занята другой транзакцией — помогает ей завершиться и повторяет.
func prepare(op *operation) bool {
	t := op.after.tx
	for {
		if s := t.done.Load(); s != 0 {
			return s == 1
		}
		cur := op.acc.balance.Load()
		if cur == op.after {
			return true // уже захвачено
		}
		if cur.tx != nil {
			commit(cur.tx) // помогаем завершить чужую транзакцию перед prepare
			continue
		}
		if cur != op.before {
			return false // кто-то изменил значение — нужен retry
		}
		op.acc.balance.CompareAndSwap(op.before, op.after)
	}
}

func transferMCAS(from, to *MCASAccount, amount int64) {
	for {
		fromBalance, toBalance := readBalance(from), readBalance(to)

		t := &tx{}
		afterFromBalance := &balance{amount: fromBalance.amount - amount, tx: t}
		afterToBalance := &balance{amount: toBalance.amount + amount, tx: t}

		t.firstOp = operation{
			acc:    from,
			before: fromBalance,
			after:  afterFromBalance,
		}
		t.secondOp = operation{
			acc:    to,
			before: toBalance,
			after:  afterToBalance,
		}

		if uintptr(unsafe.Pointer(from)) > uintptr(unsafe.Pointer(to)) {
			t.firstOp, t.secondOp = t.secondOp, t.firstOp
		}

		commit(t)
		if t.done.Load() == 1 {
			return
		}
	}
}
