package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	var a, b, c, d, e, f, g, h, i, j PaddedInt32
	fmt.Printf("address of a: %p\n", &a)
	fmt.Printf("address of b: %p\n", &b)
	fmt.Printf("address of c: %p\n", &c)
	fmt.Printf("address of d: %p\n", &d)
	fmt.Printf("address of e: %p\n", &e)
	fmt.Printf("address of f: %p\n", &f)
	fmt.Printf("address of g: %p\n", &g)
	fmt.Printf("address of h: %p\n", &h)
	fmt.Printf("address of i: %p\n", &i)
	fmt.Printf("address of j: %p\n", &j)
}

type PaddedInt32 struct {
	value atomic.Int32
	_     [60]byte // Padding to avoid false sharing (assuming 64-byte cache line)
}

func worker(v *atomic.Int32, wg *sync.WaitGroup) {
	for i := 0; i < 100000000; i++ {
		v.Add(1)
	}
	if wg != nil {
		wg.Done()
	}
}

func worker_padded(v *PaddedInt32, wg *sync.WaitGroup) {
	for i := 0; i < 100000000; i++ {
		v.value.Add(1)
	}
	if wg != nil {
		wg.Done()
	}
}
