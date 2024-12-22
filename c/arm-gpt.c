#include <stdio.h>
#include <pthread.h>
#include <stdatomic.h>

// Shared variables
atomic_int x = 0, y = 0;
atomic_int a = 0, b = 0;

void* thread1(void* arg) {
    atomic_store_explicit(&x, 1, memory_order_relaxed);  // Write to x
    a = atomic_load_explicit(&y, memory_order_relaxed);  // Read y
    return NULL;
}

void* thread2(void* arg) {
    atomic_store_explicit(&y, 1, memory_order_relaxed);  // Write to y
    b = atomic_load_explicit(&x, memory_order_relaxed);  // Read x
    return NULL;
}

int main() {
    pthread_t t1, t2;

    int reordering_detected = 0;

    // Run the test multiple times
    for (int i = 0; i < 1000000; i++) {
        // Reset shared variables
        atomic_store(&x, 0);
        atomic_store(&y, 0);
        atomic_store(&a, 0);
        atomic_store(&b, 0);

        // Create threads
        pthread_create(&t1, NULL, thread1, NULL);
        pthread_create(&t2, NULL, thread2, NULL);

        // Wait for threads to finish
        pthread_join(t1, NULL);
        pthread_join(t2, NULL);

        // Check for reordering (a == 0 and b == 0)
        if (atomic_load(&a) == 0 && atomic_load(&b) == 0) {
            reordering_detected++;
        }
    }

    // Print results
    printf("Reordering detected in %d iterations.\n", reordering_detected);

    return 0;
}
