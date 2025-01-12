import asyncio
import os
import threading

async def task_with_pause():
    """Задача, которая делает паузу, а затем пишет текст в консоль."""
    print(f"Задача 1: Ожидание 3 секунды... Thread pid: {os.getpid()}  Thread id: {threading.get_native_id()}")
    await asyncio.sleep(3)
    print("Задача 1: Прошла пауза, выводим текст в консоль.")

async def infinite_task():
    """Задача с бесконечным циклом."""
    print(f"Задача 2: Начало бесконечного цикла. Thread pid: {os.getpid()}  Thread id: {threading.get_native_id()}")

    while True:
        pass
        # await asyncio.sleep(1)  # Исключение блокировки основного потока

async def main():
    """Основная функция для запуска задач."""
    # Создаем две задачи
    task1 = asyncio.create_task(task_with_pause())
    task2 = asyncio.create_task(infinite_task())

    # Дожидаемся выполнения первой задачи, вторая продолжает выполняться
    await task1

    # Можно явно отменить вторую задачу, если это нужно
    task2.cancel()
    try:
        await task2
    except asyncio.CancelledError:
        print("Задача 2 была отменена.")

# Запуск асинхронного кода
if __name__ == "__main__":
    asyncio.run(main())
