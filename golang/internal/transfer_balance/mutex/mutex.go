package mutex

import (
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
