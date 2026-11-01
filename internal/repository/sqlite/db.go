package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

const connectionSettings = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)"

type Store struct {
	database *sql.DB
}

func Open(ctx context.Context, path string, accounts map[string]int64) (*Store, error) {
	database, _ := sql.Open("sqlite", path+connectionSettings)
	if err := createSchemaIfNew(ctx, database); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	if err := seedAccounts(ctx, database, accounts); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	return &Store{database: database}, nil
}

func (store *Store) Ping(ctx context.Context) error {
	if err := store.database.PingContext(ctx); err != nil {
		return fmt.Errorf("%w: ping database: %v", domain.ErrServiceUnavailable, err)
	}
	return nil
}

func (store *Store) Close() error {
	return store.database.Close()
}

func createSchemaIfNew(ctx context.Context, database *sql.DB) error {
	var schemaVersion int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&schemaVersion); err != nil {
		return fmt.Errorf("read schema version: %v", err)
	}
	if schemaVersion != 0 {
		return nil
	}
	if _, err := database.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create schema: %v", err)
	}
	return nil
}

func seedAccounts(ctx context.Context, database *sql.DB, accounts map[string]int64) error {
	const upsertAccount = `INSERT INTO accounts (id, storage_limit_bytes) VALUES (?, ?)
		ON CONFLICT (id) DO UPDATE SET storage_limit_bytes = excluded.storage_limit_bytes`
	for accountID, limitBytes := range accounts {
		if _, err := database.ExecContext(ctx, upsertAccount, accountID, limitBytes); err != nil {
			return fmt.Errorf("seed account %s: %v", accountID, err)
		}
	}
	return nil
}
