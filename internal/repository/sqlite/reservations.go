package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

const (
	selectReservation      = `SELECT id, account_id, idempotency_key, bytes, status, created_at, released_at FROM reservations WHERE id = ? AND account_id = ?`
	selectReservationByKey = `SELECT id, account_id, idempotency_key, bytes, status, created_at, released_at FROM reservations WHERE account_id = ? AND idempotency_key = ?`
)

func (store *Store) Reserve(ctx context.Context, candidate domain.Reservation, maxActive domain.MaxActive) (domain.Reservation, domain.Outcome, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return domain.Reservation{}, 0, translateDriverError("begin reserve", err)
	}
	defer transaction.Rollback()
	account, err := scanAccount(candidate.AccountID, transaction.QueryRowContext(ctx, selectAccount, candidate.AccountID))
	if err != nil {
		return domain.Reservation{}, 0, err
	}
	existing, err := findReservationByKey(ctx, transaction, candidate.AccountID, candidate.IdempotencyKey)
	if err != nil {
		return domain.Reservation{}, 0, err
	}
	reservation, outcome, err := account.Reserve(candidate, existing, maxActive)
	if err != nil {
		return domain.Reservation{}, 0, err
	}
	if outcome == domain.Replayed {
		return reservation, outcome, nil
	}
	if err := insertReservation(ctx, transaction, reservation); err != nil {
		return domain.Reservation{}, 0, err
	}
	if err := saveCounters(ctx, transaction, account); err != nil {
		return domain.Reservation{}, 0, err
	}
	return reservation, outcome, translateDriverError("commit reserve", transaction.Commit())
}

func findReservationByKey(ctx context.Context, transaction *sql.Tx, accountID, idempotencyKey string) (*domain.Reservation, error) {
	reservation, err := scanReservation(transaction.QueryRowContext(ctx, selectReservationByKey, accountID, idempotencyKey))
	if errors.Is(err, domain.ErrReservationNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &reservation, nil
}

func scanReservation(row *sql.Row) (domain.Reservation, error) {
	var reservationID, accountID, idempotencyKey, status string
	var bytes, createdAt int64
	var releasedAt sql.NullInt64
	err := row.Scan(&reservationID, &accountID, &idempotencyKey, &bytes, &status, &createdAt, &releasedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Reservation{}, domain.ErrReservationNotFound
	}
	if err != nil {
		return domain.Reservation{}, translateDriverError("read reservation", err)
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

func insertReservation(ctx context.Context, transaction *sql.Tx, reservation domain.Reservation) error {
	const insert = `INSERT INTO reservations (id, account_id, idempotency_key, bytes, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := transaction.ExecContext(ctx, insert,
		reservation.ID, reservation.AccountID, reservation.IdempotencyKey, reservation.Bytes, string(reservation.Status), reservation.CreatedAt.UnixMilli())
	return translateDriverError("insert reservation", err)
}

func saveCounters(ctx context.Context, transaction *sql.Tx, account domain.Account) error {
	const update = `UPDATE accounts SET reserved_bytes = ?, active_reservations = ? WHERE id = ?`
	_, err := transaction.ExecContext(ctx, update, account.ReservedBytes(), account.ActiveReservations(), account.ID)
	return translateDriverError("save account counters", err)
}

func translateDriverError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %v", domain.ErrServiceUnavailable, action, err)
}
