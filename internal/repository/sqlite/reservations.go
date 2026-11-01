package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func (store *Store) Reserve(ctx context.Context, candidate domain.Reservation, maxActive domain.MaxActive) error {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return translateDriverError("begin reserve", err)
	}
	defer transaction.Rollback()
	account, err := readAccount(ctx, transaction, candidate.AccountID)
	if err != nil {
		return err
	}
	if err := account.Reserve(candidate, maxActive); err != nil {
		return err
	}
	if err := insertReservation(ctx, transaction, candidate); err != nil {
		return err
	}
	if err := saveCounters(ctx, transaction, account); err != nil {
		return err
	}
	return translateDriverError("commit reserve", transaction.Commit())
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
