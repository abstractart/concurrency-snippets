// gcc -O2 -pthread volatile-global-var.c -o volatile-global-var
#include <stdio.h>
#include <pthread.h>
#include <unistd.h>

volatile int stop = 0; // теперь volatile

void* worker(void* arg) {
    int local_sum = 0;
    while (!stop) { // теперь каждый раз читаем из памяти
        local_sum++;
    }
    printf("Worker stopped, sum = %d\n", local_sum);
    return NULL;
}

int main() {
    pthread_t t;
    pthread_create(&t, NULL, worker, NULL);

    sleep(1);
    stop = 1;

    pthread_join(t, NULL);
    return 0;
}
