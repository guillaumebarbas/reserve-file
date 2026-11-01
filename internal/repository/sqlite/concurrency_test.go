package sqlite

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestReserve_ConcurrentRequestsNeverExceedLimit(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	if err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_setup", "key_setup", 31457280), domain.MaxActive{Value: 100, Known: true}); err != nil {
		t.Fatalf("reserve 30 MiB first: %v", err)
	}
	candidates := newCandidates("small_account", 50, 20971520)

	results := reserveConcurrently(t.Context(), store, candidates, domain.MaxActive{Value: 100, Known: true})

	outcomes := countReservationOutcomes(results)
	if outcomes.created != 3 || outcomes.insufficientStorage != 47 || len(outcomes.unexpected) > 0 {
		t.Errorf("50 concurrent requests of 20 MiB with 70 MiB free: got %+v, want 3 created and 47 refused for insufficient storage", outcomes)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280 + 3*20971520, activeReservations: 4}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserve_ConcurrentRequestsNeverExceedSlots(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	candidates := newCandidates("small_account", 50, 1048576)

	results := reserveConcurrently(t.Context(), store, candidates, domain.MaxActive{Value: 5, Known: true})

	outcomes := countReservationOutcomes(results)
	if outcomes.created != 5 || outcomes.tooManyActive != 45 || len(outcomes.unexpected) > 0 {
		t.Errorf("50 concurrent requests of 1 MiB with 5 slots: got %+v, want 5 created and 45 refused for too many active reservations", outcomes)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 5 * 1048576, activeReservations: 5}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func newCandidates(accountID string, count int, bytes int64) []domain.Reservation {
	candidates := make([]domain.Reservation, count)
	for index := range candidates {
		candidates[index] = newCandidate(accountID, fmt.Sprintf("reservation_%d", index), fmt.Sprintf("key_%d", index), bytes)
	}
	return candidates
}

func reserveConcurrently(ctx context.Context, store *Store, candidates []domain.Reservation, maxActive domain.MaxActive) []error {
	start := make(chan struct{})
	var requests sync.WaitGroup
	results := make([]error, len(candidates))
	for index, candidate := range candidates {
		requests.Go(func() {
			<-start
			results[index] = store.Reserve(ctx, candidate, maxActive)
		})
	}
	close(start)
	requests.Wait()
	return results
}

type reservationOutcomes struct {
	created             int
	insufficientStorage int
	tooManyActive       int
	unexpected          map[string]int
}

func countReservationOutcomes(results []error) reservationOutcomes {
	outcomes := reservationOutcomes{unexpected: map[string]int{}}
	for _, err := range results {
		var insufficientStorage domain.InsufficientStorageError
		var tooManyActive domain.TooManyActiveReservationsError
		switch {
		case err == nil:
			outcomes.created++
		case errors.As(err, &insufficientStorage):
			outcomes.insufficientStorage++
		case errors.As(err, &tooManyActive):
			outcomes.tooManyActive++
		default:
			outcomes.unexpected[err.Error()]++
		}
	}
	return outcomes
}
