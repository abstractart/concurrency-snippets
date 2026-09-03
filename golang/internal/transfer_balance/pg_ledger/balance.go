package pg_ledger

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateAccount создаёт счёт. allowNegative=true — для системных счетов,
// из которых деньги вводятся в систему.
func CreateAccount(
	ctx context.Context, db *pgxpool.Pool,
	name, currency string, allowNegative bool,
) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO ledger_accounts (name, currency, allow_negative)
		VALUES ($1, $2, $3) RETURNING id`,
		name, currency, allowNegative).Scan(&id)
	return id, err
}

// CachedBalance — быстрый путь, O(1). Так балансы отдают в API.
func CachedBalance(ctx context.Context, db *pgxpool.Pool, accountID int64) (int64, error) {
	var balance int64
	err := db.QueryRow(ctx,
		"SELECT balance FROM ledger_accounts WHERE id = $1", accountID).Scan(&balance)
	return balance, err
}

// DerivedBalance берёт баланс из журнала — источник истины, O(1).
//
// Читается balance_after последней записи, а не SUM по всей истории:
// индекс (account_id, account_seq) отдаёт её одним обращением.
// Кэш из ledger_accounts при этом не используется, поэтому сверка
// остаётся независимой, а не круговой.
func DerivedBalance(ctx context.Context, db *pgxpool.Pool, accountID int64) (int64, error) {
	var balance int64
	err := db.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT balance_after FROM ledger_entries
			 WHERE account_id = $1 ORDER BY account_seq DESC LIMIT 1), 0)`,
		accountID).Scan(&balance)
	return balance, err
}

// LedgerSum — сумма всех записей журнала. Всегда ноль: каждая проводка
// сбалансирована, значит и весь журнал.
func LedgerSum(ctx context.Context, db *pgxpool.Pool) (int64, error) {
	var total int64
	err := db.QueryRow(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM ledger_entries").Scan(&total)
	return total, err
}

// Discrepancy — счёт, у которого кэш разошёлся с журналом.
type Discrepancy struct {
	AccountID int64
	Cached    int64 // ledger_accounts.balance
	Derived   int64 // balance_after последней записи счёта
	LastSeq   int64 // ledger_accounts.last_seq
	NewestSeq int64 // account_seq последней записи счёта
}

func (d Discrepancy) String() string {
	return fmt.Sprintf("account %d: кэш=%d журнал=%d, курсор=%d последняя запись=%d",
		d.AccountID, d.Cached, d.Derived, d.LastSeq, d.NewestSeq)
}

// Reconcile сверяет кэш с журналом по двум равенствам:
//
//	balance  == balance_after последней записи счёта
//	last_seq == account_seq последней записи счёта
//
// Одно обращение по индексу на счёт вместо свёртки всей истории:
// O(счетов × log записей) вместо O(всех записей). Именно ради этого
// в записи хранится balance_after.
//
// Журнал остаётся источником истины — расходится кэш, и пересчитывают его.
func Reconcile(ctx context.Context, db *pgxpool.Pool) ([]Discrepancy, error) {
	rows, err := db.Query(ctx, `
		SELECT a.id, a.balance, COALESCE(last.balance_after, 0),
		       a.last_seq, COALESCE(last.account_seq, 0)
		FROM ledger_accounts a
		LEFT JOIN LATERAL (
		    SELECT balance_after, account_seq
		    FROM ledger_entries
		    WHERE account_id = a.id
		    ORDER BY account_seq DESC
		    LIMIT 1
		) last ON true
		WHERE a.balance  <> COALESCE(last.balance_after, 0)
		   OR a.last_seq <> COALESCE(last.account_seq, 0)
		ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Discrepancy
	for rows.Next() {
		var d Discrepancy
		if err := rows.Scan(&d.AccountID, &d.Cached, &d.Derived, &d.LastSeq, &d.NewestSeq); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AuditChain проверяет сам журнал: что цепочка balance_after не разорвана,
// то есть balance_after(n) == balance_after(n-1) + amount(n) для каждой записи.
//
// Это дорого, O(всех записей), и потому отдельно от Reconcile: та сверяет
// кэш с журналом на каждом прогоне, эта проверяет журнал сам по себе —
// изредка, окнами, по расписанию.
func AuditChain(ctx context.Context, db *pgxpool.Pool, accountID int64) error {
	var broken int64
	err := db.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
		    SELECT balance_after,
		           amount,
		           LAG(balance_after) OVER (ORDER BY account_seq) AS prev
		    FROM ledger_entries
		    WHERE account_id = $1
		) chain
		WHERE balance_after <> COALESCE(prev, 0) + amount`, accountID).Scan(&broken)
	if err != nil {
		return err
	}
	if broken > 0 {
		return fmt.Errorf("account %d: цепочка balance_after разорвана в %d записях", accountID, broken)
	}
	return nil
}

// StatementLine — строка выписки.
type StatementLine struct {
	Seq           int64
	TransactionID int64
	Amount        int64
	Balance       int64 // баланс после операции
	Description   string
}

// Statement — страница выписки, операции с номером больше afterSeq.
//
// Постраничность по ключу (account_seq), а не по OFFSET: стоимость страницы
// не зависит от того, насколько она далеко. Нарастающий итог берётся готовым
// из balance_after, без оконной функции по всей истории.
//
// Порядок по account_seq, а не по глобальному id: id выдаётся до взятия
// блокировки счёта, поэтому операции могли получить id в одном порядке,
// а примениться в другом.
func Statement(
	ctx context.Context, db *pgxpool.Pool,
	accountID, afterSeq int64, limit int,
) ([]StatementLine, error) {
	rows, err := db.Query(ctx, `
		SELECT e.account_seq, e.transaction_id, e.amount, e.balance_after, t.description
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE e.account_id = $1 AND e.account_seq > $2
		ORDER BY e.account_seq
		LIMIT $3`, accountID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StatementLine
	for rows.Next() {
		var l StatementLine
		if err := rows.Scan(&l.Seq, &l.TransactionID, &l.Amount, &l.Balance, &l.Description); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
