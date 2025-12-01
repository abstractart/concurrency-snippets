package main

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

const rwmutexMaxReaders = 1 << 20 // можно вернуть 1<<30, если нужно

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

type MyRWMutex struct {
	w           sync.Mutex
	readerSem   chan struct{}
	writerSem   chan struct{}
	readerCount atomic.Int32
	readerWait  atomic.Int32
}

func NewMyRWMutex() MyRWMutex {
	return MyRWMutex{
		readerSem: make(chan struct{}, 1_000_000), // большой буфер
		writerSem: make(chan struct{}, 1),
	}
}

func (mu *MyRWMutex) RLock() {
	if mu.readerCount.Add(1) < 0 {
		<-mu.readerSem
	}
}

func (mu *MyRWMutex) RUnlock() {
	if r := mu.readerCount.Add(-1); r < 0 {
		if mu.readerWait.Add(-1) == 0 {
			mu.writerSem <- struct{}{}
		}
	}
}

func (mu *MyRWMutex) Lock() {
	mu.w.Lock()
	r := mu.readerCount.Add(-rwmutexMaxReaders) + rwmutexMaxReaders
	if r != 0 {
		if mu.readerWait.Add(r) != 0 {
			<-mu.writerSem
		}
	}
}

func (mu *MyRWMutex) Unlock() {
	r := mu.readerCount.Add(rwmutexMaxReaders)
	for i := int32(0); i < r; i++ {
		mu.readerSem <- struct{}{}
	}
	mu.w.Unlock()
}

func main() {
	rand.Seed(time.Now().UnixNano())

	shared := make(map[int]struct{})
	mu := NewMyRWMutex()

	for i := 0; i < 4; i++ {
		go func() {
			reader(shared, &mu)
		}()
	}
	for i := 0; i < 2; i++ {
		go func() {
			writer(shared, &mu)
		}()
	}

	select {}
}

func reader(m map[int]struct{}, mu *MyRWMutex) {
	for {
		mu.RLock()
		_ = m[rand.Intn(1000)]
		mu.RUnlock()
		time.Sleep(time.Millisecond)
	}
}

func writer(m map[int]struct{}, mu *MyRWMutex) {
	for {
		mu.Lock()
		m[rand.Intn(1000)] = struct{}{}
		mu.Unlock()
		time.Sleep(time.Millisecond)
	}
}
