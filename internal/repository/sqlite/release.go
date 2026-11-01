package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func (store *Store) Release(ctx context.Context, accountID, reservationID string, releasedAt time.Time) (domain.Reservation, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return domain.Reservation{}, translateDriverError("begin release", err)
	}
	defer transaction.Rollback()
	account, err := scanAccount(accountID, transaction.QueryRowContext(ctx, selectAccount, accountID))
	if err != nil {
		return domain.Reservation{}, err
	}
	reservation, err := scanReservation(transaction.QueryRowContext(ctx, selectReservation, reservationID, accountID))
	if err != nil {
		return domain.Reservation{}, err
	}
	account.Release(&reservation, releasedAt)
	if err := saveReleasedReservation(ctx, transaction, reservation); err != nil {
		return domain.Reservation{}, err
	}
	if err := saveCounters(ctx, transaction, account); err != nil {
		return domain.Reservation{}, err
	}
	return reservation, translateDriverError("commit release", transaction.Commit())
}

func (store *Store) Reservation(ctx context.Context, accountID, reservationID string) (domain.Reservation, error) {
	return scanReservation(store.database.QueryRowContext(ctx, selectReservation, reservationID, accountID))
}

func saveReleasedReservation(ctx context.Context, transaction *sql.Tx, reservation domain.Reservation) error {
	const update = `UPDATE reservations SET status = ?, released_at = ? WHERE id = ? AND account_id = ? AND status = 'active'`
	result, err := transaction.ExecContext(ctx, update, string(reservation.Status), reservation.ReleasedAt.UnixMilli(), reservation.ID, reservation.AccountID)
	if err != nil {
		return translateDriverError("save released reservation", err)
	}
	changedRows, _ := result.RowsAffected()
	if changedRows != 1 {
		return fmt.Errorf("release reservation %s: it is no longer active", reservation.ID)
	}
	return nil
}
