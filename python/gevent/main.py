import gevent
from gevent import monkey
import time

# Патчинг стандартных библиотек для совместимости с gevent
monkey.patch_all()

def task_with_pause():
    """Задача с паузой перед выводом текста."""
    print("Задача 1: Ожидание 3 секунды...")
    gevent.sleep(3)
    print("Задача 1: Прошла пауза, выводим текст в консоль.")

def infinite_task():
    """Бесконечная задача."""
    print("Задача 2: Начало бесконечного цикла.")
    while True:
        pass
        # print("Задача 2: Работаем...")
        # gevent.sleep(1)  # Исключение блокировки

if __name__ == "__main__":
    # Создаем задачи (зелёные потоки)
    greenlet1 = gevent.spawn(task_with_pause)
    greenlet2 = gevent.spawn(infinite_task)

    # Ждем завершения первой задачи
    greenlet1.join()

    # Останавливаем вторую задачу
    greenlet2.kill()
    print("Задача 2 была завершена.")
