package stm

import (
	"math/rand"
	"sync"
	"sync/atomic"
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

// BenchmarkLargeReadSet: читаем все 10 аккаунтов, пишем только 2.
// STM делает 10 дешёвых Load + 2 CAS-цепочки.
func BenchmarkLargeReadSet(b *testing.B) {
	const n = 10
	accounts := make([]*Account, n)
	for i := range accounts {
		accounts[i] = NewAccount(1000)
	}

	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(rand.Int63()))
		for pb.Next() {
			from, to := rng.Intn(n), rng.Intn(n)
			if from == to {
				continue
			}
			Atomically(func(tx *Txn) {
				// читаем все аккаунты (большой read-set)
				for _, a := range accounts {
					_ = tx.Read(a)
				}
				tx.Write(accounts[from], tx.Read(accounts[from])-1)
				tx.Write(accounts[to], tx.Read(accounts[to])+1)
			})
		}
	})
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

func BenchmarkRetryRate(b *testing.B) {
	a1 := NewAccount(1_000_000)
	a2 := NewAccount(1_000_000)
	var attempts, commits atomic.Int64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for {
				attempts.Add(1)
				txn := newTxn()
				txn.Write(a1, txn.Read(a1)-1)
				txn.Write(a2, txn.Read(a2)+1)
				if txn.commit() {
					commits.Add(1)
					break
				}
			}
		}
	})

	b.ReportMetric(float64(attempts.Load())/float64(commits.Load()), "attempts/commit")
}
