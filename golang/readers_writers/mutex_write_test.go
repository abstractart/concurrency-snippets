package readers_writers

import (
	"sync"
	"testing"
	"time"
)

func BenchmarkRWWrite(b *testing.B) {
	var mu sync.RWMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			time.Sleep(time.Microsecond) // или просто более длинная работа
			mu.Unlock()
		}
	})
}

func BenchmarkMWrite(b *testing.B) {
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			time.Sleep(time.Microsecond) // или просто более длинная работа
			mu.Unlock()
		}
	})
}

func BenchmarkMyWrite(b *testing.B) {
	var mu MyRWMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			time.Sleep(time.Microsecond) // или просто более длинная работа
			mu.Unlock()
		}
	})
}
