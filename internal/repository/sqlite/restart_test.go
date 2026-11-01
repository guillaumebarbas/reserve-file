package sqlite

import (
	"slices"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestRepository_ReopenKeepsState(t *testing.T) {
	path := newDatabasePath(t)
	accounts := map[string]int64{"small_account": 104857600}
	maxActive := domain.MaxActive{Value: 5, Known: true}
	releasedAt := time.Date(2026, 11, 1, 12, 5, 0, 0, time.UTC)
	firstStore := openStore(t, path, accounts)
	kept := reserveForTest(t, firstStore, "small_account", "reservation_kept", 31457280)
	reserveForTest(t, firstStore, "small_account", "reservation_released", 20971520)
	released, err := firstStore.Release(t.Context(), "small_account", "reservation_released", releasedAt)
	if err != nil {
		t.Fatalf("release before the restart: %v", err)
	}
	_ = firstStore.Close()

	restartedStore := openStore(t, path, accounts)

	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1}}
	if got := readStoredAccounts(t, restartedStore); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the restart: got %+v, want %+v", got, wantAccounts)
	}
	replayedKept, keptOutcome, err := restartedStore.Reserve(t.Context(), newCandidate("small_account", "reservation_retry_1", "key_reservation_kept", 31457280), maxActive)
	if err != nil || keptOutcome != domain.Replayed {
		t.Fatalf("retry of the active reservation's key after the restart: got outcome %d and error %v, want domain.Replayed", keptOutcome, err)
	}
	assertSameReservation(t, replayedKept, kept)
	replayedReleased, releasedOutcome, err := restartedStore.Reserve(t.Context(), newCandidate("small_account", "reservation_retry_2", "key_reservation_released", 20971520), maxActive)
	if err != nil || releasedOutcome != domain.Replayed {
		t.Fatalf("retry of the released reservation's key after the restart: got outcome %d and error %v, want domain.Replayed", releasedOutcome, err)
	}
	assertSameReservation(t, replayedReleased, released)
	if got := readStoredAccounts(t, restartedStore); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the two retries: got %+v, want %+v", got, wantAccounts)
	}
	if _, err := restartedStore.Release(t.Context(), "small_account", "reservation_kept", releasedAt); err != nil {
		t.Fatalf("release after the restart: %v", err)
	}
	wantAccountsAfterRelease := []storedAccount{{id: "small_account", limitBytes: 104857600}}
	if got := readStoredAccounts(t, restartedStore); !slices.Equal(got, wantAccountsAfterRelease) {
		t.Errorf("accounts after the release that follows the restart: got %+v, want %+v", got, wantAccountsAfterRelease)
	}
	assertInvariants(t, restartedStore.database)
}
