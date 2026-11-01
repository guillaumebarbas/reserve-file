package sqlite

import (
	"errors"
	"testing"

	"github.com/guillaumebarbas/reserve-file/internal/domain"
)

func TestSettingsStore_SaveAndLoad(t *testing.T) {
	store := openStore(t, newDatabasePath(t), nil)

	_, foundBeforeSave, loadErrBeforeSave := store.LoadMaxParallelChunks(t.Context())
	firstSaveErr := store.SaveMaxParallelChunks(t.Context(), 5)
	secondSaveErr := store.SaveMaxParallelChunks(t.Context(), 8)
	got, found, loadErr := store.LoadMaxParallelChunks(t.Context())

	if foundBeforeSave || loadErrBeforeSave != nil {
		t.Errorf("load before any save: got found %t, error %v, want not found and no error", foundBeforeSave, loadErrBeforeSave)
	}
	if firstSaveErr != nil || secondSaveErr != nil {
		t.Fatalf("save 5 then 8: got errors %v and %v, want none", firstSaveErr, secondSaveErr)
	}
	if got != 8 || !found || loadErr != nil {
		t.Errorf("load after saving 5 then 8: got %d, found %t, error %v, want 8, found, no error", got, found, loadErr)
	}
}

func TestSettingsStore_FailsWhenDatabaseClosed(t *testing.T) {
	store := openStore(t, newDatabasePath(t), nil)
	_ = store.Close()

	_, _, loadErr := store.LoadMaxParallelChunks(t.Context())
	saveErr := store.SaveMaxParallelChunks(t.Context(), 5)

	if !errors.Is(loadErr, domain.ErrServiceUnavailable) || errors.Unwrap(loadErr) != domain.ErrServiceUnavailable {
		t.Errorf("load on a closed database: got error %v, want one that wraps only domain.ErrServiceUnavailable", loadErr)
	}
	if !errors.Is(saveErr, domain.ErrServiceUnavailable) || errors.Unwrap(saveErr) != domain.ErrServiceUnavailable {
		t.Errorf("save on a closed database: got error %v, want one that wraps only domain.ErrServiceUnavailable", saveErr)
	}
}

func TestSettingsStore_RejectsCorruptedValue(t *testing.T) {
	store := openStore(t, newDatabasePath(t), nil)
	execute(t, store, "INSERT INTO settings (key, value, updated_at) VALUES ('max_parallel_chunks', 'five', 0)")

	got, found, err := store.LoadMaxParallelChunks(t.Context())

	if err == nil || found || got != 0 {
		t.Errorf("load of the stored value 'five': got %d, found %t, error %v, want 0, not found and an error", got, found, err)
	}
}
