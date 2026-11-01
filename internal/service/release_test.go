package service

import (
	"context"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestService_ReleaseDoesNotUseLimit(t *testing.T) {
	releasedReservation := domain.Reservation{ID: "reservation_1", AccountID: "small_account", Bytes: 31457280, Status: domain.StatusReleased}
	accounts := &fakeAccountRepository{storedReservation: releasedReservation}
	clockTime := time.Date(2026, 11, 1, 13, 3, 10, 987654321, time.FixedZone("CET", 3600))
	storage := New(accounts, limitProviderNeverCalled{t: t}, fixedClock(clockTime))

	got, err := storage.Release(t.Context(), "small_account", "reservation_1")

	if err != nil {
		t.Fatalf("Release: unexpected error: %v", err)
	}
	if got != releasedReservation {
		t.Errorf("Release: got %+v, want the repository's %+v", got, releasedReservation)
	}
	wantRequest := releaseRequest{accountID: "small_account", reservationID: "reservation_1", releasedAt: time.Date(2026, 11, 1, 12, 3, 10, 987000000, time.UTC)}
	if accounts.releaseRequest != wantRequest {
		t.Errorf("release passed to the repository: got %+v, want %+v: the clock's time in UTC, to the millisecond", accounts.releaseRequest, wantRequest)
	}
}

func TestService_ReservationReturnsStoredState(t *testing.T) {
	storedReservation := domain.Reservation{ID: "reservation_1", AccountID: "small_account", Bytes: 31457280, Status: domain.StatusActive}
	storage := New(&fakeAccountRepository{storedReservation: storedReservation}, limitProviderNeverCalled{t: t}, time.Now)

	got, err := storage.Reservation(t.Context(), "small_account", "reservation_1")

	if err != nil || got != storedReservation {
		t.Errorf("Reservation: got %+v and error %v, want the repository's %+v", got, err, storedReservation)
	}
}

type releaseRequest struct {
	accountID     string
	reservationID string
	releasedAt    time.Time
}

func (repository *fakeAccountRepository) Release(ctx context.Context, accountID, reservationID string, releasedAt time.Time) (domain.Reservation, error) {
	repository.releaseRequest = releaseRequest{accountID: accountID, reservationID: reservationID, releasedAt: releasedAt}
	return repository.storedReservation, nil
}

func (repository *fakeAccountRepository) Reservation(ctx context.Context, accountID, reservationID string) (domain.Reservation, error) {
	return repository.storedReservation, nil
}

type limitProviderNeverCalled struct {
	t *testing.T
}

func (provider limitProviderNeverCalled) MaxActive() (int, bool) {
	provider.t.Error("the limit provider was called")
	return 0, false
}
