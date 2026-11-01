// Package sqlite stores the service's data in a SQLite database.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver with database/sql
)

//go:embed schema.sql
var schema string

// connectionSettings is applied by the driver to every connection of the pool.
// Why each value: docs/consistency.md, SQLite configuration.
const connectionSettings = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)"

// Store keeps the service's data in SQLite. It is safe for concurrent use;
// Close releases the database once the service has stopped.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite database at path, creates its schema if the database
// is new, and makes sure each of the given accounts exists with its storage
// limit in bytes. The error names the step that failed.
func Open(ctx context.Context, path string, accounts map[string]int64) (*Store, error) {
	// sql.Open fails only for an unknown driver name, and the import above
	// registers it. The first statement is what reaches the file.
	db, _ := sql.Open("sqlite", path+connectionSettings)
	if err := ensureSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	if err := seedAccounts(ctx, db, accounts); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %v", err)
	}
	if version != 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create schema: %v", err)
	}
	return nil
}

// seedAccounts creates the missing accounts and applies the configured limit to
// the others. It never changes the usage counters.
func seedAccounts(ctx context.Context, db *sql.DB, accounts map[string]int64) error {
	const upsert = `INSERT INTO accounts (id, storage_limit_bytes) VALUES (?, ?)
		ON CONFLICT (id) DO UPDATE SET storage_limit_bytes = excluded.storage_limit_bytes`
	for id, limitBytes := range accounts {
		if _, err := db.ExecContext(ctx, upsert, id, limitBytes); err != nil {
			return fmt.Errorf("seed account %s: %v", id, err)
		}
	}
	return nil
}
