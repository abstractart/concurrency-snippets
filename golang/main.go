package main

import (
	"sync"

	"github.com/concurrency-examples/golang/readers_writers"
)

func main() {
	var val int
	mu := readers_writers.NewChanMutex()

	var wg sync.WaitGroup
	wg.Add(10)

	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < 100000; j++ {
				mu.Lock()
				val += 1
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if val != 1000000 {
		panic(val)
	}

}
