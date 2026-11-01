package service

import (
	"context"
	"errors"
	"testing"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestService_UsageCombinesAccountAndLimit(t *testing.T) {
	account := domain.RestoreAccount("large_account", 10737418240, 4194304, 1)
	testCases := []struct {
		name          string
		limitProvider fakeLimitProvider
		wantMaxActive domain.MaxActive
	}{
		{"known_limit", fakeLimitProvider{maxActive: 5, known: true}, domain.MaxActive{Value: 5, Known: true}},
		{"unknown_limit", fakeLimitProvider{}, domain.MaxActive{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			accounts := fakeAccountRepository{accounts: map[string]domain.Account{"large_account": account}}
			storage := New(accounts, testCase.limitProvider)

			got, err := storage.Usage(t.Context(), "large_account")

			if err != nil {
				t.Fatalf("Usage: unexpected error: %v", err)
			}
			want := Usage{Account: account, MaxActive: testCase.wantMaxActive}
			if got != want {
				t.Errorf("Usage: got %+v, want %+v", got, want)
			}
		})
	}
}

func TestService_UsageReturnsAccountErrors(t *testing.T) {
	storage := New(fakeAccountRepository{}, fakeLimitProvider{maxActive: 5, known: true})

	_, err := storage.Usage(t.Context(), "unknown_account")

	if !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("Usage of an unknown account: got error %v, want domain.ErrAccountNotFound", err)
	}
}

type fakeAccountRepository struct {
	accounts map[string]domain.Account
}

func (repository fakeAccountRepository) Account(ctx context.Context, accountID string) (domain.Account, error) {
	account, found := repository.accounts[accountID]
	if !found {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	return account, nil
}

type fakeLimitProvider struct {
	maxActive int
	known     bool
}

func (provider fakeLimitProvider) MaxActive() (int, bool) {
	return provider.maxActive, provider.known
}
