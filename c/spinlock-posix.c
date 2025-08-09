#include <pthread.h>
#include <stdio.h>

pthread_spinlock_t lock;
int counter = 0;

void* worker(void* arg) {
    for (int i = 0; i < 1000000; i++) {
        pthread_spin_lock(&lock);
        counter++;
        pthread_spin_unlock(&lock);
    }
    return NULL;
}

int main() {
    pthread_t t1, t2;

    pthread_spin_init(&lock, PTHREAD_PROCESS_PRIVATE);

    pthread_create(&t1, NULL, worker, NULL);
    pthread_create(&t2, NULL, worker, NULL);

    pthread_join(t1, NULL);
    pthread_join(t2, NULL);

    pthread_spin_destroy(&lock);

    printf("Counter: %d\n", counter);
    return 0;
}
