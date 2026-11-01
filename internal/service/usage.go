package service

import (
	"context"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

type Usage struct {
	Account   domain.Account
	MaxActive domain.MaxActive
}

func (service *Service) Usage(ctx context.Context, accountID string) (Usage, error) {
	account, err := service.accounts.Account(ctx, accountID)
	if err != nil {
		return Usage{}, err
	}
	maxActive, known := service.limits.MaxActive()
	return Usage{Account: account, MaxActive: domain.MaxActive{Value: maxActive, Known: known}}, nil
}
