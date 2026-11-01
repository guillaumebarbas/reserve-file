package sqlite

import (
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestRelease_ActiveReservation(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	candidate := reserveForTest(t, store, "small_account", "reservation_1", 31457280)
	releasedAt := time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC)

	released, err := store.Release(t.Context(), "small_account", "reservation_1", releasedAt)

	if err != nil {
		t.Fatalf("Release: unexpected error: %v", err)
	}
	wantReleased := candidate
	wantReleased.Status = domain.StatusReleased
	wantReleased.ReleasedAt = releasedAt
	assertSameReservation(t, released, wantReleased)
	status, storedReleasedAt := readReleaseState(t, store, "reservation_1")
	if status != "released" || storedReleasedAt != (sql.NullInt64{Int64: releasedAt.UnixMilli(), Valid: true}) {
		t.Errorf("stored reservation: got status %q released at %v, want released at %d", status, storedReleasedAt, releasedAt.UnixMilli())
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the release: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestGet_ReturnsCurrentState(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	candidate := reserveForTest(t, store, "small_account", "reservation_1", 31457280)
	releasedAt := time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC)

	activeReservation, activeErr := store.Reservation(t.Context(), "small_account", "reservation_1")
	if _, err := store.Release(t.Context(), "small_account", "reservation_1", releasedAt); err != nil {
		t.Fatalf("Release: %v", err)
	}
	releasedReservation, releasedErr := store.Reservation(t.Context(), "small_account", "reservation_1")

	if activeErr != nil || releasedErr != nil {
		t.Fatalf("Reservation: unexpected errors %v and %v", activeErr, releasedErr)
	}
	assertSameReservation(t, activeReservation, candidate)
	wantReleased := candidate
	wantReleased.Status = domain.StatusReleased
	wantReleased.ReleasedAt = releasedAt
	assertSameReservation(t, releasedReservation, wantReleased)
	assertInvariants(t, store.database)
}

func TestRelease_NotFound(t *testing.T) {
	testCases := []struct {
		name          string
		accountID     string
		reservationID string
		wantErr       error
	}{
		{"unknown_account", "unknown_account", "reservation_1", domain.ErrAccountNotFound},
		{"unknown_reservation", "small_account", "unknown_reservation", domain.ErrReservationNotFound},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
			reserveForTest(t, store, "small_account", "reservation_1", 31457280)

			_, err := store.Release(t.Context(), testCase.accountID, testCase.reservationID, time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC))

			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("Release(%s, %s): got error %v, want %v", testCase.accountID, testCase.reservationID, err, testCase.wantErr)
			}
			if status, _ := readReleaseState(t, store, "reservation_1"); status != "active" {
				t.Errorf("reservation_1: got status %q, want it still active", status)
			}
			assertInvariants(t, store.database)
		})
	}
}

func TestGet_UnknownReservation(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})

	_, err := store.Reservation(t.Context(), "small_account", "unknown_reservation")

	if !errors.Is(err, domain.ErrReservationNotFound) {
		t.Errorf("Reservation(small_account, unknown_reservation): got error %v, want domain.ErrReservationNotFound", err)
	}
}

func TestRelease_DatabaseUnavailable(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	reserveForTest(t, store, "small_account", "reservation_1", 31457280)
	_ = store.Close()

	_, releaseErr := store.Release(t.Context(), "small_account", "reservation_1", time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC))
	_, readErr := store.Reservation(t.Context(), "small_account", "reservation_1")

	if !errors.Is(releaseErr, domain.ErrServiceUnavailable) || errors.Unwrap(releaseErr) != domain.ErrServiceUnavailable {
		t.Errorf("Release on a closed database: got error %v, want domain.ErrServiceUnavailable and nothing else to unwrap", releaseErr)
	}
	if !errors.Is(readErr, domain.ErrServiceUnavailable) || errors.Unwrap(readErr) != domain.ErrServiceUnavailable {
		t.Errorf("Reservation on a closed database: got error %v, want domain.ErrServiceUnavailable and nothing else to unwrap", readErr)
	}
}

func TestRelease_FailureMidTransactionWritesNothing(t *testing.T) {
	testCases := []struct {
		name    string
		trigger string
	}{
		{"reservation_update_fails", "CREATE TRIGGER failing_release BEFORE UPDATE ON reservations BEGIN SELECT RAISE(ABORT, 'injected failure'); END"},
		{"counter_update_fails_after_the_reservation_update", "CREATE TRIGGER failing_counters BEFORE UPDATE ON accounts BEGIN SELECT RAISE(ABORT, 'injected failure'); END"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
			reserveForTest(t, store, "small_account", "reservation_1", 31457280)
			execute(t, store, testCase.trigger)

			_, err := store.Release(t.Context(), "small_account", "reservation_1", time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC))

			if !errors.Is(err, domain.ErrServiceUnavailable) {
				t.Errorf("Release: got error %v, want domain.ErrServiceUnavailable", err)
			}
			if status, releasedAt := readReleaseState(t, store, "reservation_1"); status != "active" || releasedAt.Valid {
				t.Errorf("reservation_1: got status %q released at %v, want it still active", status, releasedAt)
			}
			wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1}}
			if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
				t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
			}
			assertInvariants(t, store.database)
		})
	}
}

func TestRelease_SecondReleaseGivesNothingBack(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	reserveForTest(t, store, "small_account", "reservation_1", 31457280)
	reserveForTest(t, store, "small_account", "reservation_2", 31457280)
	firstReleasedAt := time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC)
	if _, err := store.Release(t.Context(), "small_account", "reservation_1", firstReleasedAt); err != nil {
		t.Fatalf("first release: %v", err)
	}

	_, _ = store.Release(t.Context(), "small_account", "reservation_1", firstReleasedAt.Add(time.Minute))

	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after releasing reservation_1 twice: got %+v, want %+v", got, wantAccounts)
	}
	if status, releasedAt := readReleaseState(t, store, "reservation_1"); status != "released" || releasedAt.Int64 != firstReleasedAt.UnixMilli() {
		t.Errorf("reservation_1: got status %q released at %v, want released at %d", status, releasedAt, firstReleasedAt.UnixMilli())
	}
	assertInvariants(t, store.database)
}

func reserveForTest(t *testing.T, store *Store, accountID, reservationID string, bytes int64) domain.Reservation {
	t.Helper()
	candidate := newCandidate(accountID, reservationID, "key_"+reservationID, bytes)
	if _, _, err := store.Reserve(t.Context(), candidate, domain.MaxActive{Value: 5, Known: true}); err != nil {
		t.Fatalf("reserve %s: %v", reservationID, err)
	}
	return candidate
}

func readReleaseState(t *testing.T, store *Store, reservationID string) (string, sql.NullInt64) {
	t.Helper()
	var status string
	var releasedAt sql.NullInt64
	err := store.database.QueryRowContext(t.Context(), "SELECT status, released_at FROM reservations WHERE id = ?", reservationID).Scan(&status, &releasedAt)
	if err != nil {
		t.Fatalf("read reservation %s: %v", reservationID, err)
	}
	return status, releasedAt
}

func assertSameReservation(t *testing.T, got, want domain.Reservation) {
	t.Helper()
	sameValues := got.ID == want.ID && got.AccountID == want.AccountID && got.IdempotencyKey == want.IdempotencyKey && got.Bytes == want.Bytes && got.Status == want.Status
	if !sameValues || !got.CreatedAt.Equal(want.CreatedAt) || !got.ReleasedAt.Equal(want.ReleasedAt) {
		t.Errorf("reservation:\n got  %+v\n want %+v", got, want)
	}
}
