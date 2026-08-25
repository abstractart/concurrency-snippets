package stack

type Node struct {
	Val  int
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

func Run() {
	s := &Stack{}
	s.Push(&Node{Val: 1})
	s.Push(&Node{Val: 2})
	s.Push(&Node{Val: 3})

	println(s.Pop().Val) // 3
	println(s.Pop().Val) // 2
	println(s.Pop().Val) // 1
	println(s.Pop())     // nil
}
