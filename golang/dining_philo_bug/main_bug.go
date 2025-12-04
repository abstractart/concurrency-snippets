package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

const philosophers = 5

func main() {
	forks := make([]sync.Mutex, philosophers)

	var wg sync.WaitGroup
	wg.Add(philosophers)

	for p := range philosophers {
		go philosopher(p, &forks[p], &forks[(p+1)%philosophers])
	}

	wg.Wait()
}

func philosopher(p int, left *sync.Mutex, right *sync.Mutex) {
	for {
		if rand.Intn(philosophers) == p {
			left.Lock()
			right.Lock()
		} else {
			right.Lock()
			left.Lock()
		}

		fmt.Println("Philosopher", p, "Eating...")
		time.Sleep(time.Second)
		left.Unlock()
		right.Unlock()

		fmt.Println("Philosopher", p, "ate :)")
	}
}
