package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

const connectionSettings = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)"

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string, accounts map[string]int64) (*Store, error) {
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
