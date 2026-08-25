package gomaxprocs

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

func Run() {
	fmt.Println("Параметры виртуальных потоков (Go Runtime):")
	fmt.Printf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("NumCPU: %d\n", runtime.NumCPU())
	fmt.Printf("NumGoroutine: %d\n", runtime.NumGoroutine())

	var wg sync.WaitGroup
	wg.Add(2)

	task1 := func() {
		defer wg.Done()
		fmt.Println("Задача 1: Ожидание 3 секунды...")
		time.Sleep(3 * time.Second)
		fmt.Println("Задача 1: Прошла пауза, выводим текст в консоль.")
	}

	task2 := func() {
		defer wg.Done()
		fmt.Println("Задача 2: Начало бесконечного цикла.")
		for {
		}
	}

	go task1()
	go task2()

	wg.Wait()

	fmt.Println("Прерывание выполнения задачи 2 невозможно в Go напрямую. Завершение программы через main.")
}
