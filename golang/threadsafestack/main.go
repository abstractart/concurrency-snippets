package main

import (
	"sync"
	"time"
)

type ThreadSafeStack struct {
	top *Node
	mu  sync.Mutex
}

type Node struct {
	val  int
	next *Node
}

func (s *ThreadSafeStack) Push(n *Node) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.top == nil {
		s.top = n
	} else {
		n.next = s.top
		s.top = n
	}
}

func (s *ThreadSafeStack) Pop() *Node {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.top == nil {
		return nil
	}
	node := s.top
	s.top = node.next
	return node
}

func main() {
	s := &ThreadSafeStack{}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		for i := range 50000 {
			s.Push(&Node{val: i})
		}
		wg.Done()
	}()

	go func() {
		for i := 50000; i < 1000000; i++ {
			s.Push(&Node{val: i})
		}
		wg.Done()
	}()

	go func() {
		count := 0
		time.Sleep(100 * time.Millisecond) // wait for some pushes to happen
		for {
			if v := s.Pop(); v != nil {
				count++
			} else {
				break
			}
		}
		wg.Done()
		print(count)
	}()
	wg.Wait()
}
