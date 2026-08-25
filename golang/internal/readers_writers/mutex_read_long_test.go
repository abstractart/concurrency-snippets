package readers_writers

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

const criticalsectionDuration = time.Millisecond

func init() {
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

}

func BenchmarkPauseRWRead(b *testing.B) {
	var mu sync.RWMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.RLock()
			time.Sleep(criticalsectionDuration) // или просто более длинная работа
			mu.RUnlock()
		}
	})
}

func BenchmarkPauseMRead(b *testing.B) {
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			time.Sleep(criticalsectionDuration) // или просто более длинная работа
			mu.Unlock()
		}
	})
}

func BenchmarkPauseMChanRead(b *testing.B) {
	var mu = NewChanMutex()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			time.Sleep(criticalsectionDuration) // или просто более длинная работа
			mu.Unlock()
		}
	})
}

// func BenchmarkMyReadPause(b *testing.B) {
// 	var mu MyRWMutex
// 	b.RunParallel(func(pb *testing.PB) {
// 		for pb.Next() {
// 			mu.RLock()
// 			time.Sleep(criticalsectionDuration) // или просто более длинная работа
// 			mu.RUnlock()
// 		}
// 	})
// }
