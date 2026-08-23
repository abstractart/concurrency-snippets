package main

import (
	"fmt"
	"sync/atomic"
)

type AccountState struct {
	version int64
	balance int64
}

type Account struct {
	state atomic.Pointer[AccountState]
}

func NewAccount(balance int64) *Account {
	a := &Account{}
	a.state.Store(&AccountState{version: 0, balance: balance})
	return a
}

func (a *Account) Balance() int64 {
	return a.state.Load().balance
}

// Transfer содержит намеренный баг: откат при провале второго CAS может сам провалиться,
// деньги теряются безвозвратно.
func Transfer(from, to *Account, amount int64) {
	for {
		oldFrom := from.state.Load()
		oldTo := to.state.Load()

		newFrom := &AccountState{version: oldFrom.version + 1, balance: oldFrom.balance - amount}
		newTo := &AccountState{version: oldTo.version + 1, balance: oldTo.balance + amount}

		if from.state.CompareAndSwap(oldFrom, newFrom) {
			if to.state.CompareAndSwap(oldTo, newTo) {
				return
			}
			from.state.CompareAndSwap(newFrom, oldFrom) // откат — может провалиться!
		}
	}
}

func main() {
	a := NewAccount(1000)
	b := NewAccount(0)
	fmt.Printf("before: a=%d, b=%d\n", a.Balance(), b.Balance())
	Transfer(a, b, 300)
	fmt.Printf("after:  a=%d, b=%d\n", a.Balance(), b.Balance())
}
