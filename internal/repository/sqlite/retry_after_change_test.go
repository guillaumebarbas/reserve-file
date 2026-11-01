package sqlite

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestReserve_ReplayOfReleasedReservation(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	original := reserveForTest(t, store, "small_account", "reservation_1", 31457280)
	released, err := store.Release(t.Context(), "small_account", "reservation_1", time.Date(2026, 11, 1, 12, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("release reservation_1: %v", err)
	}
	retry := newCandidate("small_account", "reservation_retry", original.IdempotencyKey, original.Bytes)

	reservation, outcome, err := store.Reserve(t.Context(), retry, domain.MaxActive{Value: 5, Known: true})

	if err != nil || outcome != domain.Replayed {
		t.Fatalf("retry after the release: got outcome %d and error %v, want domain.Replayed", outcome, err)
	}
	assertSameReservation(t, reservation, released)
	wantReservations := []storedReservation{{id: "reservation_1", accountID: "small_account", idempotencyKey: original.IdempotencyKey, bytes: 31457280, status: "released", createdAt: original.CreatedAt.UnixMilli()}}
	if got := readStoredReservations(t, store); !slices.Equal(got, wantReservations) {
		t.Errorf("reservations after the retry: got %+v, want %+v", got, wantReservations)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the retry: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserve_ReplayWhenAccountFull(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	original := reserveForTest(t, store, "small_account", "reservation_1", 31457280)
	reserveForTest(t, store, "small_account", "reservation_2", 73400320)
	retry := newCandidate("small_account", "reservation_retry", original.IdempotencyKey, original.Bytes)
	newRequest := newCandidate("small_account", "reservation_3", "key_reservation_3", 1)

	reservation, outcome, err := store.Reserve(t.Context(), retry, domain.MaxActive{Value: 5, Known: true})
	_, _, newRequestErr := store.Reserve(t.Context(), newRequest, domain.MaxActive{Value: 5, Known: true})

	if err != nil || outcome != domain.Replayed {
		t.Fatalf("retry on the full account: got outcome %d and error %v, want domain.Replayed", outcome, err)
	}
	assertSameReservation(t, reservation, original)
	var insufficientStorage domain.InsufficientStorageError
	if !errors.As(newRequestErr, &insufficientStorage) {
		t.Errorf("a new key on the full account: got error %v, want domain.InsufficientStorageError", newRequestErr)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 104857600, activeReservations: 2}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the retry: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserve_ReplayWhenEverySlotIsUsed(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	original := reserveForTest(t, store, "small_account", "reservation_1", 1048576)
	reserveForTest(t, store, "small_account", "reservation_2", 1048576)
	twoSlots := domain.MaxActive{Value: 2, Known: true}
	retry := newCandidate("small_account", "reservation_retry", original.IdempotencyKey, original.Bytes)
	newRequest := newCandidate("small_account", "reservation_3", "key_reservation_3", 1048576)

	reservation, outcome, err := store.Reserve(t.Context(), retry, twoSlots)
	_, _, newRequestErr := store.Reserve(t.Context(), newRequest, twoSlots)

	if err != nil || outcome != domain.Replayed {
		t.Fatalf("retry while its own reservation fills the last slot: got outcome %d and error %v, want domain.Replayed", outcome, err)
	}
	assertSameReservation(t, reservation, original)
	var tooManyActive domain.TooManyActiveReservationsError
	if !errors.As(newRequestErr, &tooManyActive) {
		t.Errorf("a new key with every slot used: got error %v, want domain.TooManyActiveReservationsError", newRequestErr)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 2097152, activeReservations: 2}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the retry: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserve_ReplayWhenLimitUnknown(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	original := reserveForTest(t, store, "small_account", "reservation_1", 31457280)
	unknownLimit := domain.MaxActive{}
	retry := newCandidate("small_account", "reservation_retry", original.IdempotencyKey, original.Bytes)
	newRequest := newCandidate("small_account", "reservation_2", "key_reservation_2", 31457280)

	reservation, outcome, err := store.Reserve(t.Context(), retry, unknownLimit)
	_, _, newRequestErr := store.Reserve(t.Context(), newRequest, unknownLimit)

	if err != nil || outcome != domain.Replayed {
		t.Fatalf("retry while the limit is unknown: got outcome %d and error %v, want domain.Replayed", outcome, err)
	}
	assertSameReservation(t, reservation, original)
	if !errors.Is(newRequestErr, domain.ErrLimitUnavailable) {
		t.Errorf("a new key while the limit is unknown: got error %v, want domain.ErrLimitUnavailable", newRequestErr)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the retry: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}
