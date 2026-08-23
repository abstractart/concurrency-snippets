package main

import (
	"math/rand"
	"sync"
	"testing"
)

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

func TestTransferTwoAccounts(t *testing.T) {
	a := NewAccount(10_000)
	b := NewAccount(10_000)
	wantTotal := int64(20_000)

	var wg sync.WaitGroup
	for g := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 500 {
				if g%2 == 0 {
					Transfer(a, b, 1)
				} else {
					Transfer(b, a, 1)
				}
			}
		}()
	}
	wg.Wait()

	if got := a.Balance() + b.Balance(); got != wantTotal {
		t.Errorf("money not conserved: want %d, got %d", wantTotal, got)
	}
}

func TestTransferPingPong(t *testing.T) {
	a := NewAccount(1_000_000)
	b := NewAccount(0)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				Transfer(a, b, 1)
				Transfer(b, a, 1)
			}
		}()
	}
	wg.Wait()

	if got := a.Balance() + b.Balance(); got != 1_000_000 {
		t.Errorf("money not conserved: want 1000000, got %d", got)
	}
}

func BenchmarkTransfer(b *testing.B) {
	a1 := NewAccount(1_000_000_000)
	a2 := NewAccount(1_000_000_000)

	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(rand.Int63()))
		for pb.Next() {
			if rng.Intn(2) == 0 {
				Transfer(a1, a2, 1)
			} else {
				Transfer(a2, a1, 1)
			}
		}
	})

	if got := a1.Balance() + a2.Balance(); got != 2_000_000_000 {
		b.Errorf("money not conserved: got %d", got)
	}
}
