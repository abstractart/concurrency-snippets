package readers_writers

import (
	"sync"
	"testing"
	"time"
)

func BenchmarkPauseRWWrite(b *testing.B) {
	var mu sync.RWMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			time.Sleep(criticalsectionDuration) // или просто более длинная работа
			mu.Unlock()
		}
	})
}

func BenchmarkPauseMWrite(b *testing.B) {
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			time.Sleep(criticalsectionDuration) // или просто более длинная работа
			mu.Unlock()
		}
	})
}

func BenchmarkPauseMChanWrite(b *testing.B) {
	var mu = NewChanMutex()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			time.Sleep(criticalsectionDuration) // или просто более длинная работа
			mu.Unlock()
		}
	})
}

// func BenchmarkMyWritePause(b *testing.B) {
// 	var mu MyRWMutex
// 	b.RunParallel(func(pb *testing.PB) {
// 		for pb.Next() {
// 			mu.Lock()
// 			time.Sleep(criticalsectionDuration) // или просто более длинная работа
// 			mu.Unlock()
// 		}
// 	})
// }
