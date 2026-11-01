package limits

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestLimits_UsesFetchedValue(t *testing.T) {
	source := newSource(t, http.StatusOK, realisticPayload)
	cache := New(Config{SourceURL: source.URL, FetchTimeout: time.Second, RefreshInterval: time.Minute}, &fakeStore{}, slog.New(slog.DiscardHandler))
	_, knownBeforeInit := cache.MaxActive()

	cache.Init(t.Context())

	maxActive, known := cache.MaxActive()
	if knownBeforeInit {
		t.Error("MaxActive before Init: got known, want unknown")
	}
	if maxActive != 5 || !known {
		t.Errorf("MaxActive after Init: got %d, known %t, want 5, known", maxActive, known)
	}
}

func TestLimits_ReloadsPersistedValue(t *testing.T) {
	failingSource := newSource(t, http.StatusServiceUnavailable, "")
	store := &fakeStore{loadedValue: 7, loadFound: true}
	cache := New(Config{SourceURL: failingSource.URL, FetchTimeout: time.Second, RefreshInterval: time.Minute}, store, slog.New(slog.DiscardHandler))

	cache.Init(t.Context())

	maxActive, known := cache.MaxActive()
	if maxActive != 7 || !known {
		t.Errorf("MaxActive after Init with a saved 7 and the source down: got %d, known %t, want 7, known", maxActive, known)
	}
}

func TestLimits_FetchesWhenSavedValueCannotBeLoaded(t *testing.T) {
	source := newSource(t, http.StatusOK, realisticPayload)
	store := &fakeStore{loadErr: errors.New("service unavailable: load max_parallel_chunks: database is locked")}
	var logOutput bytes.Buffer
	cache := New(Config{SourceURL: source.URL, FetchTimeout: time.Second, RefreshInterval: time.Minute}, store, slog.New(slog.NewJSONHandler(&logOutput, nil)))

	cache.Init(t.Context())

	maxActive, known := cache.MaxActive()
	if maxActive != 5 || !known {
		t.Errorf("MaxActive after Init with an unreadable store: got %d, known %t, want 5, known", maxActive, known)
	}
	if !strings.Contains(logOutput.String(), `"msg":"saved limit not loaded"`) {
		t.Errorf("log %q: want the load failure", logOutput.String())
	}
}

func TestLimits_PersistsNewValue(t *testing.T) {
	var sourceValue atomic.Int64
	sourceValue.Store(5)
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = fmt.Fprintf(writer, `{"MAX_PARALLEL_CHUNKS": %d}`, sourceValue.Load())
	}))
	defer source.Close()
	store := &fakeStore{}
	var logOutput bytes.Buffer
	cache := New(Config{SourceURL: source.URL, FetchTimeout: time.Second, RefreshInterval: time.Minute}, store, slog.New(slog.NewJSONHandler(&logOutput, nil)))

	cache.Init(t.Context())
	sourceValue.Store(8)
	cache.refresh(t.Context())
	cache.refresh(t.Context())

	if !slices.Equal(store.savedValues, []int{5, 8}) {
		t.Errorf("saved values: got %v, want [5 8], each new value once", store.savedValues)
	}
	if updates := strings.Count(logOutput.String(), `"msg":"limit updated"`); updates != 2 {
		t.Errorf("limit updated log lines: got %d, want 2 (log %q)", updates, logOutput.String())
	}
}

func TestLimits_KeepsFetchedValueWhenSaveFails(t *testing.T) {
	source := newSource(t, http.StatusOK, realisticPayload)
	store := &fakeStore{saveErr: errors.New("service unavailable: save max_parallel_chunks: disk is full")}
	var logOutput bytes.Buffer
	cache := New(Config{SourceURL: source.URL, FetchTimeout: time.Second, RefreshInterval: time.Minute}, store, slog.New(slog.NewJSONHandler(&logOutput, nil)))

	cache.Init(t.Context())

	maxActive, known := cache.MaxActive()
	if maxActive != 5 || !known {
		t.Errorf("MaxActive after a failed save: got %d, known %t, want 5, known", maxActive, known)
	}
	if !strings.Contains(logOutput.String(), `"msg":"limit not saved"`) {
		t.Errorf("log %q: want the save failure", logOutput.String())
	}
}

func TestLimits_RefreshPicksUpNewValue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var sourceValue atomic.Int64
		sourceValue.Store(5)
		cache := newCacheWithSource(func(ctx context.Context) (int, error) { return int(sourceValue.Load()), nil })
		ctx, cancel := context.WithCancel(t.Context())
		cache.Init(ctx)
		var refreshing sync.WaitGroup
		refreshing.Go(func() { cache.Run(ctx) })

		sourceValue.Store(8)
		time.Sleep(time.Minute + time.Second)
		synctest.Wait()
		maxActive, known := cache.MaxActive()
		cancel()
		refreshing.Wait()

		if maxActive != 8 || !known {
			t.Errorf("MaxActive one refresh after the source changed to 8: got %d, known %t, want 8, known", maxActive, known)
		}
	})
}

func TestLimits_StopsWhenContextCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newCacheWithSource(func(ctx context.Context) (int, error) { return 5, nil })
		ctx, cancel := context.WithCancel(t.Context())
		runFinished := make(chan struct{})
		go func() {
			cache.Run(ctx)
			close(runFinished)
		}()

		cancel()
		synctest.Wait()

		select {
		case <-runFinished:
		default:
			t.Error("Run still running after its context was cancelled")
		}
	})
}

func newCacheWithSource(fetchMaxParallelChunks func(ctx context.Context) (int, error)) *Cache {
	return &Cache{
		fetchMaxParallelChunks: fetchMaxParallelChunks,
		refreshInterval:        time.Minute,
		store:                  &fakeStore{},
		logger:                 slog.New(slog.DiscardHandler),
	}
}

type fakeStore struct {
	mutex       sync.Mutex
	loadedValue int
	loadFound   bool
	loadErr     error
	saveErr     error
	savedValues []int
}

func (store *fakeStore) LoadMaxParallelChunks(ctx context.Context) (int, bool, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	return store.loadedValue, store.loadFound, store.loadErr
}

func (store *fakeStore) SaveMaxParallelChunks(ctx context.Context, maxParallelChunks int) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.savedValues = append(store.savedValues, maxParallelChunks)
	return store.saveErr
}
