package sqlite

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

const (
	thirtyMebibytes = 31457280
	fiftyMebibytes  = 52428800
	oneMebibyte     = 1048576
)

func TestReserve_AfterStorageLimitLowered(t *testing.T) {
	store := openStoreWithLimitLowered(t)

	_, _, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_4", "key_reservation_4", oneMebibyte), domain.MaxActive{Value: 5, Known: true})

	wantErr := domain.InsufficientStorageError{RequestedBytes: oneMebibyte, AvailableBytes: 0, LimitBytes: fiftyMebibytes}
	if !errors.Is(err, wantErr) {
		t.Errorf("reserve 1 MiB with 90 MiB reserved under a limit lowered to 50 MiB: got error %v, want %v", err, wantErr)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: fiftyMebibytes, reservedBytes: 3 * thirtyMebibytes, activeReservations: 3}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the limit was lowered: got %+v, want %+v", got, wantAccounts)
	}
	wantStatuses := []string{"active", "active", "active"}
	if got := readReservationStatuses(t, store); !slices.Equal(got, wantStatuses) {
		t.Errorf("reservations after the limit was lowered: got statuses %v, want %v", got, wantStatuses)
	}
	assertInvariants(t, store.database)
}

func TestRelease_BringsUsageBackUnderALoweredLimit(t *testing.T) {
	store := openStoreWithLimitLowered(t)
	maxActive := domain.MaxActive{Value: 5, Known: true}
	releasedAt := time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC)

	_, firstReleaseErr := store.Release(t.Context(), "small_account", "reservation_1", releasedAt)
	_, _, stillAboveErr := store.Reserve(t.Context(), newCandidate("small_account", "reservation_4", "key_reservation_4", oneMebibyte), maxActive)
	_, secondReleaseErr := store.Release(t.Context(), "small_account", "reservation_2", releasedAt)
	_, outcome, fitsAgainErr := store.Reserve(t.Context(), newCandidate("small_account", "reservation_5", "key_reservation_5", 20971520), maxActive)

	if firstReleaseErr != nil || secondReleaseErr != nil {
		t.Fatalf("releases above a lowered limit: got errors %v and %v, want nil", firstReleaseErr, secondReleaseErr)
	}
	wantStillAboveErr := domain.InsufficientStorageError{RequestedBytes: oneMebibyte, AvailableBytes: 0, LimitBytes: fiftyMebibytes}
	if !errors.Is(stillAboveErr, wantStillAboveErr) {
		t.Errorf("reserve 1 MiB with 60 MiB still reserved under 50 MiB: got error %v, want %v", stillAboveErr, wantStillAboveErr)
	}
	if fitsAgainErr != nil || outcome != domain.Created {
		t.Errorf("reserve the 20 MiB left under 50 MiB: got outcome %d and error %v, want created", outcome, fitsAgainErr)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: fiftyMebibytes, reservedBytes: fiftyMebibytes, activeReservations: 2}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after two releases and one reservation: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserve_AfterMaxActiveLowered(t *testing.T) {
	store := openStoreWithFiveActiveReservations(t)

	_, _, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_6", "key_reservation_6", oneMebibyte), domain.MaxActive{Value: 3, Known: true})

	wantErr := domain.TooManyActiveReservationsError{ActiveReservations: 5, MaxActiveReservations: 3}
	if !errors.Is(err, wantErr) {
		t.Errorf("reserve with 5 active reservations and MAX_PARALLEL_CHUNKS lowered to 3: got error %v, want %v", err, wantErr)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 5 * oneMebibyte, activeReservations: 5}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after MAX_PARALLEL_CHUNKS was lowered: got %+v, want %+v", got, wantAccounts)
	}
	wantStatuses := []string{"active", "active", "active", "active", "active"}
	if got := readReservationStatuses(t, store); !slices.Equal(got, wantStatuses) {
		t.Errorf("reservations after MAX_PARALLEL_CHUNKS was lowered: got statuses %v, want %v", got, wantStatuses)
	}
	assertInvariants(t, store.database)
}

func TestRelease_BringsActiveReservationsBackUnderALoweredMaximum(t *testing.T) {
	store := openStoreWithFiveActiveReservations(t)
	loweredMaxActive := domain.MaxActive{Value: 3, Known: true}
	releasedAt := time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC)

	_, firstReleaseErr := store.Release(t.Context(), "small_account", "reservation_1", releasedAt)
	_, secondReleaseErr := store.Release(t.Context(), "small_account", "reservation_2", releasedAt)
	_, _, stillAtMaximumErr := store.Reserve(t.Context(), newCandidate("small_account", "reservation_6", "key_reservation_6", oneMebibyte), loweredMaxActive)
	_, thirdReleaseErr := store.Release(t.Context(), "small_account", "reservation_3", releasedAt)
	_, outcome, fitsAgainErr := store.Reserve(t.Context(), newCandidate("small_account", "reservation_7", "key_reservation_7", oneMebibyte), loweredMaxActive)

	if firstReleaseErr != nil || secondReleaseErr != nil || thirdReleaseErr != nil {
		t.Fatalf("releases above a lowered maximum: got errors %v, %v and %v, want nil", firstReleaseErr, secondReleaseErr, thirdReleaseErr)
	}
	wantStillAtMaximumErr := domain.TooManyActiveReservationsError{ActiveReservations: 3, MaxActiveReservations: 3}
	if !errors.Is(stillAtMaximumErr, wantStillAtMaximumErr) {
		t.Errorf("reserve with 3 active reservations and a maximum of 3: got error %v, want %v", stillAtMaximumErr, wantStillAtMaximumErr)
	}
	if fitsAgainErr != nil || outcome != domain.Created {
		t.Errorf("reserve with 2 active reservations and a maximum of 3: got outcome %d and error %v, want created", outcome, fitsAgainErr)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 3 * oneMebibyte, activeReservations: 3}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after three releases and one reservation: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func openStoreWithLimitLowered(t *testing.T) *Store {
	t.Helper()
	path := newDatabasePath(t)
	storeBeforeRestart := openStore(t, path, map[string]int64{"small_account": 104857600})
	for _, reservationID := range []string{"reservation_1", "reservation_2", "reservation_3"} {
		reserveForTest(t, storeBeforeRestart, "small_account", reservationID, thirtyMebibytes)
	}
	_ = storeBeforeRestart.Close()
	return openStore(t, path, map[string]int64{"small_account": fiftyMebibytes})
}

func openStoreWithFiveActiveReservations(t *testing.T) *Store {
	t.Helper()
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	for reservationNumber := 1; reservationNumber <= 5; reservationNumber++ {
		reserveForTest(t, store, "small_account", fmt.Sprintf("reservation_%d", reservationNumber), oneMebibyte)
	}
	return store
}

func readReservationStatuses(t *testing.T, store *Store) []string {
	t.Helper()
	var statuses []string
	for _, reservation := range readStoredReservations(t, store) {
		statuses = append(statuses, reservation.status)
	}
	return statuses
}
