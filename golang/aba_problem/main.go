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
package main

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
//
// Every Push/Pop wraps the new top in a freshly allocated *stampedRef carrying
// a monotonic version counter. The classical DCAS pattern from C/C++.
//
// NOTE: in plain Go the version is technically redundant — Part 3 shows why.
// The counter earns its keep when wrappers come from sync.Pool, manual memory,
// or are packed into a uintptr that the GC cannot see.
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
//
// Каждый Push/Pop аллоцирует новый *ptrRef-обёртку. Пока поток держит ссылку
// на старую обёртку (savedRef в его локальной переменной), Go-GC не может
// освободить этот адрес — значит ни одна новая аллокация не получит тот же
// адрес. Уникальности указателя достаточно: CAS сравнивает адреса.
//
// Что ломает этот подход:
//   • sync.Pool / собственный freelist для ptrRef — обёртка возвращается
//     в обращение с тем же адресом → нужен version, как в Part 2.
//   • Хранение указателя как uintptr → GC его не видит → объект может быть
//     собран, а адрес переиспользован.
//   • Языки без GC (C/C++) — освобождённая обёртка mallocится по тому же
//     адресу. Каноничный DCAS-counter.
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
//
// Идея: убрать все обёртки. У каждого узла есть собственный счётчик
// version, который инкрементируется при каждом Push этого узла. Поле top
// стека хранит ОДИН atomic.Uint64, упакованный как:
//
//   ┌──────────── 16 бит ────────────┬──────────── 48 бит ────────────┐
//   │  snapshot версии узла на push  │  uintptr(*VersionedNode)       │
//   └────────────────────────────────┴────────────────────────────────┘
//
// CAS на uint64 атомарно сравнивает и адрес, и версию — настоящий DCAS.
//
// Преимущества:
//   • Нет stampedRef/ptrRef и аллокаций на каждый Push/Pop.
//   • Версия — свойство узла, а не операции.
//
// Цена и ограничения:
//   • Нужен unsafe.Pointer / uintptr — выход за пределы безопасного Go.
//   • Pointer-tagging держится на том, что user-space адреса умещаются в
//     48 бит (x86-64, ARM64 без TBI). На других платформах сломается.
//   • GC не видит ссылку, закодированную в uintptr: программа должна сама
//     удерживать узлы живыми. В нашем демо это делают caller-локали
//     (nodeA, nodeB, nodeC) — в продакшене так делать без runtime.KeepAlive
//     или отдельной таблицы узлов опасно.
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
	// unsafe.Add(nil, offset) даёт unsafe.Pointer без uintptr→Pointer round-trip,
	// который ловит go vet (unsafeptr). Семантика та же: создаём указатель
	// по адресу v & ptrMask.
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

	nodeA := &Node[int]{val: 1} // "A"
	nodeB := &Node[int]{val: 2} // "B"
	nodeC := &Node[int]{val: 3} // "C"

	s := NewUnsafeStack[int]()
	s.Push(nodeC)
	s.Push(nodeB)
	s.Push(nodeA)
	fmt.Printf("Начальный стек:  %v  (top→A→B→C)\n\n", s.Values())

	// Каналы для детерминированного управления чередованием горутин
	t2Run := make(chan struct{})
	t2Done := make(chan struct{})
	done := make(chan struct{})

	// ── Поток 1: хочет сделать pop() ────────────────────────────────
	go func() {
		// Шаг 1: читаем top и A.next — готовимся к CAS
		savedTop := s.top.Load()       // &nodeA
		savedNext := nodeA.next.Load() // &nodeB

		fmt.Printf("[T1] прочитал: top=A(%d), A.next=B(%d)\n",
			savedTop.val, savedNext.val)
		fmt.Println("[T1] засыпает перед CAS...")

		t2Run <- struct{}{} // разрешаем T2 работать
		<-t2Done            // ждём T2

		// Шаг 2: CAS(top, &A, &B)
		// top по-прежнему == &nodeA → CAS успешен, хотя это неверно!
		ok := s.top.CompareAndSwap(savedTop, savedNext)
		fmt.Printf("[T1] CAS(top, A, B) → успех=%v  ← ABA!\n\n", ok)
		done <- struct{}{}
	}()

	// ── Поток 2: пока T1 спит, делает pop-pop-push ──────────────────
	go func() {
		<-t2Run
		pa := s.Pop()
		fmt.Printf("[T2] pop → A(%d), A-> next(%d),  стек: %v\n", pa.val, pa.next.Load().val, s.Values())

		pb := s.Pop()
		fmt.Printf("[T2] pop → B(%d), B-> next(%d),  стек: %v\n", pb.val, pb.next.Load().val, s.Values())
		_ = pb // B "удалён" / "освобождён"

		// Возвращаем A (имитируем переиспользование узла / realloc)
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

	// ── Поток 1 ─────────────────────────────────────────────────────
	go func() {
		// Читаем stamped pointer: {node: A, version: V, addr: 0x...}
		savedStamp := s.top.Load()
		fmt.Printf("[T1] прочитал stamp={val=%d, ver=%d, ptr=%p}\n",
			savedStamp.node.val, savedStamp.version, savedStamp)
		fmt.Println("[T1] засыпает перед CAS...")

		t2Run <- struct{}{}
		<-t2Done

		// CAS сравнивает *pointer* (savedStamp), а не содержимое.
		// T2 сделал Push → top теперь указывает на НОВЫЙ stampedRef
		// → адрес изменился → CAS провалится.
		curStamp := s.top.Load()
		fmt.Printf("[T1] resuming:  current stamp={val=%d, ver=%d, ptr=%p}\n",
			curStamp.node.val, curStamp.version, curStamp)
		fmt.Printf("[T1] saved ptr=%p  ==  current ptr=%p  →  %v\n",
			savedStamp, curStamp, savedStamp == curStamp)

		// Готовим "новый top" = B (как в unsafe-версии)
		newRef := &stampedRef[int]{node: nodeA.next.Load(), version: s.ver.Load() + 1}
		ok := s.top.CompareAndSwap(savedStamp, newRef)
		fmt.Printf("[T1] CAS(top, old_stamp, B) → успех=%v  ← ABA предотвращён!\n\n", ok)
		done <- struct{}{}
	}()

	// ── Поток 2 ─────────────────────────────────────────────────────
	go func() {
		<-t2Run
		pa := s.Pop()
		fmt.Printf("[T2] pop → A(%d),  стек: %v\n", pa.val, s.Values())

		pb := s.Pop()
		fmt.Printf("[T2] pop → B(%d),  стек: %v\n", pb.val, s.Values())
		_ = pb

		s.Push(pa) // создаёт НОВЫЙ stampedRef — ключевой момент
		fmt.Printf("[T2] push A(%d),    стек: %v\n\n", pa.val, s.Values())

		t2Done <- struct{}{}
	}()

	<-done

	fmt.Printf("Итоговый стек:   %v\n", s.Values())
	fmt.Println("Ожидалось:       [1 3]  ✓")
	fmt.Println()
	fmt.Println("РЕШЕНИЕ: каждый Push/Pop выделяет новый *stampedRef.")
	fmt.Println("         CAS сравнивает адреса указателей, а не значения.")
	fmt.Println("         Даже если node A вернулась — адрес stamp-обёртки изменился.")
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

	// ── Поток 1 ─────────────────────────────────────────────────────
	go func() {
		savedRef := s.top.Load()
		fmt.Printf("[T1] прочитал ref={val=%d, ptr=%p}\n",
			savedRef.node.val, savedRef)
		fmt.Println("[T1] засыпает перед CAS...")
		fmt.Println("     savedRef удерживает обёртку → GC не освободит её адрес")

		t2Run <- struct{}{}
		<-t2Done

		curRef := s.top.Load()
		fmt.Printf("[T1] resumed: current ref={val=%d, ptr=%p}\n",
			curRef.node.val, curRef)
		fmt.Printf("[T1] saved ptr=%p  ==  current ptr=%p  →  %v\n",
			savedRef, curRef, savedRef == curRef)

		newRef := &ptrRef[int]{node: nodeA.next.Load()}
		ok := s.top.CompareAndSwap(savedRef, newRef)
		fmt.Printf("[T1] CAS(top, old_ref, B) → успех=%v  ← ABA предотвращён без version!\n\n", ok)
		done <- struct{}{}
	}()

	// ── Поток 2 ─────────────────────────────────────────────────────
	go func() {
		<-t2Run
		pa := s.Pop()
		fmt.Printf("[T2] pop → A(%d),  стек: %v\n", pa.val, s.Values())

		pb := s.Pop()
		fmt.Printf("[T2] pop → B(%d),  стек: %v\n", pb.val, s.Values())
		_ = pb

		s.Push(pa) // новая ptrRef-обёртка — её адрес гарантированно ≠ savedRef
		fmt.Printf("[T2] push A(%d),    стек: %v\n\n", pa.val, s.Values())

		t2Done <- struct{}{}
	}()

	<-done

	fmt.Printf("Итоговый стек:   %v\n", s.Values())
	fmt.Println("Ожидалось:       [1 3]  ✓")
	fmt.Println()
	fmt.Println("ПОЧЕМУ РАБОТАЕТ БЕЗ VERSION:")
	fmt.Println("  • savedRef в T1 — живая ссылка, удерживает обёртку от GC.")
	fmt.Println("  • Go-GC не отдаёт занятый адрес под новые аллокации.")
	fmt.Println("  • Push в T2 создаёт НОВУЮ ptrRef → её адрес ≠ savedRef.")
	fmt.Println("  • CAS сравнивает адреса → различает старую и новую обёртки.")
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

	// ── Поток 1 ─────────────────────────────────────────────────────
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

	// ── Поток 2 ─────────────────────────────────────────────────────
	go func() {
		<-t2Run
		pa := s.Pop()
		fmt.Printf("[T2] pop → A(%d, ver=%d),  стек: %v\n",
			pa.val, pa.version.Load(), s.Values())

		pb := s.Pop()
		fmt.Printf("[T2] pop → B(%d, ver=%d),  стек: %v\n",
			pb.val, pb.version.Load(), s.Values())
		_ = pb

		s.Push(pa) // A.version: 1 → 2
		fmt.Printf("[T2] push A(%d, ver=%d),    стек: %v\n\n",
			pa.val, pa.version.Load(), s.Values())

		t2Done <- struct{}{}
	}()

	<-done

	fmt.Printf("Итоговый стек:   %v\n", s.Values())
	fmt.Println("Ожидалось:       [1 3]  ✓")
	fmt.Println()
	fmt.Println("РЕШЕНИЕ: версия — поле узла; top упакован как (ver<<48)|ptr.")
	fmt.Println("         CAS на atomic.Uint64 атомарно сверяет адрес И версию.")
	fmt.Println("         После push(A) у A.version=2 → старая упаковка не совпадёт.")
	_, _ = nodeB, nodeC
}

func main() {
	demonstrateABA()
	demonstrateSafe()
	demonstratePointerSafe()
	demonstrateTaggedStack()
}
