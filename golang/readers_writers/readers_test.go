package readers_writers

import (
	"sync"
	"testing"
)

func BenchmarkSyncMutexImplementation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testCase(&MyRWMutexWithSyncMutex{}, 100, 0, 10000, map[int]struct{}{})
	}
}

func BenchmarkRWMutexImplementation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testCase(&sync.RWMutex{}, 100, 0, 10000, map[int]struct{}{})
	}
}

// func BenchmarkMyImplementation(b *testing.B) {
// 	for i := 0; i < b.N; i++ {
// 		mu := NewMyRWMutex()
// 		testCase(&mu, 100, 0, 10000, map[int]struct{}{})
// 	}
// }

func testCase(mu RMMutexI, readers int, writers int, iterations int, shared map[int]struct{}) {
	var wg sync.WaitGroup
	wg.Add(readers + writers)
	for range readers {
		go func() {
			defer wg.Done()
			for range iterations {
				Reader(shared, mu)
			}
		}()
	}

	for range writers {
		go func() {
			defer wg.Done()
			for range iterations {
				Writer(shared, mu)
			}
		}()
	}

	wg.Wait()
}
