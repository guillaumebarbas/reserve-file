package service

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestService_PassesCurrentLimit(t *testing.T) {
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
			accounts := &fakeAccountRepository{}
			storage := New(accounts, testCase.limitProvider, time.Now)

			_, _, err := storage.Reserve(t.Context(), "small_account", "key_1", 4194304)

			if err != nil {
				t.Fatalf("Reserve: unexpected error: %v", err)
			}
			if accounts.reservedMaxActive != testCase.wantMaxActive {
				t.Errorf("limit passed to the repository: got %+v, want %+v", accounts.reservedMaxActive, testCase.wantMaxActive)
			}
		})
	}
}

func TestService_BuildsCandidate(t *testing.T) {
	accounts := &fakeAccountRepository{}
	clockTime := time.Date(2026, 11, 1, 12, 0, 0, 123456789, time.FixedZone("CET", 3600))
	storage := New(accounts, fakeLimitProvider{maxActive: 5, known: true}, fixedClock(clockTime))

	reservation, _, err := storage.Reserve(t.Context(), "small_account", "key_1", 4194304)

	if err != nil {
		t.Fatalf("Reserve: unexpected error: %v", err)
	}
	candidate := accounts.reservedCandidate
	if identifier, parseErr := uuid.Parse(candidate.ID); parseErr != nil || identifier.Version() != 7 {
		t.Errorf("candidate ID %q: want a UUIDv7", candidate.ID)
	}
	if candidate.AccountID != "small_account" || candidate.IdempotencyKey != "key_1" || candidate.Bytes != 4194304 || candidate.Status != domain.StatusActive {
		t.Errorf("candidate: got %+v, want small_account, key_1, 4194304 bytes and status active", candidate)
	}
	wantCreatedAt := time.Date(2026, 11, 1, 11, 0, 0, 123000000, time.UTC)
	if !candidate.CreatedAt.Equal(wantCreatedAt) || candidate.CreatedAt.Location() != time.UTC {
		t.Errorf("candidate created at %v, want %v: the clock's time in UTC, to the millisecond", candidate.CreatedAt, wantCreatedAt)
	}
	if reservation.ID != candidate.ID {
		t.Errorf("returned reservation %q, want the candidate %q", reservation.ID, candidate.ID)
	}
}

func TestService_ReserveReturnsRepositoryErrors(t *testing.T) {
	accounts := &fakeAccountRepository{reserveErr: domain.ErrAccountNotFound}
	storage := New(accounts, fakeLimitProvider{maxActive: 5, known: true}, time.Now)

	_, _, err := storage.Reserve(t.Context(), "unknown_account", "key_1", 4194304)

	if !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("Reserve on an unknown account: got error %v, want domain.ErrAccountNotFound", err)
	}
}

func TestService_ListActiveReservationsChecksTheAccount(t *testing.T) {
	activeReservations := []domain.Reservation{
		{ID: "reservation_1", AccountID: "small_account", Bytes: 4194304, Status: domain.StatusActive},
		{ID: "reservation_2", AccountID: "small_account", Bytes: 31457280, Status: domain.StatusActive},
	}
	accounts := &fakeAccountRepository{
		accounts:           map[string]domain.Account{"small_account": domain.RestoreAccount("small_account", 104857600, 35651584, 2)},
		activeReservations: activeReservations,
	}
	storage := New(accounts, limitProviderNeverCalled{t: t}, time.Now)

	_, unknownAccountErr := storage.ListActiveReservations(t.Context(), "unknown_account")
	got, err := storage.ListActiveReservations(t.Context(), "small_account")

	if !errors.Is(unknownAccountErr, domain.ErrAccountNotFound) {
		t.Errorf("ListActiveReservations of unknown_account: got error %v, want domain.ErrAccountNotFound", unknownAccountErr)
	}
	if err != nil || !slices.Equal(got, activeReservations) {
		t.Errorf("ListActiveReservations of small_account: got %+v and error %v, want the repository's %+v", got, err, activeReservations)
	}
	if !slices.Equal(accounts.listedAccountIDs, []string{"small_account"}) {
		t.Errorf("accounts whose list was read: got %v, want only small_account: an unknown account is refused before any read", accounts.listedAccountIDs)
	}
}

func (repository *fakeAccountRepository) ListActiveReservations(ctx context.Context, accountID string) ([]domain.Reservation, error) {
	repository.listedAccountIDs = append(repository.listedAccountIDs, accountID)
	return repository.activeReservations, nil
}
