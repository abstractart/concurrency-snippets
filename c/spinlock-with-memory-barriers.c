#include <stdio.h>
#include <stdlib.h>
#include <pthread.h>
#include <stdatomic.h>
#include <stdbool.h>

#define THREADS 4
#define INCREMENTS 1000000

typedef struct {
    atomic_bool locked;
} spinlock_t;

static inline void memory_barrier(void) {
#if defined(__x86_64__) || defined(__i386__)
    __asm__ __volatile__("mfence" ::: "memory");
#elif defined(__aarch64__)
    __asm__ __volatile__("dmb ish" ::: "memory");
#else
#   error "Unsupported architecture"
#endif
}

void spinlock_init(spinlock_t *lock) {
    atomic_store(&lock->locked, false);
}

void spinlock_lock(spinlock_t *lock) {
    for (;;) {
        bool expected = false;
        if (atomic_compare_exchange_weak_explicit(
                &lock->locked,
                &expected,
                true,
                memory_order_acquire,
                memory_order_relaxed)) {
            memory_barrier();
            return;
        }
    }
}

void spinlock_unlock(spinlock_t *lock) {
    memory_barrier();
    atomic_store_explicit(&lock->locked, false, memory_order_release);
}

spinlock_t lock;
long counter = 0;

void* worker(void* arg) {
    for (long i = 0; i < INCREMENTS; i++) {
        spinlock_lock(&lock);
        counter++;
        spinlock_unlock(&lock);
    }
    return NULL;
}

int main() {
    pthread_t threads[THREADS];

    spinlock_init(&lock);

    for (int i = 0; i < THREADS; i++) {
        if (pthread_create(&threads[i], NULL, worker, NULL) != 0) {
            perror("pthread_create");
            return 1;
        }
    }

    for (int i = 0; i < THREADS; i++) {
        pthread_join(threads[i], NULL);
    }

    printf("Expected counter = %ld, actual counter = %ld\n",
           (long)THREADS * INCREMENTS, counter);

    return 0;
}
