package main

type Node struct {
	val  int
	next *Node
}
type Stack struct {
	top *Node
}

func (s *Stack) Push(n *Node) {
	if s.top == nil {
		s.top = n
	} else {
		n.next = s.top
		s.top = n
	}
}

func (s *Stack) Pop() *Node {
	if s.top == nil {
		return nil
	}
	node := s.top
	s.top = node.next
	return node
}

func main() {
	s := &Stack{}
	s.Push(&Node{val: 1})
	s.Push(&Node{val: 2})
	s.Push(&Node{val: 3})

	println(s.Pop().val) // 3
	println(s.Pop().val) // 2
	println(s.Pop().val) // 1
	println(s.Pop())     // nil
}
