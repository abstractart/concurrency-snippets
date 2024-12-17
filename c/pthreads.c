#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>

// while true; do ./a.out; done


volatile int x, y, r1, r2;

void* f1(void* vargp)
{
    x = 1;
    r1 = y;
    return NULL;
}

void* f2(void* vargp)
{
    y = 1;
    r2 = x;
    return NULL;
}
int main()
{
    pthread_t t1, t2;
    pthread_create(&t1, NULL, f1, NULL);
    pthread_create(&t2, NULL, f2, NULL);

    pthread_join(t1, NULL);
    pthread_join(t2, NULL);

    if (r1 == 0 && r2 == 0) {
        printf("r1 = %d, r2 = %d\n", r1, r2);
    }
    
    exit(0);
}
