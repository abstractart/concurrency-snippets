package main

import (
	"fmt"
	"sort"
	"sync/atomic"
	"unsafe"
)

// tvarWord — значение ячейки: реальное (desc=nil) или маркер транзакции (desc!=nil).
type tvarWord struct {
	value int64
	desc  *stmDesc
}

type Account struct {
	word atomic.Pointer[tvarWord]
}

func NewAccount(balance int64) *Account {
	a := &Account{}
	a.word.Store(&tvarWord{value: balance})
	return a
}

func (a *Account) Balance() int64 {
	return resolve(a).value
}

// resolve возвращает реальное значение, помогая завершить чужие транзакции.
func resolve(a *Account) *tvarWord {
	for {
		w := a.word.Load()
		if w.desc == nil {
			return w
		}
		commitDesc(w.desc)
	}
}

// stmDesc — MCAS-дескриптор для N переменных.
type stmDesc struct {
	done    atomic.Int32 // 0=pending, 1=ok, 2=fail
	entries []*stmEntry
}

type stmEntry struct {
	acct   *Account
	before *tvarWord
	after  *tvarWord // маркер: новое значение + указатель на дескриптор
}

func commitDesc(desc *stmDesc) {
	if desc.done.Load() == 0 {
		ok := true
		for _, e := range desc.entries {
			if !prepareEntry(e) {
				ok = false
				break
			}
		}
		status := int32(1)
		if !ok {
			status = 2
		}
		desc.done.CompareAndSwap(0, status)
	}

	success := desc.done.Load() == 1
	for _, e := range desc.entries {
		if success {
			e.acct.word.CompareAndSwap(e.after, &tvarWord{value: e.after.value})
		} else {
			e.acct.word.CompareAndSwap(e.after, e.before)
		}
	}
}

func prepareEntry(e *stmEntry) bool {
	t := e.after.desc
	for {
		if s := t.done.Load(); s != 0 {
			return s == 1
		}
		cur := e.acct.word.Load()
		if cur == e.after {
			return true
		}
		if cur.desc != nil {
			commitDesc(cur.desc)
			continue
		}
		if cur != e.before {
			return false
		}
		e.acct.word.CompareAndSwap(e.before, e.after)
	}
}

// Txn — контекст транзакции: накапливает чтения и записи, не трогая реальные значения.
type Txn struct {
	reads  map[*Account]*tvarWord
	writes map[*Account]int64
}

func newTxn() *Txn {
	return &Txn{reads: make(map[*Account]*tvarWord), writes: make(map[*Account]int64)}
}

func (t *Txn) Read(a *Account) int64 {
	if val, ok := t.writes[a]; ok {
		return val // read-your-own-writes
	}
	word := resolve(a)
	t.reads[a] = word
	return word.value
}

func (t *Txn) Write(a *Account, val int64) {
	t.writes[a] = val
}

func (t *Txn) commit() bool {
	desc := &stmDesc{}

	// Entries в порядке адресов — предотвращает circular helping.
	accounts := make([]*Account, 0, len(t.writes))
	for a := range t.writes {
		accounts = append(accounts, a)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return uintptr(unsafe.Pointer(accounts[i])) < uintptr(unsafe.Pointer(accounts[j]))
	})

	for _, a := range accounts {
		before := t.reads[a]
		if before == nil {
			before = resolve(a)
		}
		desc.entries = append(desc.entries, &stmEntry{
			acct:   a,
			before: before,
			after:  &tvarWord{value: t.writes[a], desc: desc},
		})
	}

	commitDesc(desc)
	return desc.done.Load() == 1
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

func main() {
	a := NewAccount(1000)
	b := NewAccount(0)
	fmt.Printf("before: a=%d, b=%d\n", a.Balance(), b.Balance())
	Transfer(a, b, 300)
	fmt.Printf("after:  a=%d, b=%d\n", a.Balance(), b.Balance())
}
