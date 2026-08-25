package stm

import (
	"fmt"
	"sort"
	"sync/atomic"
	"unsafe"
)

// word — значение ячейки: реальное (desc==nil) или маркер транзакции (desc!=nil).
type word struct {
	value int64
	desc  *desc
}

func (w *word) isInFlight() bool { return w.desc != nil }

type Account struct {
	word atomic.Pointer[word]
}

func NewAccount(balance int64) *Account {
	a := &Account{}
	a.word.Store(&word{value: balance})
	return a
}

func (a *Account) Balance() int64 { return a.resolve().value }

func (a *Account) resolve() *word {
	for {
		w := a.word.Load()
		if !w.isInFlight() {
			return w
		}
		w.desc.commit()
	}
}

type descStatus int32

const (
	descUnresolved descStatus = iota
	descSucceeded
	descFailed
)

// desc — MCAS-дескриптор для N переменных.
// Публикуется в ячейки до того, как значения изменятся.
type desc struct {
	done    atomic.Int32
	entries []*entry
}

func (d *desc) status() descStatus { return descStatus(d.done.Load()) }
func (d *desc) resolved() bool     { return d.status() != descUnresolved }
func (d *desc) succeeded() bool    { return d.status() == descSucceeded }
func (d *desc) tryResolve(ok bool) {
	if ok {
		d.done.CompareAndSwap(int32(descUnresolved), int32(descSucceeded))
	} else {
		d.done.CompareAndSwap(int32(descUnresolved), int32(descFailed))
	}
}

func (d *desc) commit() {
	if !d.resolved() {
		d.tryResolve(d.prepare())
	}
	if d.succeeded() {
		d.applyProgress()
	} else {
		d.applyRollback()
	}
}

func (d *desc) prepare() bool {
	for _, e := range d.entries {
		if !e.prepare() {
			return false
		}
	}
	return true
}

func (d *desc) applyProgress() {
	for _, e := range d.entries {
		e.tryFinalize()
	}
}

func (d *desc) applyRollback() {
	for _, e := range d.entries {
		e.tryRestore()
	}
}

type entry struct {
	acct   *Account
	before *word
	after  *word
}

func (e *entry) desc() *desc                { return e.after.desc }
func (e *entry) current() *word             { return e.acct.word.Load() }
func (e *entry) isClaimed(cur *word) bool   { return cur == e.after }
func (e *entry) isStale(cur *word) bool     { return cur != e.before }
func (e *entry) tryClaim() bool             { return e.acct.word.CompareAndSwap(e.before, e.after) }
func (e *entry) tryFinalize()               { e.acct.word.CompareAndSwap(e.after, &word{value: e.after.value}) }
func (e *entry) tryRestore()               { e.acct.word.CompareAndSwap(e.after, e.before) }

func (e *entry) prepare() bool {
	for {
		if e.desc().resolved() {
			return e.desc().succeeded()
		}
		cur := e.current()
		switch {
		case e.isClaimed(cur):
			return true
		case cur.isInFlight():
			cur.desc.commit()
			continue
		case e.isStale(cur):
			return false
		default:
			e.tryClaim()
		}
	}
}

type readEntry struct {
	acct *Account
	snap *word
}

type writeEntry struct {
	acct  *Account
	value int64
}

// Txn — контекст транзакции: накапливает чтения и записи, не трогая реальные значения.
// Слайсы вместо map: линейный поиск по указателю быстрее для малого N
// из-за cache locality и отсутствия хеширования.
type Txn struct {
	reads  []readEntry
	writes []writeEntry
}

func newTxn() *Txn {
	return &Txn{
		reads:  make([]readEntry, 0, 8),
		writes: make([]writeEntry, 0, 4),
	}
}

func (t *Txn) Read(a *Account) int64 {
	for i := range t.writes {
		if t.writes[i].acct == a {
			return t.writes[i].value // read-your-own-writes
		}
	}
	for i := range t.reads {
		if t.reads[i].acct == a {
			return t.reads[i].snap.value
		}
	}
	w := a.resolve()
	t.reads = append(t.reads, readEntry{acct: a, snap: w})
	return w.value
}

func (t *Txn) Write(a *Account, val int64) {
	for i := range t.writes {
		if t.writes[i].acct == a {
			t.writes[i].value = val
			return
		}
	}
	t.writes = append(t.writes, writeEntry{acct: a, value: val})
}

func (t *Txn) readSnap(a *Account) *word {
	for i := range t.reads {
		if t.reads[i].acct == a {
			return t.reads[i].snap
		}
	}
	return a.resolve()
}

func (t *Txn) commit() bool {
	// Сортируем writes по адресу — предотвращает circular helping.
	sort.Slice(t.writes, func(i, j int) bool {
		return uintptr(unsafe.Pointer(t.writes[i].acct)) < uintptr(unsafe.Pointer(t.writes[j].acct))
	})

	d := &desc{}
	for _, w := range t.writes {
		d.entries = append(d.entries, &entry{
			acct:   w.acct,
			before: t.readSnap(w.acct),
			after:  &word{value: w.value, desc: d},
		})
	}

	d.commit()
	return d.succeeded()
}

// Atomically выполняет fn в транзакции, повторяя при конфликте.
func Atomically(fn func(*Txn)) {
	for {
		txn := newTxn()
		fn(txn)
		if txn.commit() {
			return
		}
	}
}

func Transfer(from, to *Account, amount int64) {
	Atomically(func(tx *Txn) {
		tx.Write(from, tx.Read(from)-amount)
		tx.Write(to, tx.Read(to)+amount)
	})
}

func Run() {
	a := NewAccount(1000)
	b := NewAccount(0)
	fmt.Printf("before: a=%d, b=%d\n", a.Balance(), b.Balance())
	Transfer(a, b, 300)
	fmt.Printf("after:  a=%d, b=%d\n", a.Balance(), b.Balance())
}
