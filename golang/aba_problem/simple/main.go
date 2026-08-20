// Простейшая демонстрация ABA-проблемы на int.
//
// Ключевая идея: int должен быть *идентификатором*, а не количеством.
// Для арифметики (баланс, счётчик) ABA безвреден: +10 и +50/-50 коммутируют.
// А вот если значение — это ID операции / слота / сессии, и оно может быть
// переиспользовано, CAS успешно срабатывает на ЧУЖОЙ сущности.
//
// Сценарий — отмена операции по ID:
//
//   currentOp = 42        // в системе выполняется операция №42
//   T1: пользователь жмёт "Отменить #42"
//   T1: читает currentOp=42, готовится CAS(42, 0) — пауза
//   T2: операция 42 завершилась → currentOp = 0
//   T2: стартует новая операция, ID переиспользован → currentOp = 42
//   T1: CAS(42, 0) → УСПЕХ → отменили НЕ ТУ операцию.
//
// "Значение то же" ≠ "сущность та же". В этом и беда ABA.
package main

import (
	"fmt"
	"sync/atomic"
)

func main() {
	var currentOp atomic.Int32
	currentOp.Store(42) // выполняется операция №42

	t2Run := make(chan struct{})
	t2Done := make(chan struct{})
	done := make(chan struct{})

	// T1: пользователь нажал "Отменить операцию 42"
	go func() {
		myTarget := currentOp.Load()
		fmt.Printf("[T1] пользователь отменяет операцию #%d\n", myTarget)
		fmt.Println("[T1] засыпаю перед CAS (например, идёт RPC за подтверждением)...")

		t2Run <- struct{}{}
		<-t2Done

		ok := currentOp.CompareAndSwap(myTarget, 0)
		fmt.Printf("[T1] CAS(currentOp, %d, 0) = %v\n", myTarget, ok)
		fmt.Println("[T1] думаю: я отменил ту операцию, которую видел пользователь.")
		done <- struct{}{}
	}()

	// T2: жизненный цикл операций идёт своим чередом
	go func() {
		<-t2Run

		// Старая операция 42 успешно завершилась
		currentOp.Store(0)
		fmt.Printf("[T2] операция #42 завершилась успешно, currentOp = %d\n", currentOp.Load())

		// Стартует новая операция, аллокатор ID переиспользовал 42
		currentOp.Store(42)
		fmt.Printf("[T2] стартует НОВАЯ операция, ID переиспользован → currentOp = %d\n",
			currentOp.Load())

		t2Done <- struct{}{}
	}()

	<-done

	fmt.Printf("\nИтог: currentOp = %d\n", currentOp.Load())
	fmt.Println("ПРОБЛЕМА: T1 отменил операцию, которую НЕ заказывал отменять.")
	fmt.Println("          Старая #42 уже завершилась, а CAS убил НОВУЮ #42.")
	fmt.Println("          Значение совпало — сущность другая. Это ABA.")
	fmt.Println()
	fmt.Println("Фикс: версионировать ID (op_id, generation) и CAS-ить пару,")
	fmt.Println("      либо никогда не переиспользовать ID (монотонный счётчик).")
}
