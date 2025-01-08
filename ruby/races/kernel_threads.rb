def testcase()
    threadsCount = 100
    
    threads = []
    threadsCount.times do
        threads << Thread.new { sleep(100) }
    end

    # Ожидаем завершения всех потоков
    threads.each(&:join)
end

testcase()