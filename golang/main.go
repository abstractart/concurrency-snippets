package main

import "github.com/concurrency-examples/golang/readers_writers"

func main() {
	shared := make(map[int]struct{})
	mu := readers_writers.MyRWMutexWithSyncMutex{}

	for i := 0; i < 4; i++ {
		go func() {
			readers_writers.Reader(shared, &mu)
		}()
	}
	for i := 0; i < 2; i++ {
		go func() {
			readers_writers.Writer(shared, &mu)
		}()
	}

	select {}
}
