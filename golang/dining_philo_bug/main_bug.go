package main

import (
	"fmt"
	"sync"
)

func main() {
	const philosophers = 5
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
		left.Lock()
		right.Lock()

		fmt.Println("Philosopher", p, "Eating...")

		left.Unlock()
		right.Unlock()

		fmt.Println("Philosopher", p, "ate :)")
	}
}
