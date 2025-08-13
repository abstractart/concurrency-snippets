// gcc -O2 -pthread global-var-without-volatile.c -o global-var-without-volatile
#include <stdio.h>
#include <pthread.h>
#include <unistd.h>

int stop = 0;

void *worker(void *arg)
{
  int local_sum = 0;
  while (!stop)
  {
    local_sum++;
  }
  printf("Worker stopped, sum = %d\n", local_sum);
  return NULL;
}

int main()
{
  pthread_t t;
  pthread_create(&t, NULL, worker, NULL);

  sleep(1);
  stop = 1;

  pthread_join(t, NULL);
  return 0;
}
