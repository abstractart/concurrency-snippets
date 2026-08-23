package main

import (
	"fmt"
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

func (t *tx) status() txStatus { return txStatus(t.done.Load()) }
func (t *tx) resolved() bool   { return t.status() != txUnresolved }
func (t *tx) succeeded() bool  { return t.status() == txSucceeded }
func (t *tx) tryResolve(ok bool) {
	if ok {
		t.done.CompareAndSwap(int32(txUnresolved), int32(txSucceeded))
	} else {
		t.done.CompareAndSwap(int32(txUnresolved), int32(txFailed))
	}
}

func (t *tx) commit() {
	if !t.resolved() {
		t.tryResolve(t.prepare())
	}
	if t.succeeded() {
		t.applyProgress()
	} else {
		t.applyRollback()
	}
}

func (t *tx) prepare() bool {
	return t.firstOp.prepare() && t.secondOp.prepare()
}

func (t *tx) applyProgress() {
	t.firstOp.tryFinalize()
	t.secondOp.tryFinalize()
}

func (t *tx) applyRollback() {
	t.firstOp.tryRestore()
	t.secondOp.tryRestore()
}

type operation struct {
	acc    *Account
	before *balance
	after  *balance
}

func newOperation(acct *Account, t *tx, delta int64) operation {
	current := acct.readBalance()
	return operation{
		acc:    acct,
		before: current,
		after:  &balance{amount: current.amount + delta, tx: t},
	}
}

func (op *operation) tx() *tx                     { return op.after.tx }
func (op *operation) current() *balance           { return op.acc.balance.Load() }
func (op *operation) isClaimed(cur *balance) bool { return cur == op.after }
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

func (op *operation) prepare() bool {
	for {
		if op.tx().resolved() {
			return op.tx().succeeded()
		}
		cur := op.current()
		switch {
		case op.isClaimed(cur): return true
		case cur.isInFlight():  cur.tx.commit(); continue
		case op.isStale(cur):  return false
		default:               op.tryClaim()
		}
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

func main() {
	a := NewAccount(1000)
	b := NewAccount(0)
	fmt.Printf("before: a=%d, b=%d\n", a.Balance(), b.Balance())
	a.Transfer(b, 300)
	fmt.Printf("after:  a=%d, b=%d\n", a.Balance(), b.Balance())
}
