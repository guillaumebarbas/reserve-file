package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

const selectReservationByKey = `SELECT id, bytes, status, created_at FROM reservations WHERE account_id = ? AND idempotency_key = ?`

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
	var reservationID, status string
	var bytes, createdAtMilliseconds int64
	err := transaction.QueryRowContext(ctx, selectReservationByKey, accountID, idempotencyKey).Scan(&reservationID, &bytes, &status, &createdAtMilliseconds)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, translateDriverError("find the reservation of key "+idempotencyKey, err)
	}
	return &domain.Reservation{
		ID:             reservationID,
		AccountID:      accountID,
		IdempotencyKey: idempotencyKey,
		Bytes:          bytes,
		Status:         domain.ReservationStatus(status),
		CreatedAt:      time.UnixMilli(createdAtMilliseconds).UTC(),
	}, nil
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
