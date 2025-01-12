import threading
import time
import os

def work(seconds):
    print(f"Child thread pid: {os.getpid()} \nChild thread id: {threading.get_native_id()}\n\n")
    time.sleep(seconds)

def run_threads(seconds, threadsCount):
    threads = []
    for _ in range(threadsCount):
        t = threading.Thread(target=work, args=(seconds,))
        t.start()
        threads.append(t)

    for t in threads:
        t.join()
  
if __name__ == "__main__":  
    threadsCount = 10
    seconds = 60
    
    print("Main thread with pid: {}", os.getpid())
    input("Press Enter to continue...")
    
    run_threads(seconds, threadsCount)
