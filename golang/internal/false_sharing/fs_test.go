package false_sharing

import (
	"sync"
	"sync/atomic"
	"testing"
)

func BenchmarkSharing(bench *testing.B) {
	var a, b, c, d, e atomic.Int32
	var wg sync.WaitGroup

	wg.Add(5)
	go worker(&a, &wg)
	go worker(&b, &wg)
	go worker(&c, &wg)
	go worker(&d, &wg)
	go worker(&e, &wg)
	wg.Wait()
}

func BenchmarkNoSharing(bench *testing.B) {
	var a, b, c, d, e atomic.Int32

	worker(&a, nil)
	worker(&b, nil)
	worker(&c, nil)
	worker(&d, nil)
	worker(&e, nil)
}

func BenchmarkPadded(bench *testing.B) {
	var a, b, c, d, e PaddedInt32
	var wg sync.WaitGroup

	wg.Add(5)
	go workerPadded(&a, &wg)
	go workerPadded(&b, &wg)
	go workerPadded(&c, &wg)
	go workerPadded(&d, &wg)
	go workerPadded(&e, &wg)
	wg.Wait()
}

// go test -test.fullpath=true -benchmem -run=^$ -cpu=1,5 -bench ^(BenchmarkSharing|BenchmarkNoSharing|BenchmarkPadded)$ github.com/concurrency-examples/golang/internal/false_sharing
