package mcas

// Эксперимент: демонстрация бесконечной рекурсии при отсутствии сортировки операций.
//
// Механизм цикла:
//
//   tx1: Transfer(a→b): firstOp=a, secondOp=b
//   tx2: Transfer(b→a): firstOp=b, secondOp=a
//
//   1. tx1.firstOp.tryClaim() — заявляет a, успех
//   2. tx2.firstOp.tryClaim() — заявляет b, успех
//   3. tx1.commit() → tx1.secondOp.prepare() — видит дескриптор tx2 на b → вызывает tx2.commit()
//   4.   tx2.secondOp.prepare() — видит дескриптор tx1 на a → вызывает tx1.commit()
//   5.     tx1.secondOp.prepare() — видит дескриптор tx2 на b → вызывает tx2.commit()
//   6.     ... ∞  →  stack overflow
//
// Гонка воспроизводится детерминированно: мы вручную ставим оба дескриптора
// в «опасную» позицию перед вызовом commit(), не полагаясь на планировщик.

import (
	"runtime/debug"
	"testing"
)

// TestTransferWithoutOrderingCycle детерминированно воспроизводит цикл helping-а.
// Ожидаемый результат: "fatal error: stack overflow"
//
// Запуск:
//
//	go test ./internal/transfer_balance/mcas/ -run TestTransferWithoutOrderingCycle -v
func TestTransferWithoutOrderingCycle(t *testing.T) {
	debug.SetMaxStack(512 * 1024) // 512KB: crash наступит быстро, не через секунды

	a := NewAccount(1000)
	b := NewAccount(1000)

	// tx1: a → b (без сортировки: firstOp=a, secondOp=b)
	tx1 := &tx{}
	tx1.firstOp = newOperation(a, tx1, -1)
	tx1.secondOp = newOperation(b, tx1, +1)

	// tx2: b → a (без сортировки: firstOp=b, secondOp=a)
	tx2 := &tx{}
	tx2.firstOp = newOperation(b, tx2, -1)
	tx2.secondOp = newOperation(a, tx2, +1)

	// Вручную ставим оба дескриптора в опасную позицию:
	// tx1 заявил a, tx2 заявил b — оба "в полёте" на своих первых ячейках.
	tx1.firstOp.tryClaim() // CAS a: clean_balance → tx1_marker
	tx2.firstOp.tryClaim() // CAS b: clean_balance → tx2_marker

	// Теперь вызываем commit: tx1 видит tx2 на b, помогает ему,
	// tx2 видит tx1 на a, помогает ему — цикл без выхода.
	tx1.commit()
}
