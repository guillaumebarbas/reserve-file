package sqlite

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestRelease_ConcurrentReleasesCreditOnce(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	reserveForTest(t, store, "small_account", "reservation_1", 31457280)
	reserveForTest(t, store, "small_account", "reservation_2", 31457280)

	releasedReservations, releaseErrors := releaseConcurrently(t.Context(), store, "reservation_1", 20, time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC))

	stored, err := store.Reservation(t.Context(), "small_account", "reservation_1")
	if err != nil {
		t.Fatalf("read reservation_1: %v", err)
	}
	for index, releaseErr := range releaseErrors {
		if releaseErr != nil {
			t.Errorf("release %d of 20: got error %v, want the released reservation", index+1, releaseErr)
			continue
		}
		assertSameReservation(t, releasedReservations[index], stored)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after 20 concurrent releases of reservation_1: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserveRelease_ConcurrentMix(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	maxActive := domain.MaxActive{Value: 100, Known: true}
	for index := range 10 {
		candidate := newCandidate("small_account", fmt.Sprintf("reservation_old_%d", index), fmt.Sprintf("key_old_%d", index), 1048576)
		if _, _, err := store.Reserve(t.Context(), candidate, maxActive); err != nil {
			t.Fatalf("reserve %s: %v", candidate.ID, err)
		}
	}
	releasedAt := time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC)

	start := make(chan struct{})
	var requests sync.WaitGroup
	releaseErrors := make([]error, 20)
	reserveErrors := make([]error, 10)
	for index := range 10 {
		oldReservationID := fmt.Sprintf("reservation_old_%d", index)
		candidate := newCandidate("small_account", fmt.Sprintf("reservation_new_%d", index), fmt.Sprintf("key_new_%d", index), 1048576)
		requests.Go(func() {
			<-start
			_, releaseErrors[index] = store.Release(t.Context(), "small_account", oldReservationID, releasedAt)
		})
		requests.Go(func() {
			<-start
			_, releaseErrors[10+index] = store.Release(t.Context(), "small_account", oldReservationID, releasedAt.Add(time.Second))
		})
		requests.Go(func() {
			<-start
			_, _, reserveErrors[index] = store.Reserve(t.Context(), candidate, maxActive)
		})
	}
	close(start)
	requests.Wait()

	for index, releaseErr := range releaseErrors {
		if releaseErr != nil {
			t.Errorf("release %d of 20: got error %v, want nil", index+1, releaseErr)
		}
	}
	for index, reserveErr := range reserveErrors {
		if reserveErr != nil {
			t.Errorf("reservation %d of 10: got error %v, want nil", index+1, reserveErr)
		}
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 10 * 1048576, activeReservations: 10}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after 10 releases, 10 repeated releases and 10 reservations at once: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func releaseConcurrently(ctx context.Context, store *Store, reservationID string, requestCount int, firstReleasedAt time.Time) ([]domain.Reservation, []error) {
	start := make(chan struct{})
	var requests sync.WaitGroup
	releasedReservations := make([]domain.Reservation, requestCount)
	releaseErrors := make([]error, requestCount)
	for index := range requestCount {
		requests.Go(func() {
			<-start
			releasedAt := firstReleasedAt.Add(time.Duration(index) * time.Millisecond)
			releasedReservations[index], releaseErrors[index] = store.Release(ctx, "small_account", reservationID, releasedAt)
		})
	}
	close(start)
	requests.Wait()
	return releasedReservations, releaseErrors
}
