package main

import (
	"fmt"
	"sync"
	"unsafe"
)

type Account struct {
	mu      sync.Mutex
	balance int64
}

func NewAccount(balance int64) *Account {
	return &Account{balance: balance}
}

func (a *Account) Balance() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.balance
}

// Transfer переводит amount со счёта from на счёт to.
// Мьютексы захватываются в порядке адресов, чтобы избежать дедлока.
func Transfer(from, to *Account, amount int64) {
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

func main() {
	a := NewAccount(1000)
	b := NewAccount(0)
	fmt.Printf("before: a=%d, b=%d\n", a.Balance(), b.Balance())
	Transfer(a, b, 300)
	fmt.Printf("after:  a=%d, b=%d\n", a.Balance(), b.Balance())
}
