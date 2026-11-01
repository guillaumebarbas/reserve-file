package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, arguments ...any) *sql.Row
}

func (store *Store) Account(ctx context.Context, accountID string) (domain.Account, error) {
	return readAccount(ctx, store.database, accountID)
}

func readAccount(ctx context.Context, querier rowQuerier, accountID string) (domain.Account, error) {
	const selectAccount = `SELECT storage_limit_bytes, reserved_bytes, active_reservations FROM accounts WHERE id = ?`
	var limitBytes, reservedBytes int64
	var activeReservations int
	err := querier.QueryRowContext(ctx, selectAccount, accountID).Scan(&limitBytes, &reservedBytes, &activeReservations)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	if err != nil {
		return domain.Account{}, translateDriverError("read account "+accountID, err)
	}
	return domain.RestoreAccount(accountID, limitBytes, reservedBytes, activeReservations), nil
}
