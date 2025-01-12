package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

func main() {
	// Вывод значений установленных свойств
	fmt.Println("Параметры виртуальных потоков (Go Runtime):")
	fmt.Printf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("NumCPU: %d\n", runtime.NumCPU())
	fmt.Printf("NumGoroutine: %d\n", runtime.NumGoroutine())

	var wg sync.WaitGroup
	wg.Add(2)

	// Задача 1: Выполнение с паузой
	task1 := func() {
		defer wg.Done()

		fmt.Println("Задача 1: Ожидание 3 секунды...")
		time.Sleep(3 * time.Second)
		fmt.Println("Задача 1: Прошла пауза, выводим текст в консоль.")
	}

	// Задача 2: Бесконечный цикл
	task2 := func() {
		defer wg.Done()
		fmt.Println("Задача 2: Начало бесконечного цикла.")
		for {
			// fmt.Println("Задача 2: Работаем...")
			// time.Sleep(1 * time.Second)
		}
	}

	// Запуск задач в отдельных горутинах
	go task1()
	go task2()

	wg.Wait()

	fmt.Println("Прерывание выполнения задачи 2 невозможно в Go напрямую. Завершение программы через main.")
}
