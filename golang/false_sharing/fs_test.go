package main

import (
	"sync"
	"sync/atomic"
	"testing"
)

func BenchmarkSharing(bench *testing.B) {
	var a, b, c, d, e atomic.Int32 //, f, g, h, i, j atomic.Int32
	var wg sync.WaitGroup

	wg.Add(5)
	go worker(&a, &wg)
	go worker(&b, &wg)
	go worker(&c, &wg)
	go worker(&d, &wg)
	go worker(&e, &wg)
	// go worker(&f, &wg)
	// go worker(&g, &wg)
	// go worker(&h, &wg)
	// go worker(&i, &wg)
	// go worker(&j, &wg)
	wg.Wait()
}

func BenchmarkNoSharing(bench *testing.B) {
	var a, b, c, d, e atomic.Int32 //, f, g, h, i, j atomic.Int32

	worker(&a, nil)
	worker(&b, nil)
	worker(&c, nil)
	worker(&d, nil)
	worker(&e, nil)
	// worker(&f, nil)
	// worker(&g, nil)
	// worker(&h, nil)
	// worker(&i, nil)
	// worker(&j, nil)
}

func BenchmarkPadded(bench *testing.B) {
	var a, b, c, d, e PaddedInt32
	//f, g, h, i, j PaddedInt32
	var wg sync.WaitGroup

	wg.Add(5)
	go worker_padded(&a, &wg)
	go worker_padded(&b, &wg)
	go worker_padded(&c, &wg)
	go worker_padded(&d, &wg)
	go worker_padded(&e, &wg)
	// go worker_padded(&f, &wg)
	// go worker_padded(&g, &wg)
	// go worker_padded(&h, &wg)
	// go worker_padded(&i, &wg)
	// go worker_padded(&j, &wg)
	wg.Wait()
}

// go test -test.fullpath=true -benchmem -run=^$ -cpu=1,5 -bench ^(BenchmarkSharing|BenchmarkNoSharing|BenchmarkPadded)$ github.com/concurrency-examples/golang/false_sharing
/*

➜  false_sharing git:(main) ✗ go test -test.fullpath=true -benchmem -run=^$ -cpu=1,5 -bench '^(BenchmarkSharing|BenchmarkNoSharing|BenchmarkPadded)$' github.com/concurrency-examples/golang/false_sharing
goos: darwin
goarch: arm64
pkg: github.com/concurrency-examples/golang/false_sharing
cpu: Apple M4
BenchmarkSharing               1        7086535625 ns/op            7752 B/op         19 allocs/op
BenchmarkSharing-5             1        7155707459 ns/op             168 B/op          8 allocs/op
BenchmarkNoSharing      1000000000               0.8457 ns/op          0 B/op          0 allocs/op
BenchmarkNoSharing-5    1000000000               0.8327 ns/op          0 B/op          0 allocs/op
BenchmarkPadded         1000000000               0.8327 ns/op          0 B/op          0 allocs/op
BenchmarkPadded-5       1000000000               0.2431 ns/op          0 B/op          0 allocs/op
PASS
ok      github.com/concurrency-examples/golang/false_sharing    168.026s
*/
