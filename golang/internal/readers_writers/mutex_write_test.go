package readers_writers

import (
	"sync"
	"testing"
)

func BenchmarkRWWrite(b *testing.B) {
	var mu sync.RWMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			mu.Unlock()
		}
	})
}

func BenchmarkMWrite(b *testing.B) {
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			mu.Unlock()
		}
	})
}

func BenchmarkMChanWrite(b *testing.B) {
	var mu = NewChanMutex()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			mu.Unlock()
		}
	})
}

// func BenchmarkMyWrite(b *testing.B) {
// 	var mu MyRWMutex
// 	b.RunParallel(func(pb *testing.PB) {
// 		for pb.Next() {
// 			mu.Lock()
// 			time.Sleep(criticalsectionDuration) // или просто более длинная работа
// 			mu.Unlock()
// 		}
// 	})
// }
