package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func (service *Service) Reserve(ctx context.Context, accountID, idempotencyKey string, bytes int64) (domain.Reservation, domain.Outcome, error) {
	request := inFlightRequest{accountID: accountID, idempotencyKey: idempotencyKey}
	if !service.inFlight.enter(request) {
		return domain.Reservation{}, 0, domain.ErrRequestInProgress
	}
	defer service.inFlight.leave(request)
	maxActive, known := service.limits.MaxActive()
	candidate := domain.Reservation{
		ID:             uuid.Must(uuid.NewV7()).String(),
		AccountID:      accountID,
		IdempotencyKey: idempotencyKey,
		Bytes:          bytes,
		Status:         domain.StatusActive,
		CreatedAt:      service.now().UTC().Truncate(time.Millisecond),
	}
	return service.accounts.Reserve(ctx, candidate, domain.MaxActive{Value: maxActive, Known: known})
}

func (service *Service) ListActiveReservations(ctx context.Context, accountID string) ([]domain.Reservation, error) {
	if _, err := service.accounts.Account(ctx, accountID); err != nil {
		return nil, err
	}
	return service.accounts.ListActiveReservations(ctx, accountID)
}
