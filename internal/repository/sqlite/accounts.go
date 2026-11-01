package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

const selectAccount = `SELECT storage_limit_bytes, reserved_bytes, active_reservations FROM accounts WHERE id = ?`

func (store *Store) Account(ctx context.Context, accountID string) (domain.Account, error) {
	return scanAccount(accountID, store.database.QueryRowContext(ctx, selectAccount, accountID))
}

func scanAccount(accountID string, row *sql.Row) (domain.Account, error) {
	var limitBytes, reservedBytes int64
	var activeReservations int
	err := row.Scan(&limitBytes, &reservedBytes, &activeReservations)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	if err != nil {
		return domain.Account{}, translateDriverError("read account "+accountID, err)
	}
	return domain.RestoreAccount(accountID, limitBytes, reservedBytes, activeReservations), nil
}
