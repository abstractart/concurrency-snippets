require 'async'

# Устанавливаем кастомный планировщик файберов
Fiber.set_scheduler(Async::Scheduler.new)

# Функция с паузой
def task_with_pause
  Fiber.schedule do
    puts "Задача 1: Ожидание 3 секунд... PID: #{Process.pid} | Thread ID: #{Thread.current.object_id}\n\n"
    sleep 3
    puts "Задача 1: Прошла пауза, выводим текст в консоль."
  end
end

# Бесконечная задача
def infinite_task
  Fiber.schedule do
    puts "Задача 2: Начало бесконечного цикла. PID: #{Process.pid} | Thread ID: #{Thread.current.object_id}\n\n"
    loop { }  # Позволяет переключаться между файберами
  end
end

# Основная функция
Fiber.schedule do
  task1 = task_with_pause
  task2 = infinite_task

  task1.transfer # Ожидание завершения первой задачи
  puts "Задача 1 завершена."

  # Файберы нельзя убивать, но после завершения основного потока программа остановится
  puts "Задача 2 была отменена."
end

# Запускаем планировщик
Fiber.scheduler.run
