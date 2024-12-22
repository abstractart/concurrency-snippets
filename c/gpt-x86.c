#include <stdio.h>
#include <pthread.h>

// Общие переменные
volatile int x = 0, y = 0;
volatile int a = 0, b = 0;

// Первый поток
void *thread1(void *arg)
{
  x = 1; // Запись в x
  a = y; // Чтение из y
  return NULL;
}

// Второй поток
void *thread2(void *arg)
{
  y = 1; // Запись в y
  b = x; // Чтение из x
  return NULL;
}

int main()
{
  pthread_t t1, t2;

  int reorder_count = 0; // Счетчик для случаев переупорядочивания

  // Выполняем тест многократно
  for (int i = 0; i < 1000000; i++)
  {
    // Сбрасываем значения переменных
    x = y = a = b = 0;

    // Создаем потоки
    pthread_create(&t1, NULL, thread1, NULL);
    pthread_create(&t2, NULL, thread2, NULL);

    // Ждем завершения потоков
    pthread_join(t1, NULL);
    pthread_join(t2, NULL);

    // Проверяем наличие переупорядочивания (a == 0 и b == 0)
    if (a == 0 && b == 0)
    {
      reorder_count++;
    }
  }

  // Выводим результат
  printf("Количество переупорядочиваний: %d\n", reorder_count);

  return 0;
}
