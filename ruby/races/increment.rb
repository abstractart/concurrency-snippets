# Пример 1: Race Condition при изменении глобальной переменной
class Counter
    attr_reader :counter

    def initialize
        @counter = 0
    end

    def increment
        @counter += 1
    end
end

class CounterMultiLine
    attr_reader :counter

    def initialize
        @counter = 0
    end

    def increment
        c = @counter
        c = c + 1.01.round

        @counter = c
    end
end

class CounterWithRand
    attr_reader :counter

    def initialize
        @rnd = Random.new
        @counter = 0
    end

    def increment
        @counter += @rnd.rand(1..1)
    end
end

class CounterWithLambda
    attr_reader :counter

    def initialize
        @rnd = Random.new
        @counter = 0
    end

    def increment
        l = lambda do
           # sleep(0.0001)
            
            return @rnd.rand(1..1)
        end
        @counter += l.call
    end
end

class CounterWithTypeConversion
    attr_reader :counter

    def initialize
        @rnd = Random.new
        @counter = 0
    end

    def increment
        @counter += 1.01.round
    end
end

class CounterWithSleepAfterIncrement
    attr_reader :counter

    def initialize
        @rnd = Random.new
        @counter = 0
    end

    def increment
        c = @counter
        c += 1
        sleep(0.0001)
        @counter = c
    end
end

class CounterWithSleepBeforeIncrement
    attr_reader :counter

    def initialize
        @rnd = Random.new
        @counter = 0
    end

    def increment
        c = @counter
        sleep(0.001)
        c += 1

        @counter = c
    end
end

# Функция для увеличения счетчика
def increment(counter, iterations)
    iterations.times do
        counter.increment  # Ошибка: race condition здесь
    end
end

def main()
    # Запускаем два потока
    counters = [
        Counter.new, 
        CounterWithLambda.new,
        CounterWithRand.new,
        CounterWithTypeConversion.new, 
        CounterWithSleepBeforeIncrement.new, 
        CounterWithSleepAfterIncrement.new,
        CounterMultiLine.new
    ]
    for counter in counters
        testcase(counter)
    end

end

def testcase(counter)
    threadsCount = 1000
    iterationsCount = 100
    
    threads = []
    threadsCount.times do
        threads << Thread.new { increment(counter, iterationsCount) }
    end

    # Ожидаем завершения всех потоков
    threads.each(&:join)

    if threadsCount * iterationsCount != counter.counter

        puts "Final #{counter.class.name} value: #{counter.counter}, as expected: #{threadsCount * iterationsCount == counter.counter}"
    end
end

main()

