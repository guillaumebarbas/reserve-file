package service

import (
	"context"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

type fakeAccountRepository struct {
	accounts          map[string]domain.Account
	reserveErr        error
	reservedCandidate domain.Reservation
	reservedMaxActive domain.MaxActive
	storedReservation domain.Reservation
	releaseRequest    releaseRequest
}

func (repository *fakeAccountRepository) Account(ctx context.Context, accountID string) (domain.Account, error) {
	account, found := repository.accounts[accountID]
	if !found {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	return account, nil
}

func (repository *fakeAccountRepository) Reserve(ctx context.Context, candidate domain.Reservation, maxActive domain.MaxActive) (domain.Reservation, domain.Outcome, error) {
	repository.reservedCandidate = candidate
	repository.reservedMaxActive = maxActive
	return candidate, domain.Created, repository.reserveErr
}

type fakeLimitProvider struct {
	maxActive int
	known     bool
}

func (provider fakeLimitProvider) MaxActive() (int, bool) {
	return provider.maxActive, provider.known
}

func fixedClock(moment time.Time) func() time.Time {
	return func() time.Time { return moment }
}
