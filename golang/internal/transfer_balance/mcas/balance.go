package mcas

// Каждая ячейка счёта хранит либо реальный баланс (tx==nil),
// либо маркер захваченной транзакции (tx!=nil).
type balance struct {
	amount int64
	tx     *tx
}

func (b *balance) isInFlight() bool { return b.tx != nil }
