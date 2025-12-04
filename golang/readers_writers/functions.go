package readers_writers

import (
	"sync"
	"sync/atomic"
)

func Reader(m map[int]struct{}, mu RMMutexI) {
	mu.RLock()
	//_ = m[rand.Intn(1000)]
	mu.RUnlock()
}

func Writer(m map[int]struct{}, mu RMMutexI) {
	mu.Lock()
	//m[rand.Intn(1000)] = struct{}{}
	mu.Unlock()
}

const rwmutexMaxReaders = 1 << 30

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

type RMMutexI interface {
	RLock()
	RUnlock()
	Lock()
	Unlock()
}
type MyRWMutex struct {
	w           sync.Mutex
	readerSem   chan struct{}
	writerSem   chan struct{}
	readerCount atomic.Int32
	readerWait  atomic.Int32
}

type MyRWMutexWithSyncMutex struct {
	noCopy noCopy
	sync.Mutex
}

func (mu *MyRWMutexWithSyncMutex) RLock() {
	mu.Lock()
}

func (mu *MyRWMutexWithSyncMutex) RUnlock() {
	mu.Unlock()
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
