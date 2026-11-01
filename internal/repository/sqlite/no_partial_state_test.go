package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql/driver"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
	sqlitedriver "modernc.org/sqlite"
)

func TestReserve_RetryAfterAFailedStatementCreatesOnce(t *testing.T) {
	testCases := []struct {
		name            string
		breakStatement  string
		repairStatement string
	}{
		{"account_read_fails", "ALTER TABLE accounts RENAME TO hidden_accounts", "ALTER TABLE hidden_accounts RENAME TO accounts"},
		{"key_lookup_fails", "ALTER TABLE reservations RENAME TO hidden_reservations", "ALTER TABLE hidden_reservations RENAME TO reservations"},
		{"insert_fails", "CREATE TRIGGER failing_insert BEFORE INSERT ON reservations BEGIN SELECT RAISE(ABORT, 'injected failure'); END", "DROP TRIGGER failing_insert"},
		{"counter_update_fails_after_the_insert", "CREATE TRIGGER failing_counters BEFORE UPDATE ON accounts BEGIN SELECT RAISE(ABORT, 'injected failure'); END", "DROP TRIGGER failing_counters"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
			candidate := newCandidate("small_account", "reservation_1", "key_1", 31457280)
			maxActive := domain.MaxActive{Value: 5, Known: true}
			execute(t, store, testCase.breakStatement)

			_, _, failedErr := store.Reserve(t.Context(), candidate, maxActive)
			execute(t, store, testCase.repairStatement)

			if !errors.Is(failedErr, domain.ErrServiceUnavailable) {
				t.Errorf("Reserve while %s: got error %v, want domain.ErrServiceUnavailable", testCase.name, failedErr)
			}
			assertReservationRowsAndAccount(t, store, 0, storedAccount{id: "small_account", limitBytes: 104857600})
			assertRetryCreatesOnce(t, store, candidate, maxActive)
		})
	}
}

func TestRelease_RetryAfterAFailedStatementReleasesOnce(t *testing.T) {
	testCases := []struct {
		name            string
		breakStatement  string
		repairStatement string
	}{
		{"account_read_fails", "ALTER TABLE accounts RENAME TO hidden_accounts", "ALTER TABLE hidden_accounts RENAME TO accounts"},
		{"reservation_read_fails", "ALTER TABLE reservations RENAME TO hidden_reservations", "ALTER TABLE hidden_reservations RENAME TO reservations"},
		{"reservation_update_fails", "CREATE TRIGGER failing_release BEFORE UPDATE ON reservations BEGIN SELECT RAISE(ABORT, 'injected failure'); END", "DROP TRIGGER failing_release"},
		{"counter_update_fails_after_the_reservation_update", "CREATE TRIGGER failing_counters BEFORE UPDATE ON accounts BEGIN SELECT RAISE(ABORT, 'injected failure'); END", "DROP TRIGGER failing_counters"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
			reserveForTest(t, store, "small_account", "reservation_1", 31457280)
			execute(t, store, testCase.breakStatement)

			_, failedErr := store.Release(t.Context(), "small_account", "reservation_1", time.Date(2026, 11, 1, 12, 3, 0, 0, time.UTC))
			execute(t, store, testCase.repairStatement)

			if !errors.Is(failedErr, domain.ErrServiceUnavailable) {
				t.Errorf("Release while %s: got error %v, want domain.ErrServiceUnavailable", testCase.name, failedErr)
			}
			assertStillActive(t, store, "reservation_1", storedAccount{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1})
			assertRetryReleasesOnce(t, store, "reservation_1")
		})
	}
}

func TestReserve_CancelledContextRollsBack(t *testing.T) {
	testCases := []struct {
		name            string
		triggerTemplate string
	}{
		{"cancelled_after_the_insert", "CREATE TRIGGER disconnected_client AFTER INSERT ON reservations BEGIN SELECT %s(); END"},
		{"cancelled_after_the_counter_update_before_commit", "CREATE TRIGGER disconnected_client AFTER UPDATE ON accounts BEGIN SELECT %s(); END"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cancelFunctionName := registerCancellingFunction(t, cancel)
			store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
			candidate := newCandidate("small_account", "reservation_1", "key_1", 31457280)
			maxActive := domain.MaxActive{Value: 5, Known: true}
			execute(t, store, fmt.Sprintf(testCase.triggerTemplate, cancelFunctionName))

			_, _, cancelledErr := store.Reserve(ctx, candidate, maxActive)
			execute(t, store, "DROP TRIGGER disconnected_client")

			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("the trigger did not cancel the context: ctx.Err() is %v", ctx.Err())
			}
			if !errors.Is(cancelledErr, domain.ErrServiceUnavailable) {
				t.Errorf("Reserve with a context cancelled mid-transaction: got error %v, want domain.ErrServiceUnavailable", cancelledErr)
			}
			assertReservationRowsAndAccount(t, store, 0, storedAccount{id: "small_account", limitBytes: 104857600})
			assertRetryCreatesOnce(t, store, candidate, maxActive)
		})
	}
}

func TestRelease_CancelledContextRollsBack(t *testing.T) {
	testCases := []struct {
		name            string
		triggerTemplate string
	}{
		{"cancelled_after_the_reservation_update", "CREATE TRIGGER disconnected_client AFTER UPDATE ON reservations BEGIN SELECT %s(); END"},
		{"cancelled_after_the_counter_update_before_commit", "CREATE TRIGGER disconnected_client AFTER UPDATE ON accounts BEGIN SELECT %s(); END"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cancelFunctionName := registerCancellingFunction(t, cancel)
			store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
			reserveForTest(t, store, "small_account", "reservation_1", 31457280)
			execute(t, store, fmt.Sprintf(testCase.triggerTemplate, cancelFunctionName))

			_, cancelledErr := store.Release(ctx, "small_account", "reservation_1", time.Date(2026, 11, 1, 12, 3, 0, 0, time.UTC))
			execute(t, store, "DROP TRIGGER disconnected_client")

			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("the trigger did not cancel the context: ctx.Err() is %v", ctx.Err())
			}
			if !errors.Is(cancelledErr, domain.ErrServiceUnavailable) {
				t.Errorf("Release with a context cancelled mid-transaction: got error %v, want domain.ErrServiceUnavailable", cancelledErr)
			}
			assertStillActive(t, store, "reservation_1", storedAccount{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1})
			assertRetryReleasesOnce(t, store, "reservation_1")
		})
	}
}

func registerCancellingFunction(t *testing.T, cancel context.CancelFunc) string {
	t.Helper()
	functionName := "cancel_request_" + rand.Text()
	err := sqlitedriver.RegisterScalarFunction(functionName, 0, func(functionContext *sqlitedriver.FunctionContext, arguments []driver.Value) (driver.Value, error) {
		cancel()
		return nil, nil
	})
	if err != nil {
		t.Fatalf("register the SQL function %s: %v", functionName, err)
	}
	return functionName
}

func assertReservationRowsAndAccount(t *testing.T, store *Store, wantRows int, wantAccount storedAccount) {
	t.Helper()
	if got := readStoredReservations(t, store); len(got) != wantRows {
		t.Errorf("reservation rows: got %d %+v, want %d", len(got), got, wantRows)
	}
	if got := readStoredAccounts(t, store); !slices.Equal(got, []storedAccount{wantAccount}) {
		t.Errorf("accounts: got %+v, want %+v", got, []storedAccount{wantAccount})
	}
	assertInvariants(t, store.database)
}

func assertStillActive(t *testing.T, store *Store, reservationID string, wantAccount storedAccount) {
	t.Helper()
	if status, releasedAt := readReleaseState(t, store, reservationID); status != "active" || releasedAt.Valid {
		t.Errorf("%s: got status %q released at %v, want it still active", reservationID, status, releasedAt)
	}
	if got := readStoredAccounts(t, store); !slices.Equal(got, []storedAccount{wantAccount}) {
		t.Errorf("accounts: got %+v, want %+v", got, []storedAccount{wantAccount})
	}
	assertInvariants(t, store.database)
}

func assertRetryCreatesOnce(t *testing.T, store *Store, candidate domain.Reservation, maxActive domain.MaxActive) {
	t.Helper()
	_, firstOutcome, firstErr := store.Reserve(t.Context(), candidate, maxActive)
	_, secondOutcome, secondErr := store.Reserve(t.Context(), candidate, maxActive)
	if firstErr != nil || firstOutcome != domain.Created {
		t.Errorf("retry with the same key: got outcome %v and error %v, want Created", firstOutcome, firstErr)
	}
	if secondErr != nil || secondOutcome != domain.Replayed {
		t.Errorf("second retry with the same key: got outcome %v and error %v, want Replayed", secondOutcome, secondErr)
	}
	assertReservationRowsAndAccount(t, store, 1, storedAccount{id: candidate.AccountID, limitBytes: 104857600, reservedBytes: candidate.Bytes, activeReservations: 1})
}

func assertRetryReleasesOnce(t *testing.T, store *Store, reservationID string) {
	t.Helper()
	retryReleasedAt := time.Date(2026, 11, 1, 12, 4, 0, 0, time.UTC)
	firstRetry, firstErr := store.Release(t.Context(), "small_account", reservationID, retryReleasedAt)
	secondRetry, secondErr := store.Release(t.Context(), "small_account", reservationID, retryReleasedAt.Add(time.Minute))
	if firstErr != nil || firstRetry.Status != domain.StatusReleased || !firstRetry.ReleasedAt.Equal(retryReleasedAt) {
		t.Errorf("retried release: got %+v and error %v, want released at %v", firstRetry, firstErr, retryReleasedAt)
	}
	if secondErr != nil || !secondRetry.ReleasedAt.Equal(retryReleasedAt) {
		t.Errorf("second retried release: got %+v and error %v, want the reservation unchanged, released at %v", secondRetry, secondErr, retryReleasedAt)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the retried release: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}
