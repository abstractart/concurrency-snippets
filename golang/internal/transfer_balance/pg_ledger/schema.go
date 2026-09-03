package pg_ledger

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaDDL = `
CREATE TABLE IF NOT EXISTS ledger_accounts (
    id       BIGSERIAL PRIMARY KEY,
    name     TEXT     NOT NULL,
    currency CHAR(3)  NOT NULL,

    -- Кэш баланса. Источник истины — SUM(ledger_entries.amount);
    -- поле существует потому, что суммировать всю историю на каждом
    -- чтении — O(операций за всё время жизни счёта).
    balance  BIGINT   NOT NULL DEFAULT 0,

    -- Номер последней операции счёта, учтённой в balance.
    -- Номера выдаются под блокировкой строки счёта (см. applyToBalances),
    -- поэтому нумерация сплошная и совпадает с порядком применения операций.
    -- Отсюда два проверяемых равенства: last_seq == число записей счёта,
    -- balance == их сумма (см. Reconcile).
    last_seq BIGINT   NOT NULL DEFAULT 0,

    -- Системным счетам минус разрешён: из них деньги вводятся в систему,
    -- и именно поэтому сумма всего журнала остаётся нулевой.
    allow_negative BOOLEAN NOT NULL DEFAULT FALSE,

    CONSTRAINT ledger_accounts_balance_sign
        CHECK (allow_negative OR balance >= 0)
);

CREATE TABLE IF NOT EXISTS ledger_transactions (
    id              BIGSERIAL   PRIMARY KEY,

    -- Сеть ненадёжна, клиент повторяет запрос, не зная, дошёл ли первый.
    -- UNIQUE превращает повтор в no-op вместо второго списания.
    idempotency_key TEXT        NOT NULL,

    description     TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Имя задано явно: код отличает гонку по ключу идемпотентности
    -- от любого другого нарушения уникальности именно по нему.
    CONSTRAINT ledger_transactions_idempotency_key UNIQUE (idempotency_key)
);

CREATE TABLE IF NOT EXISTS ledger_entries (
    id             BIGSERIAL   PRIMARY KEY,
    transaction_id BIGINT      NOT NULL REFERENCES ledger_transactions(id),
    account_id     BIGINT      NOT NULL REFERENCES ledger_accounts(id),

    -- Порядковый номер операции ВНУТРИ СЧЁТА: 1, 2, 3, ... без пропусков.
    -- Сортировать выписку по глобальному id нельзя: он выдаётся до взятия
    -- блокировки счёта и потому не совпадает с порядком применения.
    account_seq    BIGINT      NOT NULL,

    -- Минорные единицы (копейки). Никогда не float: деньги должны сходиться.
    amount         BIGINT      NOT NULL,

    -- Баланс счёта сразу после этой операции.
    --
    -- Достаётся бесплатно: тот же UPDATE, что выдаёт account_seq, уже вернул
    -- новый баланс. Зато превращает журнал в цепочку контрольных точек:
    -- баланс счёта — это balance_after последней записи, а не SUM по истории.
    -- Чтение баланса из журнала становится O(1) по индексу вместо O(операций),
    -- и выписка обходится без оконной функции.
    --
    -- Цепочка самопроверяема: balance_after(n) = balance_after(n-1) + amount(n).
    balance_after  BIGINT      NOT NULL,
    currency       CHAR(3)     NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT ledger_entries_amount_nonzero CHECK (amount <> 0),
    CONSTRAINT ledger_entries_seq_positive   CHECK (account_seq > 0),
    CONSTRAINT ledger_entries_seq_unique     UNIQUE (account_id, account_seq)
);

CREATE INDEX IF NOT EXISTS ledger_entries_account_idx
    ON ledger_entries (account_id, account_seq);
CREATE INDEX IF NOT EXISTS ledger_entries_transaction_idx
    ON ledger_entries (transaction_id);

-- Инвариант двойной записи: сумма записей проводки равна нулю.
-- В остальных примерах это свойство проверялось тестом, здесь его
-- гарантирует схема.
--
-- DEFERRABLE обязателен: проверка откладывается до COMMIT, иначе первая же
-- половина перевода падала бы — в этот момент сумма ещё не ноль.
CREATE OR REPLACE FUNCTION ledger_assert_balanced() RETURNS TRIGGER AS $$
DECLARE
    total BIGINT;
BEGIN
    SELECT COALESCE(SUM(amount), 0) INTO total
    FROM ledger_entries WHERE transaction_id = NEW.transaction_id;

    IF total <> 0 THEN
        RAISE EXCEPTION 'unbalanced transaction %: entries sum to %',
            NEW.transaction_id, total;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS ledger_entries_balanced ON ledger_entries;
CREATE CONSTRAINT TRIGGER ledger_entries_balanced
    AFTER INSERT ON ledger_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION ledger_assert_balanced();

-- Журнал только дописывается. Ошибочную проводку сторнируют (Reverse),
-- а не правят: аудит должен видеть и ошибку, и её исправление.
CREATE OR REPLACE FUNCTION ledger_entries_immutable() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'ledger_entries is append-only: % is not allowed', TG_OP;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS ledger_entries_no_mutation ON ledger_entries;
CREATE TRIGGER ledger_entries_no_mutation
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_immutable();
`

// SetupSchema создаёт схему. Не мигрирует: CREATE TABLE IF NOT EXISTS
// не тронет таблицу, созданную по старой схеме.
func SetupSchema(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, schemaDDL)
	return err
}

// ResetSchema пересоздаёт схему, удаляя все данные. Для демо и тестов.
func ResetSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		DROP TABLE IF EXISTS ledger_entries, ledger_transactions, ledger_accounts CASCADE`); err != nil {
		return err
	}
	return SetupSchema(ctx, db)
}
