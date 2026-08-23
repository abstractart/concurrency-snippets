package main

import "sync/atomic"

func main() {
	var val atomic.Pointer[string]

	s := "Hello, World!"
	val.Store(&s)

}
