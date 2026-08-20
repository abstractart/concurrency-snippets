package main

import "sync/atomic"

type AccountState struct {
	version int64
	balance int64
}

type Account struct {
	state atomic.Pointer[AccountState]
}

func transfer(from, to *Account, amount int64) {
	for {
		oldFrom := from.state.Load()
		oldTo := to.state.Load()

		newFrom := &AccountState{version: oldFrom.version + 1, balance: oldFrom.balance - amount}
		newTo := &AccountState{version: oldTo.version + 1, balance: oldTo.balance + amount}

		if from.state.CompareAndSwap(oldFrom, newFrom) {
			if to.state.CompareAndSwap(oldTo, newTo) {
				return
			}
			from.state.CompareAndSwap(newFrom, oldFrom) // откат
		}
		// иначе — перечитываем oldFrom/oldTo заново и пробуем снова
	}
}
