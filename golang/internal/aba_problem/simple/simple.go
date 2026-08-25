// Простейшая демонстрация ABA-проблемы на int.
//
// Ключевая идея: int должен быть *идентификатором*, а не количеством.
// Для арифметики (баланс, счётчик) ABA безвреден: +10 и +50/-50 коммутируют.
// А вот если значение — это ID операции / слота / сессии, и оно может быть
// переиспользовано, CAS успешно срабатывает на ЧУЖОЙ сущности.
package simple

import (
	"fmt"
	"sync/atomic"
)

func Run() {
	var currentOp atomic.Int32
	currentOp.Store(42)

	t2Run := make(chan struct{})
	t2Done := make(chan struct{})
	done := make(chan struct{})

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

	go func() {
		<-t2Run
		currentOp.Store(0)
		fmt.Printf("[T2] операция #42 завершилась успешно, currentOp = %d\n", currentOp.Load())
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
