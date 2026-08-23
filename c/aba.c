// ABA Problem in lock-free programming — C demonstration.
//
// C делает ABA опаснее Go по двум причинам:
//   1. Нет GC — free() + malloc() часто возвращает ТОТ ЖЕ адрес,
//      т.е. «проблема A→B→A» возникает естественно, а не искусственно.
//   2. Нет защиты от use-after-free — результат: реальная порча данных.
//
// Сценарий (lock-free stack):
//
//   Начало:  top → A → B → C
//
//   T1: читает top=A, A.next=B  →  готовит CAS(top, A, B)  →  ПАУЗА
//   T2: pop A, pop B, push A    →  стек: top → A → C
//   T1: возобновляет CAS        →  успех! (top всё ещё A)
//                                  теперь top → B  ← зомби-узел!
//
// Compile: clang -O1 -std=c11 -Wall -Wextra -o aba aba.c -lpthread
// Run:     ./aba

#include <stdio.h>
#include <stdlib.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdbool.h>
#include <sched.h>
#include <pthread.h>

// ─────────────────────────────────────────────────────────────────────────────
// Node
// ─────────────────────────────────────────────────────────────────────────────

typedef struct Node {
    int                   val;
    _Atomic(struct Node*) next;
} Node;

static Node* make_node(int val) {
    Node* n = malloc(sizeof *n);
    n->val   = val;
    atomic_store(&n->next, (Node*)NULL);
    return n;
}

// ─────────────────────────────────────────────────────────────────────────────
// ЧАСТЬ 0: повторное использование адресов (только в C)
// ─────────────────────────────────────────────────────────────────────────────

static void demo_memory_reuse(void) {
    puts("════════════════════════════════════════════════════");
    puts("  ЧАСТЬ 0: повторное использование адресов (C vs Go)");
    puts("════════════════════════════════════════════════════\n");

    Node* a = make_node(42);
    printf("  malloc() → A = %p  val=%d\n", (void*)a, a->val);

    free(a);
    puts("  free(A)");

    Node* b = make_node(99);
    printf("  malloc() → B = %p  val=%d\n\n", (void*)b, b->val);

    if ((void*)a == (void*)b) {
        printf("  ★ A == B по адресу!  CAS не отличит «старый A» от «нового B».\n");
        printf("    В Go GC держит объект живым пока есть ссылки — этого не случится.\n");
    } else {
        printf("  Адреса различаются в этот раз, но аллокатор часто\n");
        printf("  возвращает тот же адрес сразу после free().\n");
    }
    free(b);
    putchar('\n');
}

// ─────────────────────────────────────────────────────────────────────────────
// ЧАСТЬ 1: ABA-уязвимый lock-free стек
// ─────────────────────────────────────────────────────────────────────────────

typedef struct {
    _Atomic(Node*) top;
} UnsafeStack;

static void us_push(UnsafeStack* s, Node* n) {
    Node* top;
    do {
        top = atomic_load(&s->top);
        atomic_store(&n->next, top);
    } while (!atomic_compare_exchange_weak(&s->top, &top, n));
}

static Node* us_pop(UnsafeStack* s) {
    Node* top, *next;
    do {
        top = atomic_load(&s->top);
        if (!top) return NULL;
        next = atomic_load(&top->next);
    } while (!atomic_compare_exchange_weak(&s->top, &top, next));
    return top;
}

static void us_print(const UnsafeStack* s, const char* label) {
    printf("%-34s [", label);
    for (Node* n = atomic_load((Node* _Atomic*)&s->top); n;
         n = atomic_load(&n->next))
        printf(" %d", n->val);
    puts(" ]");
}

// ── Данные, разделяемые между потоками ──────────────────────────────────────

typedef struct {
    UnsafeStack* s;
    Node*        node_a;
    Node*        node_b;
    _Atomic int  t2_go;    // T1 → T2: разрешение работать
    _Atomic int  t2_done;  // T2 → T1: T2 завершил
} UnsafeArgs;

// ── Поток 1: хочет сделать pop, но паузируется ──────────────────────────────

static void* t1_unsafe(void* arg) {
    UnsafeArgs* a = arg;

    // Шаг 1: читаем снимок top и top.next
    Node* saved_top  = atomic_load(&a->s->top);       // = &nodeA
    Node* saved_next = atomic_load(&a->node_a->next); // = &nodeB

    printf("[T1] прочитал: top=A(%d), A.next=B(%d)\n",
           saved_top->val, saved_next->val);
    puts("[T1] засыпает перед CAS...\n");

    atomic_store(&a->t2_go, 1);                         // запускаем T2
    while (!atomic_load(&a->t2_done)) sched_yield();    // ждём T2

    // Шаг 2: CAS(top, &A, &B)
    // top всё ещё == &nodeA (по указателю) → CAS УСПЕВАЕТ, хотя не должен!
    bool ok = atomic_compare_exchange_strong(
        &a->s->top, &saved_top, saved_next);

    printf("[T1] CAS(top, A, B) → успех=%s  ← ABA!\n\n", ok ? "true" : "false");
    return NULL;
}

// ── Поток 2: pop-pop-push, пока T1 спит ────────────────────────────────────

static void* t2_unsafe(void* arg) {
    UnsafeArgs* a = arg;
    while (!atomic_load(&a->t2_go)) sched_yield();

    Node* pa = us_pop(a->s);
    printf("[T2] pop → A(%d),  ", pa->val); us_print(a->s, "стек:");

    Node* pb = us_pop(a->s);
    printf("[T2] pop → B(%d),  ", pb->val); us_print(a->s, "стек:");

    // Имитируем free(B): записываем мусорное значение.
    // В реальной программе после free() память может быть переиспользована
    // для другого объекта — потребитель стека получит чужие данные.
    pb->val = -999;

    // Push A обратно — тот же указатель, «новый» логический объект
    atomic_store(&pa->next, (Node*)NULL);
    us_push(a->s, pa);
    printf("[T2] push A(%d),   ", pa->val); us_print(a->s, "стек:");
    putchar('\n');

    atomic_store(&a->t2_done, 1);
    return NULL;
}

static void demo_aba_unsafe(void) {
    puts("════════════════════════════════════════════════════");
    puts("  ЧАСТЬ 1: ABA-уязвимый стек");
    puts("════════════════════════════════════════════════════\n");

    Node* nA = make_node(1);
    Node* nB = make_node(2);
    Node* nC = make_node(3);

    UnsafeStack s;
    atomic_init(&s.top, (Node*)NULL);
    us_push(&s, nC); us_push(&s, nB); us_push(&s, nA);
    us_print(&s, "Начальный стек (top→A→B→C):");
    putchar('\n');

    UnsafeArgs args = {
        .s = &s, .node_a = nA, .node_b = nB,
        .t2_go = 0, .t2_done = 0,
    };

    pthread_t t1, t2;
    pthread_create(&t2, NULL, t2_unsafe, &args);
    pthread_create(&t1, NULL, t1_unsafe, &args);
    pthread_join(t1, NULL);
    pthread_join(t2, NULL);

    us_print(&s, "Итоговый стек         :");
    puts("Ожидалось              : [ 1 3 ]  (A→C)\n");

    Node* top = atomic_load(&s.top);
    printf("ПРОБЛЕМА: top→B  val=%d  (мусор после имитации free!)\n", top->val);
    Node* tn = atomic_load(&top->next);
    if (tn)
        printf("          B.next=%d  ← use-after-free: читаем чужую память\n", tn->val);
    puts("          A потерян несмотря на то что был возвращён в стек.\n");

    free(nA); free(nB); free(nC);
}

// ─────────────────────────────────────────────────────────────────────────────
// ЧАСТЬ 2: ABA-safe стек через tagged pointer (ptr + счётчик версии)
//
// CAS атомарно сравнивает ОБА поля — 16 байт за одну инструкцию.
// Даже если node A вернулась на вершину, версия другая → CAS проваливается.
//
// На x86-64: CMPXCHG16B (нужен флаг -mcx16 или -march=native)
// На ARM64:  CASP / LDXP+STXP (поддерживается Apple Silicon нативно)
// ─────────────────────────────────────────────────────────────────────────────

typedef struct {
    Node*    ptr;
    uint64_t ver;
} TaggedPtr;  // 16 байт — кандидат на 128-битный CAS

typedef struct {
    _Atomic(TaggedPtr) top;
    _Atomic(uint64_t)  ver;  // монотонно растущий счётчик
} SafeStack;

static void ss_push(SafeStack* s, Node* n) {
    TaggedPtr old, next;
    do {
        old = atomic_load(&s->top);
        atomic_store(&n->next, old.ptr);
        next = (TaggedPtr){ n, atomic_fetch_add(&s->ver, 1) + 1 };
    } while (!atomic_compare_exchange_weak(&s->top, &old, next));
}

static Node* ss_pop(SafeStack* s) {
    TaggedPtr old, next;
    do {
        old = atomic_load(&s->top);
        if (!old.ptr) return NULL;
        Node* nxt = atomic_load(&old.ptr->next);
        next = (TaggedPtr){ nxt, atomic_fetch_add(&s->ver, 1) + 1 };
    } while (!atomic_compare_exchange_weak(&s->top, &old, next));
    return old.ptr;
}

static void ss_print(const SafeStack* s, const char* label) {
    printf("%-34s [", label);
    TaggedPtr ref = atomic_load((TaggedPtr _Atomic*)&s->top);
    for (Node* n = ref.ptr; n; n = atomic_load(&n->next))
        printf(" %d", n->val);
    puts(" ]");
}

typedef struct {
    SafeStack*  s;
    Node*       node_a;
    _Atomic int t2_go;
    _Atomic int t2_done;
} SafeArgs;

static void* t1_safe(void* arg) {
    SafeArgs* a = arg;

    TaggedPtr saved = atomic_load(&a->s->top);
    printf("[T1] прочитал stamp: {val=%d, ver=%llu}\n",
           saved.ptr->val, (unsigned long long)saved.ver);
    puts("[T1] засыпает перед CAS...\n");

    atomic_store(&a->t2_go, 1);
    while (!atomic_load(&a->t2_done)) sched_yield();

    TaggedPtr after = atomic_load(&a->s->top);
    printf("[T1] текущий stamp:  {val=%d, ver=%llu}  ← ver изменилась!\n",
           after.ptr->val, (unsigned long long)after.ver);

    // Пытаемся установить top = {B, ver+1}, отправной точкой считая saved
    Node*     next_node = atomic_load(&a->node_a->next);
    TaggedPtr new_top   = { next_node, atomic_load(&a->s->ver) + 1 };

    bool ok = atomic_compare_exchange_strong(&a->s->top, &saved, new_top);
    printf("[T1] CAS({A,v%llu}, {B,...}) → успех=%s  ← ABA предотвращён!\n\n",
           (unsigned long long)saved.ver, ok ? "true" : "false");
    return NULL;
}

static void* t2_safe(void* arg) {
    SafeArgs* a = arg;
    while (!atomic_load(&a->t2_go)) sched_yield();

    Node* pa = ss_pop(a->s);
    printf("[T2] pop → A(%d)\n", pa->val);

    Node* pb = ss_pop(a->s);
    printf("[T2] pop → B(%d)\n", pb->val);
    (void)pb;

    atomic_store(&pa->next, (Node*)NULL);
    ss_push(a->s, pa);   // каждый push инкрементирует ver!

    TaggedPtr cur = atomic_load(&a->s->top);
    printf("[T2] push A(%d)  →  {val=%d, ver=%llu}\n\n",
           pa->val, cur.ptr->val, (unsigned long long)cur.ver);

    atomic_store(&a->t2_done, 1);
    return NULL;
}

static void demo_aba_safe(void) {
    puts("════════════════════════════════════════════════════");
    puts("  ЧАСТЬ 2: ABA-safe стек (tagged pointer)");
    puts("════════════════════════════════════════════════════\n");

    // Проверяем, поддерживает ли платформа lock-free 128-bit CAS
    _Atomic(TaggedPtr) probe;
    printf("TaggedPtr (16 bytes) lock-free: %s\n\n",
           atomic_is_lock_free(&probe) ? "да ✓" : "нет (используется internal lock)");

    Node* nA = make_node(1);
    Node* nB = make_node(2);
    Node* nC = make_node(3);

    SafeStack s;
    atomic_init(&s.top, ((TaggedPtr){ NULL, 0 }));
    atomic_init(&s.ver, 0);
    ss_push(&s, nC); ss_push(&s, nB); ss_push(&s, nA);

    TaggedPtr t0 = atomic_load(&s.top);
    ss_print(&s, "Начальный стек (top→A→B→C):");
    printf("top stamp: {val=%d, ver=%llu}\n\n",
           t0.ptr->val, (unsigned long long)t0.ver);

    SafeArgs args = { .s = &s, .node_a = nA, .t2_go = 0, .t2_done = 0 };

    pthread_t t1, t2;
    pthread_create(&t2, NULL, t2_safe, &args);
    pthread_create(&t1, NULL, t1_safe, &args);
    pthread_join(t1, NULL);
    pthread_join(t2, NULL);

    ss_print(&s, "Итоговый стек         :");
    puts("Ожидалось              : [ 1 3 ]  ✓\n");
    puts("РЕШЕНИЕ: CAS атомарно сравнивает {ptr + ver} — 16 байт за раз.");
    puts("         Даже если ptr вернулся к A, ver изменилась → CAS проваливается.");

    free(nA); free(nB); free(nC);
}

// ─────────────────────────────────────────────────────────────────────────────

int main(void) {
    demo_memory_reuse();
    demo_aba_unsafe();
    demo_aba_safe();
    return 0;
}
