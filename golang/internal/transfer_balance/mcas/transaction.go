package mcas

import "sync/atomic"

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
