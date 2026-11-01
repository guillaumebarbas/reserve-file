package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

const selectReservation = `SELECT idempotency_key, bytes, status, created_at, released_at FROM reservations WHERE id = ? AND account_id = ?`

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
	reservation, err := scanReservation(accountID, reservationID, transaction.QueryRowContext(ctx, selectReservation, reservationID, accountID))
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
	return scanReservation(accountID, reservationID, store.database.QueryRowContext(ctx, selectReservation, reservationID, accountID))
}

func scanReservation(accountID, reservationID string, row *sql.Row) (domain.Reservation, error) {
	var idempotencyKey, status string
	var bytes, createdAt int64
	var releasedAt sql.NullInt64
	err := row.Scan(&idempotencyKey, &bytes, &status, &createdAt, &releasedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Reservation{}, domain.ErrReservationNotFound
	}
	if err != nil {
		return domain.Reservation{}, translateDriverError("read reservation "+reservationID, err)
	}
	reservation := domain.Reservation{
		ID:             reservationID,
		AccountID:      accountID,
		IdempotencyKey: idempotencyKey,
		Bytes:          bytes,
		Status:         domain.ReservationStatus(status),
		CreatedAt:      time.UnixMilli(createdAt).UTC(),
	}
	if releasedAt.Valid {
		reservation.ReleasedAt = time.UnixMilli(releasedAt.Int64).UTC()
	}
	return reservation, nil
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
