# Пример 1: Race Condition при изменении глобальной переменной
class Arr
    attr_reader :v

    def initialize
        @v = []
    end

    def add
        @v << nil
    end

    def len
        @v.length
    end
end

# Функция для увеличения счетчика
def append(arr, iterations)
    iterations.times do
        arr.add  # Ошибка: race condition здесь
    end
end

def main()
    # Запускаем два потока
    arrs = [Arr.new]
    for arr in arrs
        testcase(arr)
    end

end

def testcase(arr)
    threadsCount = 1000
    iterationsCount = 100
    
    threads = []
    threadsCount.times do
        threads << Thread.new { append(arr, iterationsCount) }
    end

    # Ожидаем завершения всех потоков
    threads.each(&:join)

    puts "Final #{arr.class.name} value: #{arr.len}, as expected: #{threadsCount * iterationsCount == arr.len}"
end

main()

