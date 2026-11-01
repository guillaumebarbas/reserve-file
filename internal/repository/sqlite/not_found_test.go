package sqlite

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestRelease_OtherAccountIsNotFound(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"large_account": 10737418240, "small_account": 104857600})
	reserveForTest(t, store, "large_account", "reservation_of_large_account", 31457280)

	_, err := store.Release(t.Context(), "small_account", "reservation_of_large_account", time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC))

	if !errors.Is(err, domain.ErrReservationNotFound) {
		t.Errorf("Release of large_account's reservation through small_account: got error %v, want domain.ErrReservationNotFound", err)
	}
	if status, releasedAt := readReleaseState(t, store, "reservation_of_large_account"); status != "active" || releasedAt.Valid {
		t.Errorf("reservation_of_large_account: got status %q released at %v, want it still active", status, releasedAt)
	}
	wantAccounts := []storedAccount{
		{id: "large_account", limitBytes: 10737418240, reservedBytes: 31457280, activeReservations: 1},
		{id: "small_account", limitBytes: 104857600},
	}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestGet_OtherAccountIsNotFound(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"large_account": 10737418240, "small_account": 104857600})
	reserveForTest(t, store, "large_account", "reservation_of_large_account", 31457280)

	_, err := store.Reservation(t.Context(), "small_account", "reservation_of_large_account")

	if !errors.Is(err, domain.ErrReservationNotFound) {
		t.Errorf("Reservation of large_account's reservation through small_account: got error %v, want domain.ErrReservationNotFound", err)
	}
}
