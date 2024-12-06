package main

import (
	"fmt"
	"sync"
)

// GOMAXPROCS=2 go run main.go

func main() {
	x, y := 0, 0
	r1, r2 := 0, 0

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		x = 1
		r1 = y
	}()

	go func() {
		defer wg.Done()

		y = 1
		r2 = x
	}()

	wg.Wait()

	fmt.Printf("r1: %d, r2: %d\n", r1, r2)
}
