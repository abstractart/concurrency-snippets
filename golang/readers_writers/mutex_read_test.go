package readers_writers

import (
	"runtime"
	"sync"
	"testing"
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
			mu.RUnlock()
		}
	})
}

func BenchmarkMRead(b *testing.B) {
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			mu.Unlock()
		}
	})
}

func BenchmarkMChanRead(b *testing.B) {
	var mu = NewChanMutex()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			mu.Unlock()
		}
	})
}

// func BenchmarkMyRead(b *testing.B) {
// 	var mu MyRWMutex
// 	b.RunParallel(func(pb *testing.PB) {
// 		for pb.Next() {
// 			mu.RLock()
// 			//time.Sleep(criticalsectionDuration) // или просто более длинная работа
// 			mu.RUnlock()
// 		}
// 	})
// }

// go test -bench=BenchmarkMRead -benchmem -mutexprofile profile.out
// go test -bench=BenchmarkRWRead -benchmem -mutexprofile profile_rw.out

// go test -bench=BenchmarkMRead -benchmem -blockprofile profile_block.out
// go test -bench=BenchmarkRWRead -benchmem -blockprofile profile_block_rw.out

//go test -bench=BenchmarkMRead -benchmem -mutexprofile=BenchmarkMRead_mutex_profile.out -cpuprofile=BenchmarkMRead_cpu_profile.out -memprofile=BenchmarkMRead_memory_profile.out
