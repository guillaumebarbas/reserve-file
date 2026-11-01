package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func (store *Store) Account(ctx context.Context, accountID string) (domain.Account, error) {
	const selectAccount = `SELECT storage_limit_bytes, reserved_bytes, active_reservations FROM accounts WHERE id = ?`
	var limitBytes, reservedBytes int64
	var activeReservations int
	err := store.database.QueryRowContext(ctx, selectAccount, accountID).Scan(&limitBytes, &reservedBytes, &activeReservations)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("%w: read account %s: %v", domain.ErrServiceUnavailable, accountID, err)
	}
	return domain.RestoreAccount(accountID, limitBytes, reservedBytes, activeReservations), nil
}
