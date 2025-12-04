#include <stdatomic.h>
#include <stdio.h>
#include <pthread.h>
#include <unistd.h>

typedef struct
{
    atomic_uint next_ticket;
    atomic_uint now_serving;
} ticket_lock_t;

void ticket_lock_init(ticket_lock_t *lock)
{
    atomic_init(&lock->next_ticket, 0);
    atomic_init(&lock->now_serving, 0);
}

void ticket_lock_acquire(ticket_lock_t *lock)
{
    unsigned my_ticket = atomic_fetch_add_explicit(&lock->next_ticket, 1, memory_order_relaxed);
    while (atomic_load_explicit(&lock->now_serving, memory_order_acquire) != my_ticket)
    {
        // active wait
        // можно добавить sched_yield() чтобы снизить нагрузку
    }
}

void ticket_lock_release(ticket_lock_t *lock)
{
    atomic_fetch_add_explicit(&lock->now_serving, 1, memory_order_release);
}

ticket_lock_t lock;
int counter = 0;

void *worker(void *arg)
{
    for (int i = 0; i < 100000; i++)
    {
        ticket_lock_acquire(&lock);
        counter++;
        ticket_lock_release(&lock);
    }
    return NULL;
}

int main()
{
    ticket_lock_init(&lock);
    pthread_t t1, t2, t3;

    pthread_create(&t1, NULL, worker, NULL);
    pthread_create(&t2, NULL, worker, NULL);
    pthread_create(&t3, NULL, worker, NULL);

    pthread_join(t1, NULL);
    pthread_join(t2, NULL);
    pthread_join(t3, NULL);

    printf("counter = %d\n", counter);
}
