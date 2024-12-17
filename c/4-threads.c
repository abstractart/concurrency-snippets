#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>

// while true; do ./a.out; done


volatile int x, y, r1, r2, r3, r4;

void* f1(void* vargp)
{
    x = 1;
    return NULL;
}

void* f2(void* vargp)
{
    y = 1;
    return NULL;
}

void* f3(void* vargp)
{
    r1 = x;
    r2 = y;
    return NULL;
}

void* f4(void* vargp)
{
    r3 = y;
    r4 = x;
    return NULL;
}
int main()
{
    pthread_t t1, t2, t3, t4;
    pthread_create(&t1, NULL, f1, NULL);
    pthread_create(&t2, NULL, f2, NULL);
    pthread_create(&t3, NULL, f3, NULL);
    pthread_create(&t4, NULL, f4, NULL);
    
    pthread_join(t1, NULL);
    pthread_join(t2, NULL);
    pthread_join(t3, NULL);
    pthread_join(t4, NULL);


    if ((r1 == 1 && r2 == 0 && r3 == 1 && r4 == 0))  {
        printf("true\n");
    }
    
    exit(0);
}
