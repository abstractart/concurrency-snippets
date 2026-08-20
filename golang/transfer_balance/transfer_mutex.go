package main

import (
	"sync"
	"unsafe"
)

type SafeAccount struct {
	mu      sync.Mutex
	balance int64
}

func transferSafe(from, to *SafeAccount, amount int64) {
	first, second := from, to
	if uintptr(unsafe.Pointer(from)) > uintptr(unsafe.Pointer(to)) {
		first, second = to, from
	}

	first.mu.Lock()
	second.mu.Lock()

	from.balance -= amount
	to.balance += amount

	second.mu.Unlock()
	first.mu.Unlock()
}
