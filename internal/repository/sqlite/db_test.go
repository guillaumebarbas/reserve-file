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

	tableNames := readTableNames(t, store)
	schemaVersion := readSchemaVersion(t, store)

	wantTableNames := []string{"accounts", "reservations", "settings"}
	if !slices.Equal(tableNames, wantTableNames) {
		t.Errorf("tables: got %v, want %v", tableNames, wantTableNames)
	}
	if schemaVersion != 1 {
		t.Errorf("user_version: got %d, want 1", schemaVersion)
	}
}

func TestOpen_AppliesPragmas(t *testing.T) {
	const synchronousFull = "2"
	store := openStore(t, newDatabasePath(t), nil)
	want := map[string]string{
		"busy_timeout": "5000",
		"journal_mode": "wal",
		"synchronous":  synchronousFull,
		"foreign_keys": "1",
	}

	connections := []*sql.Conn{holdConnection(t, store), holdConnection(t, store)}

	for index, connection := range connections {
		for name, wantValue := range want {
			if got := readPragma(t, connection, name); got != wantValue {
				t.Errorf("connection %d, PRAGMA %s: got %s, want %s", index+1, name, got, wantValue)
			}
		}
	}
}

func TestSchema_EnforcesConstraints(t *testing.T) {
	const insertReservation = "INSERT INTO reservations (id, account_id, idempotency_key, bytes, status, created_at, released_at) VALUES "
	testCases := []struct {
		name      string
		statement string
		wantError string
	}{
		{"zero_bytes", insertReservation + "('reservation_1', 'test_account', 'key_1', 0, 'active', 0, NULL)", "CHECK constraint failed"},
		{"bytes_as_text", insertReservation + "('reservation_1', 'test_account', 'key_1', 'ten', 'active', 0, NULL)", "cannot store TEXT value"},
		{"unknown_status", insertReservation + "('reservation_1', 'test_account', 'key_1', 10, 'pending', 0, NULL)", "CHECK constraint failed"},
		{"released_without_released_at", insertReservation + "('reservation_1', 'test_account', 'key_1', 10, 'released', 0, NULL)", "CHECK constraint failed"},
		{"active_with_released_at", insertReservation + "('reservation_1', 'test_account', 'key_1', 10, 'active', 0, 5)", "CHECK constraint failed"},
		{"duplicate_idempotency_key", insertReservation + "('reservation_1', 'test_account', 'key_1', 10, 'active', 0, NULL), ('reservation_2', 'test_account', 'key_1', 10, 'active', 0, NULL)", "UNIQUE constraint failed"},
		{"unknown_account", insertReservation + "('reservation_1', 'unknown_account', 'key_1', 10, 'active', 0, NULL)", "FOREIGN KEY constraint failed"},
		{"negative_storage_limit", "INSERT INTO accounts (id, storage_limit_bytes) VALUES ('another_account', -1)", "CHECK constraint failed"},
		{"negative_reserved_bytes", "UPDATE accounts SET reserved_bytes = -1 WHERE id = 'test_account'", "CHECK constraint failed"},
		{"negative_active_reservations", "UPDATE accounts SET active_reservations = -1 WHERE id = 'test_account'", "CHECK constraint failed"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := openStore(t, newDatabasePath(t), nil)
			execute(t, store, "INSERT INTO accounts (id, storage_limit_bytes) VALUES ('test_account', 100)")

			_, err := store.database.ExecContext(t.Context(), testCase.statement)

			if err == nil {
				t.Fatalf("%s: got nil error, want %q", testCase.statement, testCase.wantError)
			}
			if !strings.Contains(err.Error(), testCase.wantError) {
				t.Errorf("%s: got error %q, want %q", testCase.statement, err, testCase.wantError)
			}
		})
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	path := newDatabasePath(t)
	firstStore := openStore(t, path, nil)
	execute(t, firstStore, "INSERT INTO accounts (id, storage_limit_bytes) VALUES ('test_account', 100)")
	_ = firstStore.Close()

	secondStore := openStore(t, path, nil)

	if schemaVersion := readSchemaVersion(t, secondStore); schemaVersion != 1 {
		t.Errorf("user_version after reopening: got %d, want 1", schemaVersion)
	}
	var accountCount int
	if err := secondStore.database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM accounts WHERE id = 'test_account'").Scan(&accountCount); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if accountCount != 1 {
		t.Errorf("accounts named test_account after reopening: got %d, want 1", accountCount)
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

	store, err := Open(t.Context(), path, map[string]int64{"large_account": 100})

	if err == nil {
		_ = store.Close()
		t.Fatalf("Open on a version 1 database without the storage_limit_bytes column: got nil error, want an error")
	}
	if !strings.Contains(err.Error(), "seed account large_account") {
		t.Errorf("Open on a version 1 database without the storage_limit_bytes column: error %q does not name the failed step", err)
	}
}

func TestSeed_CreatesAccounts(t *testing.T) {
	accounts := map[string]int64{"large_account": 10737418240, "small_account": 104857600}

	store := openStore(t, newDatabasePath(t), accounts)

	got := readStoredAccounts(t, store)
	want := []storedAccount{
		{id: "large_account", limitBytes: 10737418240},
		{id: "small_account", limitBytes: 104857600},
	}
	if !slices.Equal(got, want) {
		t.Errorf("accounts: got %+v, want %+v", got, want)
	}
}

func TestSeed_UpsertsLimitOnly(t *testing.T) {
	path := newDatabasePath(t)
	firstStore := openStore(t, path, map[string]int64{"small_account": 104857600})
	execute(t, firstStore, "INSERT INTO reservations (id, account_id, idempotency_key, bytes, status, created_at) VALUES ('reservation_1', 'small_account', 'key_1', 30, 'active', 0)")
	execute(t, firstStore, "UPDATE accounts SET reserved_bytes = 30, active_reservations = 1 WHERE id = 'small_account'")
	_ = firstStore.Close()

	secondStore := openStore(t, path, map[string]int64{"small_account": 52428800})

	got := readStoredAccounts(t, secondStore)
	want := []storedAccount{{id: "small_account", limitBytes: 52428800, reservedBytes: 30, activeReservations: 1}}
	if !slices.Equal(got, want) {
		t.Errorf("accounts after reseeding with a lower limit: got %+v, want %+v", got, want)
	}
}

func newDatabasePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "reserve.db")
}

func openStore(t *testing.T, path string, accounts map[string]int64) *Store {
	t.Helper()
	store, err := Open(t.Context(), path, accounts)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func readTableNames(t *testing.T, store *Store) []string {
	t.Helper()
	rows, err := store.database.QueryContext(t.Context(), "SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var tableNames []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tableNames = append(tableNames, tableName)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	return tableNames
}

type storedAccount struct {
	id                 string
	limitBytes         int64
	reservedBytes      int64
	activeReservations int
}

func readStoredAccounts(t *testing.T, store *Store) []storedAccount {
	t.Helper()
	rows, err := store.database.QueryContext(t.Context(), "SELECT id, storage_limit_bytes, reserved_bytes, active_reservations FROM accounts ORDER BY id")
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

func createDatabase(t *testing.T, path string, statements string) {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer database.Close()
	if _, err := database.ExecContext(t.Context(), statements); err != nil {
		t.Fatalf("create the database: %v", err)
	}
}

func execute(t *testing.T, store *Store, statement string) {
	t.Helper()
	if _, err := store.database.ExecContext(t.Context(), statement); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

func holdConnection(t *testing.T, store *Store) *sql.Conn {
	t.Helper()
	connection, err := store.database.Conn(t.Context())
	if err != nil {
		t.Fatalf("get a connection: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func readPragma(t *testing.T, connection *sql.Conn, name string) string {
	t.Helper()
	var value string
	if err := connection.QueryRowContext(t.Context(), "PRAGMA "+name).Scan(&value); err != nil {
		t.Fatalf("read PRAGMA %s: %v", name, err)
	}
	return value
}

func readSchemaVersion(t *testing.T, store *Store) int {
	t.Helper()
	var schemaVersion int
	if err := store.database.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&schemaVersion); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return schemaVersion
}
