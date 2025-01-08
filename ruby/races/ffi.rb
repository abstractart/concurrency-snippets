# Пример 2: Data Race при вызове C-функции через FFI
require 'ffi'

# Описание библиотеки FFI
module MyLib
  extend FFI::Library
  ffi_lib 'libc'

  attach_function :sleep, [:int], :int
end

class Counter
  attr_reader :counter

  def initialize
      @rnd = Random.new
      @counter = 0
  end

  def increment
      @counter += @rnd.rand(1..1)
  end
end

# Функция для увеличения счетчика
def increment(counter)
  1000.times do
      counter.increment  # Ошибка: race condition здесь
      MyLib.sleep(0.0001)  # Внешний вызов, GIL может быть освобожден
  end
end

def main()
  # Запускаем два потока
  threads = []
  counter = Counter.new
  100.times do
      threads << Thread.new { increment(counter) }
  end

  # Ожидаем завершения всех потоков
  threads.each(&:join)

  puts "Final counter value: #{counter.counter}"
end

main()

