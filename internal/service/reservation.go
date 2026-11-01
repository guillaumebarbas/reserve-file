package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func (service *Service) Reserve(ctx context.Context, accountID, idempotencyKey string, bytes int64) (domain.Reservation, error) {
	maxActive, known := service.limits.MaxActive()
	candidate := domain.Reservation{
		ID:             uuid.Must(uuid.NewV7()).String(),
		AccountID:      accountID,
		IdempotencyKey: idempotencyKey,
		Bytes:          bytes,
		Status:         domain.StatusActive,
		CreatedAt:      service.now().UTC().Truncate(time.Millisecond),
	}
	if err := service.accounts.Reserve(ctx, candidate, domain.MaxActive{Value: maxActive, Known: known}); err != nil {
		return domain.Reservation{}, err
	}
	return candidate, nil
}
