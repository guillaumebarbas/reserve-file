package service

import (
	"context"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

type AccountRepository interface {
	Account(ctx context.Context, accountID string) (domain.Account, error)
	Reserve(ctx context.Context, candidate domain.Reservation, maxActive domain.MaxActive) (domain.Reservation, domain.Outcome, error)
	Release(ctx context.Context, accountID, reservationID string, releasedAt time.Time) (domain.Reservation, error)
	Reservation(ctx context.Context, accountID, reservationID string) (domain.Reservation, error)
	ListActiveReservations(ctx context.Context, accountID string) ([]domain.Reservation, error)
}

type LimitProvider interface {
	MaxActive() (int, bool)
}
