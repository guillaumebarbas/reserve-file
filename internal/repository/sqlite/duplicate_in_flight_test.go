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

func TestReserve_ConcurrentSameKeyCreatesOneRow(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	candidates := make([]domain.Reservation, 50)
	for index := range candidates {
		candidates[index] = newCandidate("small_account", fmt.Sprintf("reservation_%d", index), "key_shared", 1048576)
	}

	results := reserveSameKeyConcurrently(t.Context(), store, candidates)

	outcomes := map[domain.Outcome]int{}
	reservationIDs := map[string]bool{}
	for index, result := range results {
		if result.err != nil {
			t.Errorf("request %d: got error %v, want a created or replayed reservation", index, result.err)
			continue
		}
		outcomes[result.outcome]++
		reservationIDs[result.reservation.ID] = true
	}
	if outcomes[domain.Created] != 1 || outcomes[domain.Replayed] != 49 {
		t.Errorf("50 concurrent requests with one key: got %d created and %d replayed, want 1 and 49", outcomes[domain.Created], outcomes[domain.Replayed])
	}
	if len(reservationIDs) != 1 {
		t.Errorf("reservation IDs returned: got %d different ones, want the same one for every request", len(reservationIDs))
	}
	if stored := readStoredReservations(t, store); len(stored) != 1 {
		t.Errorf("stored reservations: got %d, want 1", len(stored))
	}
	assertInvariants(t, store.database)
}

func TestReserve_UniqueViolationMapsToInProgress(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	execute(t, store, `CREATE TRIGGER insert_the_same_key_first BEFORE INSERT ON reservations
		WHEN NEW.id <> 'reservation_injected'
		BEGIN
			INSERT INTO reservations (id, account_id, idempotency_key, bytes, status, created_at)
			VALUES ('reservation_injected', NEW.account_id, NEW.idempotency_key, NEW.bytes, 'active', NEW.created_at);
		END`)

	_, _, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_1", "key_1", 1048576), domain.MaxActive{Value: 5, Known: true})

	if !errors.Is(err, domain.ErrRequestInProgress) {
		t.Errorf("Reserve hitting the UNIQUE constraint on (account_id, idempotency_key): got error %v, want domain.ErrRequestInProgress", err)
	}
	if errors.Unwrap(err) != domain.ErrRequestInProgress {
		t.Errorf("Reserve hitting the UNIQUE constraint: error %v wraps more than domain.ErrRequestInProgress", err)
	}
	if stored := readStoredReservations(t, store); len(stored) != 0 {
		t.Errorf("stored reservations after the violation: got %+v, want none", stored)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the violation: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

type reserveResult struct {
	reservation domain.Reservation
	outcome     domain.Outcome
	err         error
}

func reserveSameKeyConcurrently(ctx context.Context, store *Store, candidates []domain.Reservation) []reserveResult {
	start := make(chan struct{})
	var requests sync.WaitGroup
	results := make([]reserveResult, len(candidates))
	for index, candidate := range candidates {
		requests.Go(func() {
			<-start
			reservation, outcome, err := store.Reserve(ctx, candidate, domain.MaxActive{Value: 5, Known: true})
			results[index] = reserveResult{reservation: reservation, outcome: outcome, err: err}
		})
	}
	close(start)
	requests.Wait()
	return results
}
