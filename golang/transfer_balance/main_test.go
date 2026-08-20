package main

import (
	"math/rand"
	"sync"
	"testing"
)

// --- helpers для baggy CAS-реализации ---

func newAccount(balance int64) *Account {
	a := &Account{}
	a.state.Store(&AccountState{version: 0, balance: balance})
	return a
}

func totalBalance(accounts []*Account) int64 {
	var total int64
	for _, a := range accounts {
		total += a.state.Load().balance
	}
	return total
}

// --- helpers для mutex-реализации ---

func newSafeAccount(balance int64) *SafeAccount {
	return &SafeAccount{balance: balance}
}

func totalSafeBalance(accounts []*SafeAccount) int64 {
	var total int64
	for _, a := range accounts {
		a.mu.Lock()
		total += a.balance
		a.mu.Unlock()
	}
	return total
}

// =============================================================
// Тесты для багованной CAS-реализации (transfer)
// =============================================================

// TestTransferConservation проверяет что сумма балансов не меняется.
func TestTransferConservation(t *testing.T) {
	const (
		numAccounts   = 10
		initialFunds  = 1000
		numGoroutines = 50
		numTransfers  = 200
	)

	accounts := make([]*Account, numAccounts)
	for i := range accounts {
		accounts[i] = newAccount(initialFunds)
	}
	wantTotal := int64(numAccounts * initialFunds)

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(rand.Int63()))
			for i := 0; i < numTransfers; i++ {
				from := accounts[rng.Intn(numAccounts)]
				to := accounts[rng.Intn(numAccounts)]
				if from == to {
					continue
				}
				transfer(from, to, int64(rng.Intn(10)+1))
			}
		}()
	}
	wg.Wait()

	got := totalBalance(accounts)
	if got != wantTotal {
		t.Errorf("money not conserved: want %d, got %d (delta %d)", wantTotal, got, wantTotal-got)
	}
}

// TestTransferTwoAccounts простейший сценарий: два счёта, много горутин.
func TestTransferTwoAccounts(t *testing.T) {
	const (
		initial       = 10_000
		numGoroutines = 100
		numTransfers  = 500
		amount        = 1
	)

	a := newAccount(initial)
	b := newAccount(initial)
	wantTotal := int64(2 * initial)

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < numTransfers; i++ {
				if id%2 == 0 {
					transfer(a, b, amount)
				} else {
					transfer(b, a, amount)
				}
			}
		}(g)
	}
	wg.Wait()

	got := totalBalance([]*Account{a, b})
	if got != wantTotal {
		t.Errorf("money not conserved: want %d, got %d (delta %d)", wantTotal, got, wantTotal-got)
	}
}

// TestTransferVersionMonotonicity — пинг-понг между двумя счетами.
func TestTransferVersionMonotonicity(t *testing.T) {
	a := newAccount(1_000_000)
	b := newAccount(0)

	const numGoroutines = 20
	const numTransfers = 1000

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numTransfers; i++ {
				transfer(a, b, 1)
				transfer(b, a, 1)
			}
		}()
	}
	wg.Wait()

	got := totalBalance([]*Account{a, b})
	if got != 1_000_000 {
		t.Errorf("money not conserved: want 1000000, got %d", got)
	}
}

// =============================================================
// Тесты для корректной mutex-реализации (transferSafe)
// =============================================================

// TestSafeTransferConservation — те же сценарии, но с SafeAccount.
func TestSafeTransferConservation(t *testing.T) {
	const (
		numAccounts   = 10
		initialFunds  = 1000
		numGoroutines = 50
		numTransfers  = 200
	)

	accounts := make([]*SafeAccount, numAccounts)
	for i := range accounts {
		accounts[i] = newSafeAccount(initialFunds)
	}
	wantTotal := int64(numAccounts * initialFunds)

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(rand.Int63()))
			for i := 0; i < numTransfers; i++ {
				from := accounts[rng.Intn(numAccounts)]
				to := accounts[rng.Intn(numAccounts)]
				if from == to {
					continue
				}
				transferSafe(from, to, int64(rng.Intn(10)+1))
			}
		}()
	}
	wg.Wait()

	got := totalSafeBalance(accounts)
	if got != wantTotal {
		t.Errorf("money not conserved: want %d, got %d (delta %d)", wantTotal, got, wantTotal-got)
	}
}

// TestSafeTransferTwoAccounts — два счёта, встречные переводы.
func TestSafeTransferTwoAccounts(t *testing.T) {
	const (
		initial       = 10_000
		numGoroutines = 100
		numTransfers  = 500
		amount        = 1
	)

	a := newSafeAccount(initial)
	b := newSafeAccount(initial)
	wantTotal := int64(2 * initial)

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < numTransfers; i++ {
				if id%2 == 0 {
					transferSafe(a, b, amount)
				} else {
					transferSafe(b, a, amount)
				}
			}
		}(g)
	}
	wg.Wait()

	got := totalSafeBalance([]*SafeAccount{a, b})
	if got != wantTotal {
		t.Errorf("money not conserved: want %d, got %d (delta %d)", wantTotal, got, wantTotal-got)
	}
}

// TestSafeTransferPingPong — пинг-понг, проверяем отсутствие дедлока.
func TestSafeTransferPingPong(t *testing.T) {
	a := newSafeAccount(1_000_000)
	b := newSafeAccount(0)

	const numGoroutines = 20
	const numTransfers = 1000

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numTransfers; i++ {
				transferSafe(a, b, 1)
				transferSafe(b, a, 1)
			}
		}()
	}
	wg.Wait()

	got := totalSafeBalance([]*SafeAccount{a, b})
	if got != 1_000_000 {
		t.Errorf("money not conserved: want 1000000, got %d", got)
	}
}

// =============================================================
// Бенчмарки
// =============================================================

func BenchmarkTransfer(b *testing.B) {
	a1 := newAccount(1_000_000_000)
	a2 := newAccount(1_000_000_000)

	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(rand.Int63()))
		for pb.Next() {
			if rng.Intn(2) == 0 {
				transfer(a1, a2, 1)
			} else {
				transfer(a2, a1, 1)
			}
		}
	})

	got := totalBalance([]*Account{a1, a2})
	if got != 2_000_000_000 {
		b.Errorf("money not conserved: got %d", got)
	}
}

// =============================================================
// Тесты для lock-free MCAS-реализации (transferMCAS)
// =============================================================

func totalMCASBalance(accounts []*MCASAccount) int64 {
	var total int64
	for _, a := range accounts {
		total += readMCASBalance(a)
	}
	return total
}

func TestMCASTransferConservation(t *testing.T) {
	const (
		numAccounts   = 10
		initialFunds  = 1000
		numGoroutines = 50
		numTransfers  = 200
	)

	accounts := make([]*MCASAccount, numAccounts)
	for i := range accounts {
		accounts[i] = newMCASAccount(initialFunds)
	}
	wantTotal := int64(numAccounts * initialFunds)

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(rand.Int63()))
			for i := 0; i < numTransfers; i++ {
				from := accounts[rng.Intn(numAccounts)]
				to := accounts[rng.Intn(numAccounts)]
				if from == to {
					continue
				}
				transferMCAS(from, to, int64(rng.Intn(10)+1))
			}
		}()
	}
	wg.Wait()

	got := totalMCASBalance(accounts)
	if got != wantTotal {
		t.Errorf("money not conserved: want %d, got %d (delta %d)", wantTotal, got, wantTotal-got)
	}
}

func TestMCASTransferTwoAccounts(t *testing.T) {
	const (
		initial       = 10_000
		numGoroutines = 100
		numTransfers  = 500
		amount        = 1
	)

	a := newMCASAccount(initial)
	b := newMCASAccount(initial)
	wantTotal := int64(2 * initial)

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < numTransfers; i++ {
				if id%2 == 0 {
					transferMCAS(a, b, amount)
				} else {
					transferMCAS(b, a, amount)
				}
			}
		}(g)
	}
	wg.Wait()

	got := totalMCASBalance([]*MCASAccount{a, b})
	if got != wantTotal {
		t.Errorf("money not conserved: want %d, got %d (delta %d)", wantTotal, got, wantTotal-got)
	}
}

func TestMCASTransferPingPong(t *testing.T) {
	a := newMCASAccount(1_000_000)
	b := newMCASAccount(0)

	const numGoroutines = 20
	const numTransfers = 1000

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numTransfers; i++ {
				transferMCAS(a, b, 1)
				transferMCAS(b, a, 1)
			}
		}()
	}
	wg.Wait()

	got := totalMCASBalance([]*MCASAccount{a, b})
	if got != 1_000_000 {
		t.Errorf("money not conserved: want 1000000, got %d", got)
	}
}

func BenchmarkTransferMCAS(b *testing.B) {
	a1 := newMCASAccount(1_000_000_000)
	a2 := newMCASAccount(1_000_000_000)

	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(rand.Int63()))
		for pb.Next() {
			if rng.Intn(2) == 0 {
				transferMCAS(a1, a2, 1)
			} else {
				transferMCAS(a2, a1, 1)
			}
		}
	})

	got := totalMCASBalance([]*MCASAccount{a1, a2})
	if got != 2_000_000_000 {
		b.Errorf("money not conserved: got %d", got)
	}
}

func BenchmarkTransferSafe(b *testing.B) {
	a1 := newSafeAccount(1_000_000_000)
	a2 := newSafeAccount(1_000_000_000)

	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(rand.Int63()))
		for pb.Next() {
			if rng.Intn(2) == 0 {
				transferSafe(a1, a2, 1)
			} else {
				transferSafe(a2, a1, 1)
			}
		}
	})

	got := totalSafeBalance([]*SafeAccount{a1, a2})
	if got != 2_000_000_000 {
		b.Errorf("money not conserved: got %d", got)
	}
}
