package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(3)

	go func() {
		mu.Lock()

		time.Sleep(10 * time.Second)
		wg.Done()
		fmt.Println("#1 done")
	}()

	go func() {
		time.Sleep(2 * time.Second)
		mu.Unlock()

		wg.Done()
		fmt.Println("#2 done")
	}()

	go func() {
		time.Sleep(3 * time.Second)
		mu.Lock()

		wg.Done()
		fmt.Println("#3 done")
	}()

	wg.Wait()
}
