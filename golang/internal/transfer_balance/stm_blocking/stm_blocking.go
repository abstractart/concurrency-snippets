package stm_blocking

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
	"unsafe"
)

// Account — транзакционная переменная (TVar).
// version увеличивается при каждой записи — дёшево, без аллокаций.
// waiters создаётся только когда кто-то реально вызвал Retry() и ждёт.
type Account struct {
	mu      sync.Mutex
	value   int64
	version uint64
	waiters []chan struct{}
}

func NewAccount(value int64) *Account {
	return &Account{value: value}
}

func (a *Account) Balance() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.value
}

func (a *Account) write(value int64) {
	a.value = value
	a.version++
	// Будим ждущих только если они есть — в большинстве операций эта ветка не берётся.
	if len(a.waiters) > 0 {
		for _, ch := range a.waiters {
			close(ch)
		}
		a.waiters = a.waiters[:0]
	}
}

// subscribe регистрирует канал ожидания и возвращает его.
// Вызывается только из waitForChange, т.е. только при Retry().
func (a *Account) subscribe() chan struct{} {
	a.mu.Lock()
	ch := make(chan struct{})
	a.waiters = append(a.waiters, ch)
	a.mu.Unlock()
	return ch
}

// snapshot — снимок аккаунта на момент чтения в транзакции.
type snapshot struct {
	value   int64
	version uint64
}

// isStale вызывается пока держим mu — сравнение версий без аллокаций.
func (s snapshot) isStale(a *Account) bool {
	return a.version != s.version
}

// Txn — контекст транзакции.
// Read/Write не трогают реальные значения — только накапливают намерения.
type Txn struct {
	reads     map[*Account]snapshot
	writes    map[*Account]int64
	retryFlag bool
}

func newTxn() *Txn {
	return &Txn{
		reads:  make(map[*Account]snapshot),
		writes: make(map[*Account]int64),
	}
}

func (t *Txn) Read(a *Account) int64 {
	if val, ok := t.writes[a]; ok {
		return val // read-your-own-writes
	}
	a.mu.Lock()
	snap := snapshot{value: a.value, version: a.version}
	a.mu.Unlock()
	t.reads[a] = snap
	return snap.value
}

func (t *Txn) Write(a *Account, val int64) {
	t.writes[a] = val
}

// Retry блокирует транзакцию до тех пор пока один из прочитанных аккаунтов
// не изменится, после чего транзакция перезапускается.
func (t *Txn) Retry() {
	t.retryFlag = true
}

func (t *Txn) commit() bool {
	accounts := make([]*Account, 0, len(t.writes))
	for a := range t.writes {
		accounts = append(accounts, a)
	}
	// Блокируем в порядке адресов — предотвращает deadlock.
	sort.Slice(accounts, func(i, j int) bool {
		return uintptr(unsafe.Pointer(accounts[i])) < uintptr(unsafe.Pointer(accounts[j]))
	})
	for _, a := range accounts {
		a.mu.Lock()
	}
	defer func() {
		for _, a := range accounts {
			a.mu.Unlock()
		}
	}()

	// Валидируем reads: если версия изменилась пока думали — retry.
	for a, snap := range t.reads {
		if snap.isStale(a) {
			return false
		}
	}

	for _, a := range accounts {
		a.write(t.writes[a])
	}
	return true
}

// waitForChange подписывается на изменения и блокируется до первого из них.
// Каналы создаются здесь — только когда Retry() реально нужен.
func waitForChange(reads map[*Account]snapshot) {
	cases := make([]reflect.SelectCase, 0, len(reads))
	for a := range reads {
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(a.subscribe()),
		})
	}
	if len(cases) > 0 {
		reflect.Select(cases)
	}
}

// Atomically выполняет fn в транзакции.
// При конфликте — немедленный retry.
// При Retry() — блокируется до изменения одного из прочитанных аккаунтов.
func Atomically(fn func(*Txn)) {
	for {
		txn := newTxn()
		fn(txn)
		if txn.retryFlag {
			waitForChange(txn.reads)
			continue
		}
		if txn.commit() {
			return
		}
	}
}

func Transfer(from, to *Account, amount int64) {
	Atomically(func(tx *Txn) {
		tx.Write(from, tx.Read(from)-amount)
		tx.Write(to, tx.Read(to)+amount)
	})
}

// WithdrawWhenReady демонстрирует Retry: блокируется пока средств не хватит,
// затем снимает атомарно — без spin-loop и без ручного sync.Cond.
func WithdrawWhenReady(from *Account, amount int64) {
	Atomically(func(tx *Txn) {
		if tx.Read(from) < amount {
			tx.Retry()
			return
		}
		tx.Write(from, tx.Read(from)-amount)
	})
}

func Run() {
	a := NewAccount(1000)
	b := NewAccount(0)
	fmt.Printf("before: a=%d, b=%d\n", a.Balance(), b.Balance())
	Transfer(a, b, 300)
	fmt.Printf("after:  a=%d, b=%d\n", a.Balance(), b.Balance())

	c := NewAccount(50)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("goroutine: waiting to withdraw 100 from c (balance=50)...")
		WithdrawWhenReady(c, 100)
		fmt.Printf("goroutine: withdrew 100, c=%d\n", c.Balance())
	}()

	fmt.Println("main: depositing 100 to c...")
	Transfer(b, c, 100)
	wg.Wait()
}
