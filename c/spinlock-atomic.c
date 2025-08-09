#include <stdio.h>
#include <stdatomic.h>
#include <pthread.h>

atomic_flag lock = ATOMIC_FLAG_INIT;  // атомарный флаг
int counter = 0;

void spin_lock(atomic_flag *l) {
    while (atomic_flag_test_and_set(l)) {
        // крутимся, пока lock == true
    }
}

void spin_unlock(atomic_flag *l) {
    atomic_flag_clear(l);
}

void* worker(void* arg) {
    for (int i = 0; i < 1000000; i++) {
        spin_lock(&lock);
        counter++;
        spin_unlock(&lock);
    }
    return NULL;
}

int main() {
    pthread_t t1, t2;

    pthread_create(&t1, NULL, worker, NULL);
    pthread_create(&t2, NULL, worker, NULL);

    pthread_join(t1, NULL);
    pthread_join(t2, NULL);

    printf("Counter: %d\n", counter);
    return 0;
}
