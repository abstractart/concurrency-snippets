package main

import (
	"fmt"
	"time"
)

type node struct {
	value int
	next  *node
}

func generate() *node {
	root := &node{
		value: 200,
	}

	prev := root

	for i := 0; i < 10; i++ {
		n := node{
			value: 300 + i,
		}
		prev.next = &n
		prev = &n
		time.Sleep(time.Millisecond * 100)
	}

	return root
}

func main() {
	var n1, n2 *node
	ch := make(chan struct{})

	go func() {
		n1 = generate()
		ch <- struct{}{}
	}()

	go func() {
		n2 = generate()
		ch <- struct{}{}
	}()

	<-ch
	<-ch

	root := &node{
		value: 100,
		next:  n1,
	}

	fmt.Println("root")
	printNodes(root)

	fmt.Println("n2")
	printNodes(n2)

}

func printNodes(n *node) {
	for n != nil {
		fmt.Printf("val: %d, memory: %p\n", n.value, n)
		n = n.next
	}
}
