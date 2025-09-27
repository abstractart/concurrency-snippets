package main

import (
	"fmt"
	"sync/atomic"
)

type MyMutex struct {
	v atomic.Int32
}

func (m *MyMutex) Lock() {
	for {
		if m.v.CompareAndSwap(0, 1) {
			return
		}
		//time.Sleep(time.Millisecond)
	}
}

func (m *MyMutex) Unlock() {
	m.v.Store(0)
}

type MyWaitGroup struct {
	v atomic.Int32
}

func (m *MyWaitGroup) Add(delta int) {
	m.v.Add(int32(delta))
}

func (m *MyWaitGroup) Wait() {
	for {
		if m.v.Load() == 0 {
			return
		}
		//time.Sleep(time.Millisecond)
	}
}

func (m *MyWaitGroup) Done() {
	m.v.Add(-1)
}

func main() {
	var mu MyMutex
	var wg MyWaitGroup
	var count int

	goroutines := 11
	iterations := 100000

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			for range iterations {
				mu.Lock()
				count++
				mu.Unlock()
			}
			wg.Done()
		}(i)
	}

	wg.Wait()
	fmt.Println(count, goroutines*iterations)
}
