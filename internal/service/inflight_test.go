package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestService_DuplicateInFlightReturnsInProgress(t *testing.T) {
	accounts := newBlockingAccountRepository("small_account", "key_1")
	storage := newGuardedService(accounts)
	firstResult := reserveInBackground(storage, "small_account", "key_1")
	<-accounts.entered

	_, _, duplicateErr := storage.Reserve(t.Context(), "small_account", "key_1", 100)
	callsWhileInFlight := accounts.reserveCalls.Load()
	close(accounts.release)
	firstErr := <-firstResult
	_, _, retryErr := storage.Reserve(t.Context(), "small_account", "key_1", 100)

	if !errors.Is(duplicateErr, domain.ErrRequestInProgress) {
		t.Errorf("Reserve while the same key is in flight: got error %v, want domain.ErrRequestInProgress", duplicateErr)
	}
	if callsWhileInFlight != 1 {
		t.Errorf("repository calls while the first request is in flight: got %d, want 1", callsWhileInFlight)
	}
	if firstErr != nil {
		t.Errorf("first Reserve: got error %v, want nil", firstErr)
	}
	if retryErr != nil || accounts.reserveCalls.Load() != 2 {
		t.Errorf("Reserve after the first one finished: got error %v and %d repository calls, want nil and 2", retryErr, accounts.reserveCalls.Load())
	}
}

func TestService_DifferentKeysAreNotBlocked(t *testing.T) {
	accounts := newBlockingAccountRepository("small_account", "key_1")
	storage := newGuardedService(accounts)
	firstResult := reserveInBackground(storage, "small_account", "key_1")
	<-accounts.entered

	_, _, otherKeyErr := storage.Reserve(t.Context(), "small_account", "key_2", 100)
	close(accounts.release)
	firstErr := <-firstResult

	if otherKeyErr != nil {
		t.Errorf("Reserve with another key while key_1 is in flight: got error %v, want nil", otherKeyErr)
	}
	if firstErr != nil {
		t.Errorf("first Reserve: got error %v, want nil", firstErr)
	}
}

func TestService_SameKeyOnDifferentAccountsIsNotBlocked(t *testing.T) {
	accounts := newBlockingAccountRepository("small_account", "key_1")
	storage := newGuardedService(accounts)
	firstResult := reserveInBackground(storage, "small_account", "key_1")
	<-accounts.entered

	_, _, otherAccountErr := storage.Reserve(t.Context(), "large_account", "key_1", 100)
	close(accounts.release)
	firstErr := <-firstResult

	if otherAccountErr != nil {
		t.Errorf("Reserve with key_1 on another account while it is in flight on small_account: got error %v, want nil", otherAccountErr)
	}
	if firstErr != nil {
		t.Errorf("first Reserve: got error %v, want nil", firstErr)
	}
}

func TestService_GuardReleasedAfterError(t *testing.T) {
	accounts := &fakeAccountRepository{reserveErr: domain.ErrServiceUnavailable}
	storage := newGuardedService(accounts)

	_, _, firstErr := storage.Reserve(t.Context(), "small_account", "key_1", 100)
	_, _, retryErr := storage.Reserve(t.Context(), "small_account", "key_1", 100)

	if !errors.Is(firstErr, domain.ErrServiceUnavailable) {
		t.Errorf("first Reserve: got error %v, want domain.ErrServiceUnavailable", firstErr)
	}
	if !errors.Is(retryErr, domain.ErrServiceUnavailable) {
		t.Errorf("retry after a failed Reserve: got error %v, want the repository's domain.ErrServiceUnavailable, not a key still in flight", retryErr)
	}
}

func TestService_GuardReleasedAfterPanic(t *testing.T) {
	accounts := &panickingAccountRepository{}
	storage := newGuardedService(accounts)

	reservePanics(t, storage, "small_account", "key_1")
	reservePanics(t, storage, "small_account", "key_1")

	if accounts.reserveCalls != 2 {
		t.Errorf("repository calls: got %d, want 2: the retry after a panic must reach the repository", accounts.reserveCalls)
	}
}

func newGuardedService(accounts AccountRepository) *Service {
	return New(accounts, fakeLimitProvider{maxActive: 5, known: true}, fixedClock(time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)))
}

func reserveInBackground(storage *Service, accountID, idempotencyKey string) <-chan error {
	result := make(chan error, 1)
	go func() {
		_, _, err := storage.Reserve(context.Background(), accountID, idempotencyKey, 100)
		result <- err
	}()
	return result
}

func reservePanics(t *testing.T, storage *Service, accountID, idempotencyKey string) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("Reserve(%s, %s): got no panic, want the repository's panic", accountID, idempotencyKey)
		}
	}()
	_, _, _ = storage.Reserve(t.Context(), accountID, idempotencyKey, 100)
}

type blockingAccountRepository struct {
	blockedAccountID      string
	blockedIdempotencyKey string
	entered               chan struct{}
	release               chan struct{}
	firstCallBlocked      atomic.Bool
	reserveCalls          atomic.Int32
}

func newBlockingAccountRepository(blockedAccountID, blockedIdempotencyKey string) *blockingAccountRepository {
	return &blockingAccountRepository{
		blockedAccountID:      blockedAccountID,
		blockedIdempotencyKey: blockedIdempotencyKey,
		entered:               make(chan struct{}),
		release:               make(chan struct{}),
	}
}

func (repository *blockingAccountRepository) Reserve(ctx context.Context, candidate domain.Reservation, maxActive domain.MaxActive) (domain.Reservation, domain.Outcome, error) {
	repository.reserveCalls.Add(1)
	isBlockedPair := candidate.AccountID == repository.blockedAccountID && candidate.IdempotencyKey == repository.blockedIdempotencyKey
	if isBlockedPair && repository.firstCallBlocked.CompareAndSwap(false, true) {
		close(repository.entered)
		<-repository.release
	}
	return candidate, domain.Created, nil
}

func (repository *blockingAccountRepository) Account(ctx context.Context, accountID string) (domain.Account, error) {
	return domain.Account{}, domain.ErrAccountNotFound
}

func (repository *blockingAccountRepository) Release(ctx context.Context, accountID, reservationID string, releasedAt time.Time) (domain.Reservation, error) {
	return domain.Reservation{}, domain.ErrReservationNotFound
}

func (repository *blockingAccountRepository) Reservation(ctx context.Context, accountID, reservationID string) (domain.Reservation, error) {
	return domain.Reservation{}, domain.ErrReservationNotFound
}

func (repository *blockingAccountRepository) ListActiveReservations(ctx context.Context, accountID string) ([]domain.Reservation, error) {
	return nil, nil
}

type panickingAccountRepository struct {
	reserveCalls int
}

func (repository *panickingAccountRepository) Reserve(ctx context.Context, candidate domain.Reservation, maxActive domain.MaxActive) (domain.Reservation, domain.Outcome, error) {
	repository.reserveCalls++
	panic("bug in the repository")
}

func (repository *panickingAccountRepository) Account(ctx context.Context, accountID string) (domain.Account, error) {
	return domain.Account{}, domain.ErrAccountNotFound
}

func (repository *panickingAccountRepository) Release(ctx context.Context, accountID, reservationID string, releasedAt time.Time) (domain.Reservation, error) {
	return domain.Reservation{}, domain.ErrReservationNotFound
}

func (repository *panickingAccountRepository) Reservation(ctx context.Context, accountID, reservationID string) (domain.Reservation, error) {
	return domain.Reservation{}, domain.ErrReservationNotFound
}

func (repository *panickingAccountRepository) ListActiveReservations(ctx context.Context, accountID string) ([]domain.Reservation, error) {
	return nil, nil
}
