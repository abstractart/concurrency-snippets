package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type TicketLock struct {
	next       uint32
	nowServing uint32
}

func (l *TicketLock) Lock() uint32 {
	my := atomic.AddUint32(&l.next, 1) - 1
	for {
		if atomic.LoadUint32(&l.nowServing) == my {
			return my
		}
		// уступаем планировщику, чтобы не крутить CPU
		runtime.Gosched()
	}
}

func (l *TicketLock) Unlock(my uint32) {
	atomic.AddUint32(&l.nowServing, 1)
}

var lock TicketLock
var counter int

func worker(id int, iterations int, wg *sync.WaitGroup) {
	defer wg.Done()
	for i := 0; i < iterations; i++ {
		myTicket := lock.Lock()
		fmt.Printf("[Worker %2d] acquired lock, ticket=%d, counter=%d\n", id, myTicket, counter)
		counter++
		time.Sleep(10 * time.Millisecond) // имитация работы в критической секции
		fmt.Printf("[Worker %2d] done lock, ticket=%d\n", id, myTicket)
		lock.Unlock(myTicket)
		// небольшая задержка, чтобы goroutine "перемешивались"
		time.Sleep(time.Duration(id*5) * time.Millisecond)
	}
}

func main() {
	var wg sync.WaitGroup
	numWorkers := 5
	iterations := 5

	wg.Add(numWorkers)
	for i := 1; i <= numWorkers; i++ {
		go worker(i, iterations, &wg)
	}

	wg.Wait()
	fmt.Println("Final counter =", counter)
}
