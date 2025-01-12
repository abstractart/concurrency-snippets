require 'async'

# Задача, которая делает паузу, а затем пишет текст в консоль.
def task_with_pause
  Async do
    puts "Задача 1: Ожидание 3 секунды... thread PID: #{Process.pid} thread ID: #{Thread.current.native_thread_id}\n\n"
    sleep 3
    puts "Задача 1: Прошла пауза, выводим текст в консоль."
  end
end

# Задача с бесконечным циклом.
def infinite_task
  Async do
    puts "Задача 2: Начало бесконечного цикла. thread PID: #{Process.pid} thread ID: #{Thread.current.native_thread_id}\n\n"
    loop do
      # puts "Задача 2: Работаем..."
      # sleep 1  # Исключение блокировки основного потока
    end
  end
end

# Основная функция для запуска задач.
Async do |task|
  # Запуск задачи с паузой
  task1 = task_with_pause

  # Запуск бесконечной задачи
  task2 = infinite_task

  # Дождаться завершения первой задачи
  task1.wait

  # Явная отмена второй задачи
  task2.stop
  puts "Задача 2 была отменена."
end
