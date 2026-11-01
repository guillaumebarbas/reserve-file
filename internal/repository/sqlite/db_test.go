package sqlite

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestOpen_CreatesSchema(t *testing.T) {
	store := openStore(t, newDatabasePath(t), nil)

	tables := tableNames(t, store)
	version := schemaVersion(t, store)

	wantTables := []string{"accounts", "reservations", "settings"}
	if !slices.Equal(tables, wantTables) {
		t.Errorf("tables: got %v, want %v", tables, wantTables)
	}
	if version != 1 {
		t.Errorf("user_version: got %d, want 1", version)
	}
}

func TestOpen_AppliesPragmas(t *testing.T) {
	store := openStore(t, newDatabasePath(t), nil)
	want := map[string]string{
		"busy_timeout": "5000",
		"journal_mode": "wal",
		"synchronous":  "2", // FULL
		"foreign_keys": "1",
	}

	// Two connections held at the same time are two different connections of the pool.
	connections := []*sql.Conn{holdConnection(t, store), holdConnection(t, store)}

	for i, conn := range connections {
		for name, wantValue := range want {
			if got := pragma(t, conn, name); got != wantValue {
				t.Errorf("connection %d, PRAGMA %s: got %s, want %s", i+1, name, got, wantValue)
			}
		}
	}
}

func TestSchema_EnforcesConstraints(t *testing.T) {
	const insertReservation = "INSERT INTO reservations (id, account_id, idempotency_key, bytes, status, created_at, released_at) VALUES "
	tests := []struct {
		name      string
		statement string
		wantError string
	}{
		{"zero_bytes", insertReservation + "('r1', 'acc', 'k1', 0, 'active', 0, NULL)", "CHECK constraint failed"},
		{"bytes_as_text", insertReservation + "('r1', 'acc', 'k1', 'ten', 'active', 0, NULL)", "cannot store TEXT value"},
		{"unknown_status", insertReservation + "('r1', 'acc', 'k1', 10, 'pending', 0, NULL)", "CHECK constraint failed"},
		{"released_without_released_at", insertReservation + "('r1', 'acc', 'k1', 10, 'released', 0, NULL)", "CHECK constraint failed"},
		{"active_with_released_at", insertReservation + "('r1', 'acc', 'k1', 10, 'active', 0, 5)", "CHECK constraint failed"},
		{"duplicate_idempotency_key", insertReservation + "('r1', 'acc', 'k1', 10, 'active', 0, NULL), ('r2', 'acc', 'k1', 10, 'active', 0, NULL)", "UNIQUE constraint failed"},
		{"unknown_account", insertReservation + "('r1', 'nobody', 'k1', 10, 'active', 0, NULL)", "FOREIGN KEY constraint failed"},
		{"negative_storage_limit", "INSERT INTO accounts (id, storage_limit_bytes) VALUES ('other', -1)", "CHECK constraint failed"},
		{"negative_reserved_bytes", "UPDATE accounts SET reserved_bytes = -1 WHERE id = 'acc'", "CHECK constraint failed"},
		{"negative_active_reservations", "UPDATE accounts SET active_reservations = -1 WHERE id = 'acc'", "CHECK constraint failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := openStore(t, newDatabasePath(t), nil)
			exec(t, store, "INSERT INTO accounts (id, storage_limit_bytes) VALUES ('acc', 100)")

			_, err := store.db.ExecContext(t.Context(), tc.statement)

			if err == nil {
				t.Fatalf("%s: got nil error, want %q", tc.statement, tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Errorf("%s: got error %q, want %q", tc.statement, err, tc.wantError)
			}
		})
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	path := newDatabasePath(t)
	first := openStore(t, path, nil)
	exec(t, first, "INSERT INTO accounts (id, storage_limit_bytes) VALUES ('acc', 100)")
	_ = first.Close()

	second := openStore(t, path, nil)

	if version := schemaVersion(t, second); version != 1 {
		t.Errorf("user_version after reopening: got %d, want 1", version)
	}
	var accounts int
	if err := second.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM accounts WHERE id = 'acc'").Scan(&accounts); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if accounts != 1 {
		t.Errorf("accounts named acc after reopening: got %d, want 1", accounts)
	}
}

func TestOpen_FailsOnUnusablePath(t *testing.T) {
	directory := t.TempDir()

	store, err := Open(t.Context(), directory, nil)

	if err == nil {
		_ = store.Close()
		t.Fatalf("Open(%s), a directory: got nil error, want an error", directory)
	}
	if !strings.Contains(err.Error(), "read schema version") {
		t.Errorf("Open(%s), a directory: error %q does not name the failed step", directory, err)
	}
}

func TestOpen_FailsOnForeignDatabase(t *testing.T) {
	path := newDatabasePath(t)
	createDatabase(t, path, "CREATE TABLE accounts (id TEXT PRIMARY KEY)")

	store, err := Open(t.Context(), path, nil)

	if err == nil {
		_ = store.Close()
		t.Fatalf("Open on a database the service did not create: got nil error, want an error")
	}
	if !strings.Contains(err.Error(), "create schema") {
		t.Errorf("Open on a database the service did not create: error %q does not name the failed step", err)
	}
}

func TestOpen_FailsOnIncompatibleSchema(t *testing.T) {
	path := newDatabasePath(t)
	createDatabase(t, path, "CREATE TABLE accounts (id TEXT PRIMARY KEY); PRAGMA user_version = 1;")

	store, err := Open(t.Context(), path, map[string]int64{"acc_demo": 100})

	if err == nil {
		_ = store.Close()
		t.Fatalf("Open on a version 1 database without the storage_limit_bytes column: got nil error, want an error")
	}
	if !strings.Contains(err.Error(), "seed account acc_demo") {
		t.Errorf("Open on a version 1 database without the storage_limit_bytes column: error %q does not name the failed step", err)
	}
}

func TestSeed_CreatesAccounts(t *testing.T) {
	accounts := map[string]int64{"acc_demo": 10737418240, "acc_small": 104857600}

	store := openStore(t, newDatabasePath(t), accounts)

	got := storedAccounts(t, store)
	want := []storedAccount{
		{id: "acc_demo", limitBytes: 10737418240},
		{id: "acc_small", limitBytes: 104857600},
	}
	if !slices.Equal(got, want) {
		t.Errorf("accounts: got %+v, want %+v", got, want)
	}
}

func TestSeed_UpsertsLimitOnly(t *testing.T) {
	path := newDatabasePath(t)
	first := openStore(t, path, map[string]int64{"acc_small": 104857600})
	exec(t, first, "INSERT INTO reservations (id, account_id, idempotency_key, bytes, status, created_at) VALUES ('r1', 'acc_small', 'k1', 30, 'active', 0)")
	exec(t, first, "UPDATE accounts SET reserved_bytes = 30, active_reservations = 1 WHERE id = 'acc_small'")
	_ = first.Close()

	second := openStore(t, path, map[string]int64{"acc_small": 52428800})

	got := storedAccounts(t, second)
	want := []storedAccount{{id: "acc_small", limitBytes: 52428800, reservedBytes: 30, activeReservations: 1}}
	if !slices.Equal(got, want) {
		t.Errorf("accounts after reseeding with a lower limit: got %+v, want %+v", got, want)
	}
}

func newDatabasePath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "reserve.db")
}

// openStore opens a store that is closed at the end of the test.
func openStore(t *testing.T, path string, accounts map[string]int64) *Store {
	t.Helper()
	store, err := Open(t.Context(), path, accounts)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func tableNames(t *testing.T, store *Store) []string {
	t.Helper()
	rows, err := store.db.QueryContext(t.Context(), "SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	return names
}

type storedAccount struct {
	id                 string
	limitBytes         int64
	reservedBytes      int64
	activeReservations int
}

func storedAccounts(t *testing.T, store *Store) []storedAccount {
	t.Helper()
	rows, err := store.db.QueryContext(t.Context(), "SELECT id, storage_limit_bytes, reserved_bytes, active_reservations FROM accounts ORDER BY id")
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	defer rows.Close()
	var accounts []storedAccount
	for rows.Next() {
		var account storedAccount
		if err := rows.Scan(&account.id, &account.limitBytes, &account.reservedBytes, &account.activeReservations); err != nil {
			t.Fatalf("scan account: %v", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	return accounts
}

// createDatabase creates a database at path with the given statements, without
// going through Open, as another program would.
func createDatabase(t *testing.T, path string, statements string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer db.Close()
	if _, err := db.ExecContext(t.Context(), statements); err != nil {
		t.Fatalf("create the database: %v", err)
	}
}

func exec(t *testing.T, store *Store, statement string) {
	t.Helper()
	if _, err := store.db.ExecContext(t.Context(), statement); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

func holdConnection(t *testing.T, store *Store) *sql.Conn {
	t.Helper()
	conn, err := store.db.Conn(t.Context())
	if err != nil {
		t.Fatalf("get a connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func pragma(t *testing.T, conn *sql.Conn, name string) string {
	t.Helper()
	var value string
	if err := conn.QueryRowContext(t.Context(), "PRAGMA "+name).Scan(&value); err != nil {
		t.Fatalf("read PRAGMA %s: %v", name, err)
	}
	return value
}

func schemaVersion(t *testing.T, store *Store) int {
	t.Helper()
	var version int
	if err := store.db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return version
}
