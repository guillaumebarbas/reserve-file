package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

const maxParallelChunksSetting = "max_parallel_chunks"

func (store *Store) LoadMaxParallelChunks(ctx context.Context) (int, bool, error) {
	var storedValue string
	err := store.database.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", maxParallelChunksSetting).Scan(&storedValue)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("%w: load %s: %v", domain.ErrServiceUnavailable, maxParallelChunksSetting, err)
	}
	maxParallelChunks, err := strconv.Atoi(storedValue)
	if err != nil {
		return 0, false, fmt.Errorf("load %s: the stored value %q is not a number", maxParallelChunksSetting, storedValue)
	}
	return maxParallelChunks, true, nil
}

func (store *Store) SaveMaxParallelChunks(ctx context.Context, maxParallelChunks int) error {
	const upsertSetting = `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`
	_, err := store.database.ExecContext(ctx, upsertSetting, maxParallelChunksSetting, strconv.Itoa(maxParallelChunks), time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("%w: save %s: %v", domain.ErrServiceUnavailable, maxParallelChunksSetting, err)
	}
	return nil
}
