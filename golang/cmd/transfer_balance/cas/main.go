package main

import (
	"fmt"

	"github.com/concurrency-examples/golang/internal/transfer_balance/cas"
)

func main() {
	a := cas.NewAccount(1000)
	b := cas.NewAccount(0)
	fmt.Printf("before: a=%d, b=%d\n", a.Balance(), b.Balance())
	cas.Transfer(a, b, 300)
	fmt.Printf("after:  a=%d, b=%d\n", a.Balance(), b.Balance())
}
