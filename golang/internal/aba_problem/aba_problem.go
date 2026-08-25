// ABA Problem in lock-free programming.
//
// Classic scenario with a lock-free stack:
//
//	Initial: top → A → B → C
//
//	Thread 1: reads top=A, A.next=B  →  prepares CAS(top, A, B)  →  PAUSES
//	Thread 2: pop A, pop B, push A   →  stack becomes: top → A → C
//	Thread 1: resumes, CAS succeeds  →  top now points at stale B !
//
//	Result: stack = B → C  (A lost; B is a "zombie" node)
//
// Fix: wrap every pointer with a monotonic version counter.
// Even when the same node A returns to the top, the stamped pointer
// is a new allocation with a higher version → Thread 1's CAS fails.
package aba_problem

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

// ─────────────────────────────────────────────────────────────────────────────
// Shared generic node type
//
// A dummy sentinel node lives at the bottom of every stack. Real nodes whose
// "next" would otherwise be nil instead point at the sentinel — so Push/Pop
// never need to handle a nil pointer.
// ─────────────────────────────────────────────────────────────────────────────

type Node[T any] struct {
	val  T
	next atomic.Pointer[Node[T]]
}

type UnsafeStack[T any] struct {
	dummy *Node[T]
	top   atomic.Pointer[Node[T]]
}

func NewUnsafeStack[T any]() *UnsafeStack[T] {
	s := &UnsafeStack[T]{dummy: &Node[T]{}}
	s.top.Store(s.dummy)
	return s
}

func (s *UnsafeStack[T]) Push(n *Node[T]) {
	for {
		top := s.top.Load()
		n.next.Store(top)
		if s.top.CompareAndSwap(top, n) {
			return
		}
	}
}

func (s *UnsafeStack[T]) Pop() *Node[T] {
	for {
		top := s.top.Load()
		if top == s.dummy {
			return nil
		}
		if s.top.CompareAndSwap(top, top.next.Load()) {
			return top
		}
	}
}

func (s *UnsafeStack[T]) Values() []T {
	var out []T
	for n := s.top.Load(); n != s.dummy; n = n.next.Load() {
		out = append(out, n.val)
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// PART 2 — ABA-safe stack: stamped (versioned) pointer
// ─────────────────────────────────────────────────────────────────────────────

type stampedRef[T any] struct {
	node    *Node[T]
	version uint64
}

type SafeStack[T any] struct {
	dummy *Node[T]
	top   atomic.Pointer[stampedRef[T]]
	ver   atomic.Uint64
}

func NewSafeStack[T any]() *SafeStack[T] {
	s := &SafeStack[T]{dummy: &Node[T]{}}
	s.top.Store(&stampedRef[T]{node: s.dummy})
	return s
}

func (s *SafeStack[T]) stamp(n *Node[T]) *stampedRef[T] {
	return &stampedRef[T]{node: n, version: s.ver.Add(1)}
}

func (s *SafeStack[T]) Push(n *Node[T]) {
	for {
		old := s.top.Load()
		n.next.Store(old.node)
		if s.top.CompareAndSwap(old, s.stamp(n)) {
			return
		}
	}
}

func (s *SafeStack[T]) Pop() *Node[T] {
	for {
		old := s.top.Load()
		if old.node == s.dummy {
			return nil
		}
		if s.top.CompareAndSwap(old, s.stamp(old.node.next.Load())) {
			return old.node
		}
	}
}

func (s *SafeStack[T]) Values() []T {
	var out []T
	for n := s.top.Load().node; n != s.dummy; n = n.next.Load() {
		out = append(out, n.val)
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// PART 3 — ABA-safe stack БЕЗ счётчика версий
// ─────────────────────────────────────────────────────────────────────────────

type ptrRef[T any] struct {
	node *Node[T]
}

type PointerStack[T any] struct {
	dummy *Node[T]
	top   atomic.Pointer[ptrRef[T]]
}

func NewPointerStack[T any]() *PointerStack[T] {
	s := &PointerStack[T]{dummy: &Node[T]{}}
	s.top.Store(&ptrRef[T]{node: s.dummy})
	return s
}

func (s *PointerStack[T]) Push(n *Node[T]) {
	for {
		old := s.top.Load()
		n.next.Store(old.node)
		if s.top.CompareAndSwap(old, &ptrRef[T]{node: n}) {
			return
		}
	}
}

func (s *PointerStack[T]) Pop() *Node[T] {
	for {
		old := s.top.Load()
		if old.node == s.dummy {
			return nil
		}
		if s.top.CompareAndSwap(old, &ptrRef[T]{node: old.node.next.Load()}) {
			return old.node
		}
	}
}

func (s *PointerStack[T]) Values() []T {
	var out []T
	for n := s.top.Load().node; n != s.dummy; n = n.next.Load() {
		out = append(out, n.val)
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// PART 4 — версия живёт в самом узле, top упакован в atomic.Uint64
// ─────────────────────────────────────────────────────────────────────────────

type VersionedNode struct {
	val     int
	next    atomic.Pointer[VersionedNode]
	version atomic.Uint64
}

type TaggedStack struct {
	dummy *VersionedNode
	top   atomic.Uint64
}

const versionShift = 48
const ptrMask = (uint64(1) << versionShift) - 1

func pack(n *VersionedNode, ver uint64) uint64 {
	return uint64(uintptr(unsafe.Pointer(n))) | (ver << versionShift)
}

func unpack(v uint64) (*VersionedNode, uint64) {
	ptr := unsafe.Add(unsafe.Pointer((*VersionedNode)(nil)), v&ptrMask)
	return (*VersionedNode)(ptr), v >> versionShift
}

func NewTaggedStack() *TaggedStack {
	s := &TaggedStack{dummy: &VersionedNode{}}
	s.top.Store(pack(s.dummy, 0))
	return s
}

func (s *TaggedStack) Push(n *VersionedNode) {
	newVer := n.version.Add(1)
	for {
		old := s.top.Load()
		oldNode, _ := unpack(old)
		n.next.Store(oldNode)
		if s.top.CompareAndSwap(old, pack(n, newVer)) {
			return
		}
	}
}

func (s *TaggedStack) Pop() *VersionedNode {
	for {
		old := s.top.Load()
		node, _ := unpack(old)
		if node == s.dummy {
			return nil
		}
		nxt := node.next.Load()
		if s.top.CompareAndSwap(old, pack(nxt, nxt.version.Load())) {
			return node
		}
	}
}

func (s *TaggedStack) Values() []int {
	var out []int
	cur, _ := unpack(s.top.Load())
	for cur != s.dummy {
		out = append(out, cur.val)
		cur = cur.next.Load()
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// Demonstrations
// ─────────────────────────────────────────────────────────────────────────────

func demonstrateABA() {
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  ЧАСТЬ 1: ABA-уязвимый стек")
	fmt.Println("═══════════════════════════════════════════════════")

	nodeA := &Node[int]{val: 1}
	nodeB := &Node[int]{val: 2}
	nodeC := &Node[int]{val: 3}

	s := NewUnsafeStack[int]()
	s.Push(nodeC)
	s.Push(nodeB)
	s.Push(nodeA)
	fmt.Printf("Начальный стек:  %v  (top→A→B→C)\n\n", s.Values())

	t2Run := make(chan struct{})
	t2Done := make(chan struct{})
	done := make(chan struct{})

	go func() {
		savedTop := s.top.Load()
		savedNext := nodeA.next.Load()
		fmt.Printf("[T1] прочитал: top=A(%d), A.next=B(%d)\n", savedTop.val, savedNext.val)
		fmt.Println("[T1] засыпает перед CAS...")
		t2Run <- struct{}{}
		<-t2Done
		ok := s.top.CompareAndSwap(savedTop, savedNext)
		fmt.Printf("[T1] CAS(top, A, B) → успех=%v  ← ABA!\n\n", ok)
		done <- struct{}{}
	}()

	go func() {
		<-t2Run
		pa := s.Pop()
		fmt.Printf("[T2] pop → A(%d), A-> next(%d),  стек: %v\n", pa.val, pa.next.Load().val, s.Values())
		pb := s.Pop()
		fmt.Printf("[T2] pop → B(%d), B-> next(%d),  стек: %v\n", pb.val, pb.next.Load().val, s.Values())
		_ = pb
		s.Push(pa)
		fmt.Printf("[T2] push A(%d),    стек: %v  (top→A→C)\n\n", pa.val, s.Values())
		t2Done <- struct{}{}
	}()

	<-done

	top := s.top.Load()
	fmt.Printf("Итоговый стек:   %v\n", s.Values())
	fmt.Printf("top.val = %d", top.val)
	if next := top.next.Load(); next != s.dummy {
		fmt.Printf(",  top.next.val = %d  ← зомби-узел B!", next.val)
	}
	fmt.Println()
	fmt.Println()
	fmt.Println("ПРОБЛЕМА: T1 установил top=B, хотя B был удалён T2.")
	fmt.Println("          A потерян несмотря на то что был возвращён в стек.")
}

func demonstrateSafe() {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  ЧАСТЬ 2: ABA-safe стек (версионированный указатель)")
	fmt.Println("═══════════════════════════════════════════════════")

	nodeA := &Node[int]{val: 1}
	nodeB := &Node[int]{val: 2}
	nodeC := &Node[int]{val: 3}

	s := NewSafeStack[int]()
	s.Push(nodeC)
	s.Push(nodeB)
	s.Push(nodeA)
	fmt.Printf("Начальный стек:  %v  (top→A→B→C)\n\n", s.Values())

	t2Run := make(chan struct{})
	t2Done := make(chan struct{})
	done := make(chan struct{})

	go func() {
		savedStamp := s.top.Load()
		fmt.Printf("[T1] прочитал stamp={val=%d, ver=%d, ptr=%p}\n",
			savedStamp.node.val, savedStamp.version, savedStamp)
		fmt.Println("[T1] засыпает перед CAS...")
		t2Run <- struct{}{}
		<-t2Done
		curStamp := s.top.Load()
		fmt.Printf("[T1] resuming:  current stamp={val=%d, ver=%d, ptr=%p}\n",
			curStamp.node.val, curStamp.version, curStamp)
		fmt.Printf("[T1] saved ptr=%p  ==  current ptr=%p  →  %v\n",
			savedStamp, curStamp, savedStamp == curStamp)
		newRef := &stampedRef[int]{node: nodeA.next.Load(), version: s.ver.Load() + 1}
		ok := s.top.CompareAndSwap(savedStamp, newRef)
		fmt.Printf("[T1] CAS(top, old_stamp, B) → успех=%v  ← ABA предотвращён!\n\n", ok)
		done <- struct{}{}
	}()

	go func() {
		<-t2Run
		pa := s.Pop()
		fmt.Printf("[T2] pop → A(%d),  стек: %v\n", pa.val, s.Values())
		pb := s.Pop()
		fmt.Printf("[T2] pop → B(%d),  стек: %v\n", pb.val, s.Values())
		_ = pb
		s.Push(pa)
		fmt.Printf("[T2] push A(%d),    стек: %v\n\n", pa.val, s.Values())
		t2Done <- struct{}{}
	}()

	<-done
	fmt.Printf("Итоговый стек:   %v\n", s.Values())
	fmt.Println("Ожидалось:       [1 3]  ✓")
	_, _ = nodeB, nodeC
}

func demonstratePointerSafe() {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  ЧАСТЬ 3: ABA-safe стек БЕЗ счётчика версий")
	fmt.Println("═══════════════════════════════════════════════════")

	nodeA := &Node[int]{val: 1}
	nodeB := &Node[int]{val: 2}
	nodeC := &Node[int]{val: 3}

	s := NewPointerStack[int]()
	s.Push(nodeC)
	s.Push(nodeB)
	s.Push(nodeA)
	fmt.Printf("Начальный стек:  %v  (top→A→B→C)\n\n", s.Values())

	t2Run := make(chan struct{})
	t2Done := make(chan struct{})
	done := make(chan struct{})

	go func() {
		savedRef := s.top.Load()
		fmt.Printf("[T1] прочитал ref={val=%d, ptr=%p}\n", savedRef.node.val, savedRef)
		fmt.Println("[T1] засыпает перед CAS...")
		t2Run <- struct{}{}
		<-t2Done
		curRef := s.top.Load()
		fmt.Printf("[T1] resumed: current ref={val=%d, ptr=%p}\n", curRef.node.val, curRef)
		fmt.Printf("[T1] saved ptr=%p  ==  current ptr=%p  →  %v\n",
			savedRef, curRef, savedRef == curRef)
		newRef := &ptrRef[int]{node: nodeA.next.Load()}
		ok := s.top.CompareAndSwap(savedRef, newRef)
		fmt.Printf("[T1] CAS(top, old_ref, B) → успех=%v  ← ABA предотвращён без version!\n\n", ok)
		done <- struct{}{}
	}()

	go func() {
		<-t2Run
		pa := s.Pop()
		fmt.Printf("[T2] pop → A(%d),  стек: %v\n", pa.val, s.Values())
		pb := s.Pop()
		fmt.Printf("[T2] pop → B(%d),  стек: %v\n", pb.val, s.Values())
		_ = pb
		s.Push(pa)
		fmt.Printf("[T2] push A(%d),    стек: %v\n\n", pa.val, s.Values())
		t2Done <- struct{}{}
	}()

	<-done
	fmt.Printf("Итоговый стек:   %v\n", s.Values())
	fmt.Println("Ожидалось:       [1 3]  ✓")
	_, _ = nodeB, nodeC
}

func demonstrateTaggedStack() {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  ЧАСТЬ 4: версия в узле, top упакован в uint64")
	fmt.Println("═══════════════════════════════════════════════════")

	nodeA := &VersionedNode{val: 1}
	nodeB := &VersionedNode{val: 2}
	nodeC := &VersionedNode{val: 3}

	s := NewTaggedStack()
	s.Push(nodeC)
	s.Push(nodeB)
	s.Push(nodeA)
	fmt.Printf("Начальный стек:  %v  (top→A→B→C)\n", s.Values())
	fmt.Printf("A.version=%d, B.version=%d, C.version=%d\n\n",
		nodeA.version.Load(), nodeB.version.Load(), nodeC.version.Load())

	t2Run := make(chan struct{})
	t2Done := make(chan struct{})
	done := make(chan struct{})

	go func() {
		savedTop := s.top.Load()
		savedNode, savedVer := unpack(savedTop)
		fmt.Printf("[T1] прочитал top: node=%d, ver=%d (packed=0x%x)\n",
			savedNode.val, savedVer, savedTop)
		fmt.Println("[T1] засыпает перед CAS...")
		t2Run <- struct{}{}
		<-t2Done
		curTop := s.top.Load()
		curNode, curVer := unpack(curTop)
		fmt.Printf("[T1] resumed: top: node=%d, ver=%d (packed=0x%x)\n",
			curNode.val, curVer, curTop)
		fmt.Printf("[T1] saved=0x%x  ==  current=0x%x  →  %v\n",
			savedTop, curTop, savedTop == curTop)
		nxt := nodeA.next.Load()
		newPacked := pack(nxt, nxt.version.Load())
		ok := s.top.CompareAndSwap(savedTop, newPacked)
		fmt.Printf("[T1] CAS → успех=%v  ← ABA предотвращён (версия A теперь другая)\n\n", ok)
		done <- struct{}{}
	}()

	go func() {
		<-t2Run
		pa := s.Pop()
		fmt.Printf("[T2] pop → A(%d, ver=%d),  стек: %v\n",
			pa.val, pa.version.Load(), s.Values())
		pb := s.Pop()
		fmt.Printf("[T2] pop → B(%d, ver=%d),  стек: %v\n",
			pb.val, pb.version.Load(), s.Values())
		_ = pb
		s.Push(pa)
		fmt.Printf("[T2] push A(%d, ver=%d),    стек: %v\n\n",
			pa.val, pa.version.Load(), s.Values())
		t2Done <- struct{}{}
	}()

	<-done
	fmt.Printf("Итоговый стек:   %v\n", s.Values())
	fmt.Println("Ожидалось:       [1 3]  ✓")
	_, _ = nodeB, nodeC
}

func Run() {
	demonstrateABA()
	demonstrateSafe()
	demonstratePointerSafe()
	demonstrateTaggedStack()
}
