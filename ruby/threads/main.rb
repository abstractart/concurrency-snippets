require 'thread'

def work(seconds)
  puts "Child thread pid: #{Process.pid} \nChild thread id: #{Thread.current.native_thread_id}\n\n"
  sleep(seconds)
end

def run_threads(seconds, threads_count)
  threads = []
  
  threads_count.times do
    thread = Thread.new { work(seconds) }
    threads << thread
  end
  
  threads.each(&:join)
end

if __FILE__ == $0
  threads_count = 10
  seconds = 60

  puts "Main thread with pid: #{Process.pid}"
  puts "Press Enter to continue..."
  gets

  run_threads(seconds, threads_count)
end