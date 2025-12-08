package readers_writers

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func init() {
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

}

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

// go test -bench=BenchmarkMRead -benchmem -mutexprofile profile.out
// go test -bench=BenchmarkRWRead -benchmem -mutexprofile profile_rw.out

// go test -bench=BenchmarkMRead -benchmem -blockprofile profile_block.out
// go test -bench=BenchmarkRWRead -benchmem -blockprofile profile_block_rw.out
