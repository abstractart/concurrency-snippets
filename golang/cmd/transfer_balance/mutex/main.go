package main

import (
	"fmt"

	"github.com/concurrency-examples/golang/internal/transfer_balance/mutex"
)

func main() {
	a := mutex.NewAccount(1000)
	b := mutex.NewAccount(0)
	fmt.Printf("before: a=%d, b=%d\n", a.Balance(), b.Balance())
	mutex.Transfer(a, b, 300)
	fmt.Printf("after:  a=%d, b=%d\n", a.Balance(), b.Balance())
}
