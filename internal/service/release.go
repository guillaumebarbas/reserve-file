package service

import (
	"context"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func (service *Service) Release(ctx context.Context, accountID, reservationID string) (domain.Reservation, error) {
	releasedAt := service.now().UTC().Truncate(time.Millisecond)
	return service.accounts.Release(ctx, accountID, reservationID, releasedAt)
}

func (service *Service) Reservation(ctx context.Context, accountID, reservationID string) (domain.Reservation, error) {
	return service.accounts.Reservation(ctx, accountID, reservationID)
}
