package threadsafestack

import (
	"sync"
	"time"
)

type Node struct {
	Val  int
	next *Node
}

type ThreadSafeStack struct {
	top *Node
	mu  sync.Mutex
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

func Run() {
	s := &ThreadSafeStack{}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		for i := range 50000 {
			s.Push(&Node{Val: i})
		}
		wg.Done()
	}()

	go func() {
		for i := 50000; i < 1000000; i++ {
			s.Push(&Node{Val: i})
		}
		wg.Done()
	}()

	go func() {
		count := 0
		time.Sleep(100 * time.Millisecond)
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
