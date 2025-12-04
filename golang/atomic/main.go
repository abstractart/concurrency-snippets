package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type MutexI interface {
	Lock()
	Unlock()
}

type WaitGroupI interface {
	Add(delta int)
	Wait()
	Done()
}

type SpinMutex struct {
	v atomic.Int32
}

func (m *SpinMutex) Lock() {
	for {
		if m.v.CompareAndSwap(0, 1) {
			return
		}
	}
}

func (m *SpinMutex) Unlock() {
	m.v.Store(0)
}

type SpinWaitGroup struct {
	v atomic.Int32
}

func (m *SpinWaitGroup) Add(delta int) {
	m.v.Add(int32(delta))
}

func (m *SpinWaitGroup) Wait() {
	for {
		if m.v.Load() == 0 {
			return
		}
	}
}

func (m *SpinWaitGroup) Done() {
	m.v.Add(-1)
}

type MutexWithPause struct {
	v atomic.Int32
}

func (m *MutexWithPause) Lock() {
	for {
		if m.v.CompareAndSwap(0, 1) {
			return
		}
		runtime.Gosched()
	}
}

func (m *MutexWithPause) Unlock() {
	m.v.Store(0)
}

type WaitGroupWithPause struct {
	v atomic.Int32
}

func (m *WaitGroupWithPause) Add(delta int) {
	m.v.Add(int32(delta))
}

func (m *WaitGroupWithPause) Wait() {
	for {
		if m.v.Load() == 0 {
			return
		}
		runtime.Gosched()
	}
}

func (m *WaitGroupWithPause) Done() {
	m.v.Add(-1)
}

type SyncPrimitiveTuple struct {
	mu MutexI
	wg WaitGroupI
}

var implementations = map[string]SyncPrimitiveTuple{
	"SpinImplementation": {&SpinMutex{}, &SpinWaitGroup{}},
	"Stdlib":             {&sync.Mutex{}, &sync.WaitGroup{}},
	"Gosched":            {&MutexWithPause{}, &sync.WaitGroup{}},
}

var tasks = map[string]func(mu MutexI, wg WaitGroupI, goroutines int, iterations int) int{
	"cpu": cpuTask,
	"io":  ioTask,
}

func main() {
	goroutines, err := strconv.Atoi(os.Getenv("GOROUTINES"))
	if err != nil {
		panic(err)
	}

	iterations, err := strconv.Atoi(os.Getenv("ITERATIONS"))
	if err != nil {
		panic(err)
	}

	testCase := os.Getenv("TESTCASE")
	if _, ok := implementations[testCase]; !ok {
		panic("unknown TESTCASE")
	}
	taskName := os.Getenv("TASK")
	if _, ok := tasks[taskName]; !ok {
		panic("unknown TASK")
	}
	test := implementations[testCase]
	task := tasks[taskName]

	result := task(test.mu, test.wg, goroutines, iterations)
	if result != goroutines*iterations {
		panic("result != goroutines*iterations")
	}
	fmt.Println(testCase, "GOMAXPROCS =", runtime.GOMAXPROCS(0), "passeed:", result == goroutines*iterations)
}

func cpuTask(mu MutexI, wg WaitGroupI, goroutines int, iterations int) int {
	var count int

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			for range iterations {
				mu.Lock()
				count++
				//time.Sleep(time.Microsecond)
				if count%1000 == 0 {
					//fmt.Println(count)
				}

				mu.Unlock()
			}
			wg.Done()
		}(i)
	}

	wg.Wait()

	return count
}

func ioTask(mu MutexI, wg WaitGroupI, goroutines int, iterations int) int {
	var count int

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			for range iterations {
				mu.Lock()

				count++
				time.Sleep(time.Millisecond)

				mu.Unlock()
			}
			wg.Done()
		}(i)
	}

	wg.Wait()

	return count
}
