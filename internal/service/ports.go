package service

import (
	"context"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

type AccountRepository interface {
	Account(ctx context.Context, accountID string) (domain.Account, error)
	Reserve(ctx context.Context, candidate domain.Reservation, maxActive domain.MaxActive) (domain.Reservation, domain.Outcome, error)
}

type LimitProvider interface {
	MaxActive() (int, bool)
}
