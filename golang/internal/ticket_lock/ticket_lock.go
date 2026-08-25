package ticket_lock

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
		runtime.Gosched()
	}
}

func (l *TicketLock) Unlock(my uint32) {
	atomic.AddUint32(&l.nowServing, 1)
}

func worker(id int, iterations int, lock *TicketLock, counter *int, wg *sync.WaitGroup) {
	defer wg.Done()
	for i := 0; i < iterations; i++ {
		myTicket := lock.Lock()
		fmt.Printf("[Worker %2d] acquired lock, ticket=%d, counter=%d\n", id, myTicket, *counter)
		*counter++
		time.Sleep(10 * time.Millisecond)
		fmt.Printf("[Worker %2d] done lock, ticket=%d\n", id, myTicket)
		lock.Unlock(myTicket)
		time.Sleep(time.Duration(id*5) * time.Millisecond)
	}
}

func Run() {
	var lock TicketLock
	var counter int
	var wg sync.WaitGroup
	numWorkers := 5
	iterations := 5

	wg.Add(numWorkers)
	for i := 1; i <= numWorkers; i++ {
		go worker(i, iterations, &lock, &counter, &wg)
	}

	wg.Wait()
	fmt.Println("Final counter =", counter)
}
