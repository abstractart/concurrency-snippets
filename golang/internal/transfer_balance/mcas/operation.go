package mcas

type operation struct {
	acc    *Account
	before *balance
	after  *balance
}

func newOperation(acct *Account, t *tx, delta int64) operation {
	current := acct.readBalance()
	return operation{
		acc:    acct,
		before: current,
		after:  &balance{amount: current.amount + delta, tx: t},
	}
}

func (op *operation) tx() *tx                     { return op.after.tx }
func (op *operation) current() *balance           { return op.acc.balance.Load() }
func (op *operation) isClaimed(cur *balance) bool { return cur == op.after }
func (op *operation) isStale(cur *balance) bool   { return cur != op.before }
func (op *operation) tryClaim() bool {
	return op.acc.balance.CompareAndSwap(op.before, op.after)
}

func (op *operation) tryFinalize() {
	op.acc.balance.CompareAndSwap(op.after, &balance{amount: op.after.amount})
}

func (op *operation) tryRestore() {
	op.acc.balance.CompareAndSwap(op.after, op.before)
}

func (op *operation) prepare() bool {
	for {
		if op.tx().resolved() {
			return op.tx().succeeded()
		}
		cur := op.current()
		switch {
		case op.isClaimed(cur):
			return true
		case cur.isInFlight():
			cur.tx.commit()
			continue
		case op.isStale(cur):
			return false
		default:
			op.tryClaim()
		}
	}
}
