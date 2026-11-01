package sqlite

import (
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestReserve_DatabaseLockedReturnsUnavailable(t *testing.T) {
	path := newDatabasePath(t)
	store := openStore(t, path, map[string]int64{"small_account": 104857600})
	shortenBusyTimeout(t, store)
	lock := holdWriteLockFromAnotherConnection(t, path)
	candidate := newCandidate("small_account", "reservation_1", "key_1", 31457280)

	_, _, lockedErr := store.Reserve(t.Context(), candidate, domain.MaxActive{Value: 5, Known: true})

	assertOnlyServiceUnavailable(t, "Reserve while another connection holds the write lock", lockedErr)
	if reservations := readStoredReservations(t, store); len(reservations) != 0 {
		t.Errorf("reservations after the locked attempt: got %+v, want none", reservations)
	}

	_ = lock.Rollback()
	_, firstOutcome, retryErr := store.Reserve(t.Context(), candidate, domain.MaxActive{Value: 5, Known: true})
	_, secondOutcome, replayErr := store.Reserve(t.Context(), candidate, domain.MaxActive{Value: 5, Known: true})

	if retryErr != nil || firstOutcome != domain.Created || replayErr != nil || secondOutcome != domain.Replayed {
		t.Errorf("retries once the lock is released: got outcome %v (error %v) then %v (error %v), want Created then Replayed", firstOutcome, retryErr, secondOutcome, replayErr)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600, reservedBytes: 31457280, activeReservations: 1}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestRelease_DatabaseLockedReturnsUnavailable(t *testing.T) {
	path := newDatabasePath(t)
	store := openStore(t, path, map[string]int64{"small_account": 104857600})
	reserveForTest(t, store, "small_account", "reservation_1", 31457280)
	shortenBusyTimeout(t, store)
	lock := holdWriteLockFromAnotherConnection(t, path)
	releasedAt := time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC)

	_, lockedErr := store.Release(t.Context(), "small_account", "reservation_1", releasedAt)

	assertOnlyServiceUnavailable(t, "Release while another connection holds the write lock", lockedErr)
	if status, _ := readReleaseState(t, store, "reservation_1"); status != "active" {
		t.Errorf("status after the locked attempt: got %s, want active", status)
	}

	_ = lock.Rollback()
	released, retryErr := store.Release(t.Context(), "small_account", "reservation_1", releasedAt)

	if retryErr != nil || released.Status != domain.StatusReleased {
		t.Errorf("release once the lock is released: got status %s and error %v, want released", released.Status, retryErr)
	}
	wantAccounts := []storedAccount{{id: "small_account", limitBytes: 104857600}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}

func TestRepository_DiskFullReturnsUnavailable(t *testing.T) {
	store := openStore(t, newDatabasePath(t), map[string]int64{"small_account": 104857600})
	spillingKey := strings.Repeat("k", 10000)
	stored := newCandidate("small_account", "reservation_1", spillingKey, 31457280)
	if _, _, err := store.Reserve(t.Context(), stored, domain.MaxActive{Value: 5, Known: true}); err != nil {
		t.Fatalf("reserve before the disk is full: %v", err)
	}
	capDatabaseAtItsCurrentSize(t, store)

	_, _, reserveErr := store.Reserve(t.Context(), newCandidate("small_account", "reservation_2", spillingKey+"_2", 1048576), domain.MaxActive{Value: 5, Known: true})
	_, releaseErr := store.Release(t.Context(), "small_account", "reservation_1", time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC))

	assertOnlyServiceUnavailable(t, "Reserve on a full disk", reserveErr)
	assertOnlyServiceUnavailable(t, "Release on a full disk", releaseErr)
	assertOnlyActiveReservation(t, store, stored)
}

func TestRepository_ReadOnlyDatabaseReturnsUnavailable(t *testing.T) {
	path := newDatabasePath(t)
	store := openStore(t, path, map[string]int64{"small_account": 104857600})
	stored := reserveForTest(t, store, "small_account", "reservation_1", 31457280)
	readOnlyStore := openReadOnlyStore(t, path)

	_, _, reserveErr := readOnlyStore.Reserve(t.Context(), newCandidate("small_account", "reservation_2", "key_2", 1048576), domain.MaxActive{Value: 5, Known: true})
	_, releaseErr := readOnlyStore.Release(t.Context(), "small_account", "reservation_1", time.Date(2026, 11, 1, 12, 3, 10, 0, time.UTC))

	assertOnlyServiceUnavailable(t, "Reserve on a read-only file", reserveErr)
	assertOnlyServiceUnavailable(t, "Release on a read-only file", releaseErr)
	assertOnlyActiveReservation(t, store, stored)
}

func shortenBusyTimeout(t *testing.T, store *Store) {
	t.Helper()
	store.database.SetMaxOpenConns(1)
	execute(t, store, "PRAGMA busy_timeout = 100")
}

func holdWriteLockFromAnotherConnection(t *testing.T, path string) *sql.Tx {
	t.Helper()
	otherConnection, _ := sql.Open("sqlite", path+connectionSettings)
	t.Cleanup(func() { _ = otherConnection.Close() })
	lock, err := otherConnection.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("take the write lock from another connection: %v", err)
	}
	t.Cleanup(func() { _ = lock.Rollback() })
	return lock
}

func capDatabaseAtItsCurrentSize(t *testing.T, store *Store) {
	t.Helper()
	store.database.SetMaxOpenConns(1)
	var pageCount int
	if err := store.database.QueryRowContext(t.Context(), "PRAGMA page_count").Scan(&pageCount); err != nil {
		t.Fatalf("read the page count: %v", err)
	}
	execute(t, store, "PRAGMA max_page_count = "+strconv.Itoa(pageCount))
}

func openReadOnlyStore(t *testing.T, path string) *Store {
	t.Helper()
	database, _ := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+connectionSettings+"&mode=ro")
	readOnlyStore := &Store{database: database}
	t.Cleanup(func() { _ = readOnlyStore.Close() })
	return readOnlyStore
}

func assertOnlyServiceUnavailable(t *testing.T, operation string, err error) {
	t.Helper()
	if !errors.Is(err, domain.ErrServiceUnavailable) || errors.Unwrap(err) != domain.ErrServiceUnavailable {
		t.Errorf("%s: got error %v, want domain.ErrServiceUnavailable and nothing else to unwrap", operation, err)
	}
}

func assertOnlyActiveReservation(t *testing.T, store *Store, reservation domain.Reservation) {
	t.Helper()
	wantReservations := []storedReservation{{
		id:             reservation.ID,
		accountID:      reservation.AccountID,
		idempotencyKey: reservation.IdempotencyKey,
		bytes:          reservation.Bytes,
		status:         "active",
		createdAt:      reservation.CreatedAt.UnixMilli(),
	}}
	if got := readStoredReservations(t, store); !slices.Equal(got, wantReservations) {
		t.Errorf("reservations: got %+v, want only %s, still active", got, reservation.ID)
	}
	wantAccounts := []storedAccount{{id: reservation.AccountID, limitBytes: 104857600, reservedBytes: reservation.Bytes, activeReservations: 1}}
	if got := readStoredAccounts(t, store); !slices.Equal(got, wantAccounts) {
		t.Errorf("accounts: got %+v, want %+v", got, wantAccounts)
	}
	assertInvariants(t, store.database)
}
