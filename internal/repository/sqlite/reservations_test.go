package sqlite

import (
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
	sqlitedriver "modernc.org/sqlite"
)

func TestReserve_CreatesAndUpdatesCounters(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	candidate := newCandidate("small_account", "reservation_1", "key_1", 31457280)

	_, _, err := store.Reserve(t.Context(), candidate, domain.MaxActive{Value: 5, Known: true})

	if err != nil {
		t.Fatalf("Reserve: unexpected error: %v", err)
	}
	wantReservations := []storedReservation{{id: "reservation_1", accountID: "small_account", idempotencyKey: "key_1", bytes: 31457280, status: "active", createdAt: candidate.CreatedAt.UnixMilli()}}
	if got := readStoredReservations(t, store); !slices.Equal(got, wantReservations) {
		t.Errorf("reservations: got %+v, want %+v", got, wantReservations)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserve_RefusalWritesNothing(t *testing.T) {
	testCases := []struct {
		name           string
		maxActive      domain.MaxActive
		requestedBytes int64
		wantErr        error
	}{
		{"insufficient_storage", domain.MaxActive{Value: 5, Known: true}, 20971520, domain.InsufficientStorageError{RequestedBytes: 20971520, AvailableBytes: 10485760, LimitBytes: 104857600}},
		{"too_many_active_reservations", domain.MaxActive{Value: 1, Known: true}, 1048576, domain.TooManyActiveReservationsError{ActiveReservations: 1, MaxActiveReservations: 1}},
		{"limit_unavailable", domain.MaxActive{}, 1048576, domain.ErrLimitUnavailable},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
			if _, _, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_1", "key_1", 94371840), domain.MaxActive{Value: 5, Known: true}); err != nil {
				t.Fatalf("reserve 90 MiB first: %v", err)
			}
			reservationsBefore := readStoredReservations(t, store)

			_, _, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_2", "key_2", testCase.requestedBytes), testCase.maxActive)

			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("Reserve: got error %v, want %v", err, testCase.wantErr)
			}
			if got := readStoredReservations(t, store); !slices.Equal(got, reservationsBefore) {
				t.Errorf("reservations after a refusal: got %+v, want them unchanged: %+v", got, reservationsBefore)
			}
			wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 94371840, activeReservations: 1}}
			if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
				t.Errorf("accounts after a refusal: got %+v, want %+v", got, wantAccounts)
			}
			assertInvariants(t, store.database)
		})
	}
}

func TestReserve_UnknownAccount(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})

	_, _, err := store.Reserve(t.Context(), newCandidate("unknown_account", "reservation_1", "key_1", 1048576), domain.MaxActive{Value: 5, Known: true})

	if !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("Reserve on an unknown account: got error %v, want domain.ErrAccountNotFound", err)
	}
	if got := readStoredReservations(t, store); len(got) != 0 {
		t.Errorf("reservations: got %+v, want none", got)
	}
	assertInvariants(t, store.database)
}

func TestReserve_DatabaseUnavailable(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	_ = store.Close()

	_, _, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_1", "key_1", 1048576), domain.MaxActive{Value: 5, Known: true})

	if !errors.Is(err, domain.ErrServiceUnavailable) {
		t.Errorf("Reserve on a closed database: got error %v, want domain.ErrServiceUnavailable", err)
	}
	if errors.Unwrap(err) != domain.ErrServiceUnavailable {
		t.Errorf("Reserve on a closed database: error %v wraps more than domain.ErrServiceUnavailable", err)
	}
}

func TestReserve_FailureMidTransactionWritesNothing(t *testing.T) {
	testCases := []struct {
		name    string
		trigger string
	}{
		{"insert_fails", "CREATE TRIGGER failing_insert BEFORE INSERT ON reservations BEGIN SELECT RAISE(ABORT, 'injected failure'); END"},
		{"counter_update_fails_after_the_insert", "CREATE TRIGGER failing_update BEFORE UPDATE ON accounts BEGIN SELECT RAISE(ABORT, 'injected failure'); END"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
			execute(t, store, testCase.trigger)

			_, _, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_1", "key_1", 1048576), domain.MaxActive{Value: 5, Known: true})

			if !errors.Is(err, domain.ErrServiceUnavailable) {
				t.Errorf("Reserve: got error %v, want domain.ErrServiceUnavailable", err)
			}
			if got := readStoredReservations(t, store); len(got) != 0 {
				t.Errorf("reservations: got %+v, want none", got)
			}
			wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600}}
			if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
				t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
			}
			assertInvariants(t, store.database)
		})
	}
}

func TestReserve_RetryReturnsSameReservation(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	original := newCandidate("small_account", "reservation_1", "key_1", 31457280)
	if _, _, err := store.Reserve(t.Context(), original, domain.MaxActive{Value: 5, Known: true}); err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	retry := newCandidate("small_account", "reservation_2", "key_1", 31457280)

	reservation, outcome, err := store.Reserve(t.Context(), retry, domain.MaxActive{Value: 5, Known: true})

	if err != nil {
		t.Fatalf("retry with the same key: unexpected error: %v", err)
	}
	if outcome != domain.Replayed {
		t.Errorf("retry with the same key: got outcome %d, want domain.Replayed", outcome)
	}
	if reservation.ID != original.ID || reservation.AccountID != original.AccountID || reservation.IdempotencyKey != original.IdempotencyKey ||
		reservation.Bytes != original.Bytes || reservation.Status != original.Status || !reservation.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("retry with the same key: got %+v, want the stored reservation %+v", reservation, original)
	}
	wantReservations := []storedReservation{{id: "reservation_1", accountID: "small_account", idempotencyKey: "key_1", bytes: 31457280, status: "active", createdAt: original.CreatedAt.UnixMilli()}}
	if got := readStoredReservations(t, store); !slices.Equal(got, wantReservations) {
		t.Errorf("reservations after the retry: got %+v, want %+v", got, wantReservations)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the retry: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserve_KeyReuseWithDifferentBytes(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	original := newCandidate("small_account", "reservation_1", "key_1", 31457280)
	if _, _, err := store.Reserve(t.Context(), original, domain.MaxActive{Value: 5, Known: true}); err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	otherSize := newCandidate("small_account", "reservation_2", "key_1", 32505856)

	_, _, err := store.Reserve(t.Context(), otherSize, domain.MaxActive{Value: 5, Known: true})

	wantErr := domain.IdempotencyKeyReusedError{OriginalBytes: 31457280}
	if !errors.Is(err, wantErr) {
		t.Errorf("same key with 31 MiB after 30 MiB: got error %v, want %v", err, wantErr)
	}
	wantReservations := []storedReservation{{id: "reservation_1", accountID: "small_account", idempotencyKey: "key_1", bytes: 31457280, status: "active", createdAt: original.CreatedAt.UnixMilli()}}
	if got := readStoredReservations(t, store); !slices.Equal(got, wantReservations) {
		t.Errorf("reservations after the key reuse: got %+v, want %+v", got, wantReservations)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts after the key reuse: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserve_RefusedAttemptIsNotRemembered(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	_, _, refusedErr := store.Reserve(t.Context(), newCandidate("small_account", "reservation_1", "key_1", 209715200), domain.MaxActive{Value: 5, Known: true})
	var refusal domain.InsufficientStorageError
	if !errors.As(refusedErr, &refusal) {
		t.Fatalf("first attempt of 200 MiB on 100 MiB: got error %v, want domain.InsufficientStorageError", refusedErr)
	}
	execute(t, store, "UPDATE accounts SET storage_limit_bytes = 314572800 WHERE id = 'small_account'")

	reservation, outcome, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_2", "key_1", 209715200), domain.MaxActive{Value: 5, Known: true})

	if err != nil {
		t.Fatalf("retry with the same key once the limit is raised: unexpected error: %v", err)
	}
	if outcome != domain.Created || reservation.ID != "reservation_2" {
		t.Errorf("retry with the same key once the limit is raised: got outcome %d and reservation %q, want domain.Created and reservation_2", outcome, reservation.ID)
	}
	wantReservations := []storedReservation{{id: "reservation_2", accountID: "small_account", idempotencyKey: "key_1", bytes: 209715200, status: "active", createdAt: reservation.CreatedAt.UnixMilli()}}
	if got := readStoredReservations(t, store); !slices.Equal(got, wantReservations) {
		t.Errorf("reservations: got %+v, want %+v", got, wantReservations)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 314572800, reservedBytes: 209715200, activeReservations: 1}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserve_DifferentKeysCreateDifferentReservations(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	if _, _, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_1", "key_1", 4194304), domain.MaxActive{Value: 5, Known: true}); err != nil {
		t.Fatalf("first reservation: %v", err)
	}

	reservation, outcome, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_2", "key_2", 4194304), domain.MaxActive{Value: 5, Known: true})

	if err != nil {
		t.Fatalf("same size with another key: unexpected error: %v", err)
	}
	if outcome != domain.Created || reservation.ID != "reservation_2" {
		t.Errorf("same size with another key: got outcome %d and reservation %q, want domain.Created and reservation_2", outcome, reservation.ID)
	}
	createdAt := reservation.CreatedAt.UnixMilli()
	wantReservations := []storedReservation{
		{id: "reservation_1", accountID: "small_account", idempotencyKey: "key_1", bytes: 4194304, status: "active", createdAt: createdAt},
		{id: "reservation_2", accountID: "small_account", idempotencyKey: "key_2", bytes: 4194304, status: "active", createdAt: createdAt},
	}
	if got := readStoredReservations(t, store); !slices.Equal(got, wantReservations) {
		t.Errorf("reservations: got %+v, want %+v", got, wantReservations)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 8388608, activeReservations: 2}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestReserve_KeyLookupFailureWritesNothing(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	execute(t, store, "DROP TABLE reservations")

	_, _, err := store.Reserve(t.Context(), newCandidate("small_account", "reservation_1", "key_1", 1048576), domain.MaxActive{Value: 5, Known: true})

	if !errors.Is(err, domain.ErrServiceUnavailable) {
		t.Errorf("Reserve when the key lookup fails: got error %v, want domain.ErrServiceUnavailable", err)
	}
	if errors.Unwrap(err) != domain.ErrServiceUnavailable {
		t.Errorf("Reserve when the key lookup fails: error %v wraps more than domain.ErrServiceUnavailable", err)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
	}
}

func newCandidate(accountID, reservationID, idempotencyKey string, bytes int64) domain.Reservation {
	return domain.Reservation{
		ID:             reservationID,
		AccountID:      accountID,
		IdempotencyKey: idempotencyKey,
		Bytes:          bytes,
		Status:         domain.StatusActive,
		CreatedAt:      time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
	}
}

type storedReservation struct {
	id             string
	accountID      string
	idempotencyKey string
	bytes          int64
	status         string
	createdAt      int64
}

func readStoredReservations(t *testing.T, store *Store) []storedReservation {
	t.Helper()
	rows, err := store.database.QueryContext(t.Context(), "SELECT id, account_id, idempotency_key, bytes, status, created_at FROM reservations ORDER BY id")
	if err != nil {
		t.Fatalf("list reservations: %v", err)
	}
	defer rows.Close()
	var reservations []storedReservation
	for rows.Next() {
		var reservation storedReservation
		if err := rows.Scan(&reservation.id, &reservation.accountID, &reservation.idempotencyKey, &reservation.bytes, &reservation.status, &reservation.createdAt); err != nil {
			t.Fatalf("scan reservation: %v", err)
		}
		reservations = append(reservations, reservation)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list reservations: %v", err)
	}
	return reservations
}

func assertInvariants(t *testing.T, database *sql.DB) {
	t.Helper()
	const accountsBreakingInvariants = `SELECT account.id
		FROM accounts account
		LEFT JOIN reservations reservation ON reservation.account_id = account.id AND reservation.status = 'active'
		GROUP BY account.id
		HAVING account.reserved_bytes <> COALESCE(SUM(reservation.bytes), 0)
		    OR account.active_reservations <> COUNT(reservation.id)`
	rows, err := database.QueryContext(t.Context(), accountsBreakingInvariants)
	if err != nil {
		t.Fatalf("check the invariants: %v", err)
	}
	defer rows.Close()
	var accountIDs []string
	for rows.Next() {
		var accountID string
		if err := rows.Scan(&accountID); err != nil {
			t.Fatalf("scan an account breaking the invariants: %v", err)
		}
		accountIDs = append(accountIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("check the invariants: %v", err)
	}
	if len(accountIDs) > 0 {
		t.Errorf("invariants I1 and I2 broken for %v: the counters differ from the active reservations", accountIDs)
	}
}

func TestListActiveReservations_OnlyActiveOldestFirst(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600, "large_account": 10737418240})
	oldest := newCandidate("small_account", "reservation_c", "key_b", 1048576)
	oldest.CreatedAt = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	middle := newCandidate("small_account", "reservation_a", "key_c", 2097152)
	middle.CreatedAt = time.Date(2026, 11, 1, 12, 1, 0, 0, time.UTC)
	newest := newCandidate("small_account", "reservation_b", "key_a", 3145728)
	newest.CreatedAt = time.Date(2026, 11, 1, 12, 2, 0, 0, time.UTC)
	released := newCandidate("small_account", "reservation_d", "key_d", 4194304)
	otherAccount := newCandidate("large_account", "reservation_e", "key_e", 5242880)
	for _, candidate := range []domain.Reservation{middle, released, oldest, otherAccount, newest} {
		if _, _, err := store.Reserve(t.Context(), candidate, domain.MaxActive{Value: 5, Known: true}); err != nil {
			t.Fatalf("reserve %s: %v", candidate.ID, err)
		}
	}
	if _, err := store.Release(t.Context(), "small_account", released.ID, time.Date(2026, 11, 1, 12, 3, 0, 0, time.UTC)); err != nil {
		t.Fatalf("release %s: %v", released.ID, err)
	}

	got, err := store.ListActiveReservations(t.Context(), "small_account")

	want := []domain.Reservation{oldest, middle, newest}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("ListActiveReservations of small_account:\n got  %+v and error %v\n want %+v: its active reservations, oldest first", got, err, want)
	}
	assertInvariants(t, store.database)
}

func TestListActiveReservations_DatabaseUnavailable(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	reserveForTest(t, store, "small_account", "reservation_1", 1048576)
	_ = store.Close()

	reservations, err := store.ListActiveReservations(t.Context(), "small_account")

	assertOnlyServiceUnavailable(t, "ListActiveReservations on a closed database", err)
	if reservations != nil {
		t.Errorf("ListActiveReservations on a closed database: got %+v, want no reservations", reservations)
	}
}

func TestListActiveReservations_UnreadableRowFailsTheWholeList(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	reserveForTest(t, store, "small_account", "reservation_1", 1048576)
	reserveForTest(t, store, "small_account", "reservation_2", 2097152)
	execute(t, store, "ALTER TABLE reservations RENAME TO stored_reservations")
	execute(t, store, "CREATE VIEW reservations AS SELECT id, account_id, CASE id WHEN 'reservation_2' THEN NULL ELSE idempotency_key END AS idempotency_key, bytes, status, created_at, released_at FROM stored_reservations")

	reservations, err := store.ListActiveReservations(t.Context(), "small_account")

	assertOnlyServiceUnavailable(t, "ListActiveReservations with an unreadable second row", err)
	if reservations != nil {
		t.Errorf("ListActiveReservations with an unreadable second row: got %+v, want no reservations, not the first one alone", reservations)
	}
}

func TestListActiveReservations_ErrorOnTheSecondRowFailsTheWholeList(t *testing.T) {
	failingFunctionName := registerFunctionFailingOnReservation(t, "reservation_2")
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	reserveForTest(t, store, "small_account", "reservation_1", 1048576)
	reserveForTest(t, store, "small_account", "reservation_2", 2097152)
	execute(t, store, "ALTER TABLE reservations RENAME TO stored_reservations")
	execute(t, store, "CREATE VIEW reservations AS SELECT id, account_id, "+failingFunctionName+"(id, idempotency_key) AS idempotency_key, bytes, status, created_at, released_at FROM stored_reservations")

	reservations, err := store.ListActiveReservations(t.Context(), "small_account")

	assertOnlyServiceUnavailable(t, "ListActiveReservations when the second row raises an error", err)
	if reservations != nil {
		t.Errorf("ListActiveReservations when the second row raises an error: got %+v, want no reservations, not the first one alone", reservations)
	}
}

func registerFunctionFailingOnReservation(t *testing.T, failingReservationID string) string {
	t.Helper()
	functionName := "fail_on_reservation_" + rand.Text()
	err := sqlitedriver.RegisterScalarFunction(functionName, 2, func(functionContext *sqlitedriver.FunctionContext, arguments []driver.Value) (driver.Value, error) {
		if arguments[0] == failingReservationID {
			return nil, errors.New("injected failure")
		}
		return arguments[1], nil
	})
	if err != nil {
		t.Fatalf("register the SQL function %s: %v", functionName, err)
	}
	return functionName
}
