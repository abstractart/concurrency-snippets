package cas

import (
	"math/rand"
	"sync"
	"testing"
)

// Тесты намеренно падают — демонстрируют баг потери денег при конкурентных переводах.

func TestTransferBasic(t *testing.T) {
	a := NewAccount(1000)
	b := NewAccount(0)

	Transfer(a, b, 300)

	if got := a.Balance(); got != 700 {
		t.Errorf("from: want 700, got %d", got)
	}
	if got := b.Balance(); got != 300 {
		t.Errorf("to: want 300, got %d", got)
	}
}

func TestTransferConservation(t *testing.T) {
	const (
		numAccounts   = 10
		initialFunds  = 1000
		numGoroutines = 50
		numTransfers  = 200
	)

	accounts := make([]*Account, numAccounts)
	for i := range accounts {
		accounts[i] = NewAccount(initialFunds)
	}
	wantTotal := int64(numAccounts * initialFunds)

	var wg sync.WaitGroup
	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(rand.Int63()))
			for range numTransfers {
				from := accounts[rng.Intn(numAccounts)]
				to := accounts[rng.Intn(numAccounts)]
				if from == to {
					continue
				}
				Transfer(from, to, int64(rng.Intn(10)+1))
			}
		}()
	}
	wg.Wait()

	var total int64
	for _, a := range accounts {
		total += a.Balance()
	}
	if total != wantTotal {
		t.Errorf("money not conserved: want %d, got %d (delta %d)", wantTotal, total, wantTotal-total)
	}
}
