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

func (b *balance) isInFlight() bool { return b.tx != nil }

type MCASAccount struct {
	balance atomic.Pointer[balance]
}

type txStatus int32

const (
	txUnresolved txStatus = iota
	txSucceeded
	txFailed
)

// tx — дескриптор атомарной операции над двумя счетами.
// Публикуется в обе ячейки до того, как значения изменятся.
type tx struct {
	done     atomic.Int32
	firstOp  operation
	secondOp operation
}

func (t *tx) status() txStatus    { return txStatus(t.done.Load()) }
func (t *tx) resolved() bool      { return t.status() != txUnresolved }
func (t *tx) succeeded() bool     { return t.status() == txSucceeded }
func (t *tx) tryResolve(ok bool) {
	if ok {
		t.done.CompareAndSwap(int32(txUnresolved), int32(txSucceeded))
	} else {
		t.done.CompareAndSwap(int32(txUnresolved), int32(txFailed))
	}
}

type operation struct {
	acc    *MCASAccount
	before *balance // что было
	after  *balance // маркер: новый баланс + указатель на tx
}

func (op *operation) current() *balance             { return op.acc.balance.Load() }
func (op *operation) isClaimed(cur *balance) bool  { return cur == op.after }
func (op *operation) isStale(cur *balance) bool   { return cur != op.before }
func (op *operation) tryClaim() bool {
	return op.acc.balance.CompareAndSwap(op.before, op.after)
}

func (op *operation) tryFinalize() {
	op.acc.balance.CompareAndSwap(op.after, &balance{amount: op.after.amount})
}

func (op *operation) tryRestore() {
	op.acc.balance.CompareAndSwap(op.after, op.before)
}

func newMCASAccount(amount int64) *MCASAccount {
	a := &MCASAccount{}
	a.balance.Store(&balance{amount: amount})
	return a
}

// readBalance возвращает реальное значение ячейки,
// помогая завершить чужую транзакцию если ячейка захвачена.
func (a *MCASAccount) readBalance() *balance {
	for {
		b := a.balance.Load()
		if !b.isInFlight() {
			return b
		}
		b.tx.commit()
	}
}

// commit пытается завершить транзакцию (может вызываться несколькими горутинами).
func (t *tx) commit() {
	if !t.resolved() {
		ok := prepare(&t.firstOp) && prepare(&t.secondOp)
		t.tryResolve(ok)
	}

	if t.succeeded() {
		t.applyProgress()
	} else {
		t.applyRollback()
	}
}

func (t *tx) applyProgress() {
	t.firstOp.tryFinalize()
	t.secondOp.tryFinalize()
}

func (t *tx) applyRollback() {
	t.firstOp.tryRestore()
	t.secondOp.tryRestore()
}

// prepare захватывает ячейку для транзакции op.after.tx.
// Если ячейка занята другой транзакцией — помогает ей завершиться и повторяет.
func prepare(op *operation) bool {
	for {
		if op.after.tx.resolved() {
			return op.after.tx.succeeded()
		}

		cur := op.current()
		switch {
		case op.isClaimed(cur):  return true
		case cur.isInFlight():   cur.tx.commit(); continue
		case op.isStale(cur):    return false
		default:                 op.tryClaim()
		}
	}
}

func buildOperation(acct *MCASAccount, t *tx, delta int64) operation {
	current := acct.readBalance()
	return operation{
		acc:    acct,
		before: current,
		after:  &balance{amount: current.amount + delta, tx: t},
	}
}

func transferMCAS(from, to *MCASAccount, amount int64) {
	for {
		t := &tx{}
		t.firstOp = buildOperation(from, t, -amount)
		t.secondOp = buildOperation(to, t, +amount)

		if uintptr(unsafe.Pointer(from)) > uintptr(unsafe.Pointer(to)) {
			t.firstOp, t.secondOp = t.secondOp, t.firstOp
		}

		if t.commit(); t.succeeded() {
			return
		}
	}
}
