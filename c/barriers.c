#include <stdio.h>
#include <pthread.h>
#include <stdatomic.h>

// Общие переменные
atomic_int X = 0, Y = 0;
int r1 = 0, r2 = 0;

void *thread1(void *arg) {
    atomic_store_explicit(&X, 1, memory_order_relaxed);  // Запись в X
    asm volatile ("dmb ish" ::: "memory");              // Барьер для ARM (не влияет на x86)
    r1 = atomic_load_explicit(&Y, memory_order_relaxed); // Чтение из Y
    return NULL;
}

void *thread2(void *arg) {
    atomic_store_explicit(&Y, 1, memory_order_relaxed);  // Запись в Y
    asm volatile ("dmb ish" ::: "memory");              // Барьер для ARM (не влияет на x86)
    r2 = atomic_load_explicit(&X, memory_order_relaxed); // Чтение из X
    return NULL;
}

int main() {
    pthread_t t1, t2;

    for (int i = 0; i < 1000000; i++) { // Цикл для увеличения вероятности
        // Сброс переменных
        atomic_store_explicit(&X, 0, memory_order_relaxed);
        atomic_store_explicit(&Y, 0, memory_order_relaxed);
        r1 = r2 = 0;

        // Создаем два потока
        pthread_create(&t1, NULL, thread1, NULL);
        pthread_create(&t2, NULL, thread2, NULL);

        // Ждем завершения потоков
        pthread_join(t1, NULL);
        pthread_join(t2, NULL);

        // Проверяем на гарантированную разницу
        if (r1 == 0 && r2 == 0) {
            printf("Observed r1 = %d, r2 = %d on iteration %d\n", r1, r2, i + 1);
            break;
        }
    }

    return 0;
}
