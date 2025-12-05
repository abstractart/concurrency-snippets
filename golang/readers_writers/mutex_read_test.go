package readers_writers

import (
	"sync"
	"testing"
	"time"
)

func BenchmarkRWRead(b *testing.B) {
	var mu sync.RWMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.RLock()
			time.Sleep(time.Microsecond) // или просто более длинная работа
			mu.RUnlock()
		}
	})
}

func BenchmarkMRead(b *testing.B) {
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			time.Sleep(time.Microsecond) // или просто более длинная работа
			mu.Unlock()
		}
	})
}

func BenchmarkMyRead(b *testing.B) {
	var mu MyRWMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.RLock()
			time.Sleep(time.Microsecond) // или просто более длинная работа
			mu.RUnlock()
		}
	})
}
