package main

import (
	"fmt"
	"sync"
	"time"
)

const philosophers = 5

func main() {
	steward := make(chan int, philosophers-1)
	forks := make([]sync.Mutex, philosophers)

	for p := range philosophers {
		go philosopher(p, &forks[p], &forks[(p+1)%philosophers], steward)
	}

	quit := make(chan struct{})
	<-quit
}

func philosopher(p int, left *sync.Mutex, right *sync.Mutex, steward chan int) {
	for {

		steward <- p
		left.Lock()
		right.Lock()
		fmt.Println("Philosopher", p, "Eating...")
		time.Sleep(time.Millisecond)

		left.Unlock()
		right.Unlock()
		<-steward

		fmt.Println("Philosopher", p, "ate :)")
	}
}
