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

func TestLimits_KeepsLastValidValueOnFailure(t *testing.T) {
	testCases := []struct {
		name   string
		status int
		body   string
	}{
		{"server_error", http.StatusInternalServerError, `{"MAX_PARALLEL_CHUNKS": 9}`},
		{"body_over_1_mib", http.StatusOK, `{"MAX_PARALLEL_CHUNKS": 9, "PADDING": "` + strings.Repeat("x", 1<<20) + `"}`},
		{"value_as_string", http.StatusOK, `{"MAX_PARALLEL_CHUNKS": "9"}`},
		{"zero", http.StatusOK, `{"MAX_PARALLEL_CHUNKS": 0}`},
		{"missing_key", http.StatusOK, `{"OTHER_SETTING": 9}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var sourceFailing atomic.Bool
			source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if !sourceFailing.Load() {
					_, _ = writer.Write([]byte(realisticPayload))
					return
				}
				writer.WriteHeader(testCase.status)
				_, _ = writer.Write([]byte(testCase.body))
			}))
			defer source.Close()
			var logOutput bytes.Buffer
			cache := New(Config{SourceURL: source.URL, FetchTimeout: time.Second, RefreshInterval: time.Minute}, &fakeStore{}, slog.New(slog.NewJSONHandler(&logOutput, nil)))
			cache.Init(t.Context())

			sourceFailing.Store(true)
			cache.refresh(t.Context())

			maxActive, known := cache.MaxActive()
			if maxActive != 5 || !known {
				t.Errorf("MaxActive after a failed refresh: got %d, known %t, want the last valid 5", maxActive, known)
			}
			if !strings.Contains(logOutput.String(), `"level":"WARN","msg":"limit not refreshed"`) {
				t.Errorf("log %q: want a warning for the failed refresh", logOutput.String())
			}
		})
	}
}

func TestLimits_KeepsLastValidValueWhenSourceTimesOut(t *testing.T) {
	var sourceSlow atomic.Bool
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if sourceSlow.Load() {
			<-request.Context().Done()
			return
		}
		_, _ = writer.Write([]byte(realisticPayload))
	}))
	defer source.Close()
	var logOutput bytes.Buffer
	cache := New(Config{SourceURL: source.URL, FetchTimeout: 50 * time.Millisecond, RefreshInterval: time.Minute}, &fakeStore{}, slog.New(slog.NewJSONHandler(&logOutput, nil)))
	cache.Init(t.Context())

	sourceSlow.Store(true)
	cache.refresh(t.Context())

	maxActive, known := cache.MaxActive()
	if maxActive != 5 || !known {
		t.Errorf("MaxActive after a refresh slower than the timeout: got %d, known %t, want the last valid 5", maxActive, known)
	}
	if !strings.Contains(logOutput.String(), `"level":"WARN","msg":"limit not refreshed"`) {
		t.Errorf("log %q: want a warning for the timed out refresh", logOutput.String())
	}
}

func TestLimits_NeverLogsBody(t *testing.T) {
	var sourceInvalid atomic.Bool
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if sourceInvalid.Load() {
			_, _ = writer.Write([]byte(`{"ANALYTICS_TOKEN": "fake-token-for-tests", "MAX_PARALLEL_CHUNKS": "5"}`))
			return
		}
		_, _ = writer.Write([]byte(realisticPayload))
	}))
	defer source.Close()
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cache := New(Config{SourceURL: source.URL, FetchTimeout: time.Second, RefreshInterval: time.Minute}, &fakeStore{}, logger)

	cache.Init(t.Context())
	sourceInvalid.Store(true)
	cache.refresh(t.Context())

	for _, payloadPart := range []string{"fake-token-for-tests", "ANALYTICS_TOKEN", "api.example.test"} {
		if strings.Contains(logOutput.String(), payloadPart) {
			t.Errorf("log %q contains %q from the payload", logOutput.String(), payloadPart)
		}
	}
	if !strings.Contains(logOutput.String(), `"msg":"limit updated"`) || !strings.Contains(logOutput.String(), `"msg":"limit not refreshed"`) {
		t.Errorf("log %q: want both the accepted and the rejected payload logged", logOutput.String())
	}
}

func TestLimits_RetriesSaveAfterFailure(t *testing.T) {
	source := newSource(t, http.StatusOK, realisticPayload)
	store := &fakeStore{saveFailuresLeft: 1}
	var logOutput bytes.Buffer
	cache := New(Config{SourceURL: source.URL, FetchTimeout: time.Second, RefreshInterval: time.Minute}, store, slog.New(slog.NewJSONHandler(&logOutput, nil)))

	cache.Init(t.Context())
	cache.refresh(t.Context())
	cache.refresh(t.Context())

	if !slices.Equal(store.savedValues, []int{5, 5}) {
		t.Errorf("save attempts: got %v, want [5 5], a failed save retried once by the next refresh", store.savedValues)
	}
	if failures := strings.Count(logOutput.String(), `"msg":"limit not saved"`); failures != 1 {
		t.Errorf("limit not saved log lines: got %d, want 1 (log %q)", failures, logOutput.String())
	}
}

func TestLimits_RetriesWithBackoffWhileUnknown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attemptTimes []time.Time
		cache := newCacheWithSource(func(ctx context.Context) (int, error) {
			attemptTimes = append(attemptTimes, time.Now())
			return 0, errors.New("limit source down")
		})
		ctx, cancel := context.WithCancel(t.Context())
		cache.Init(ctx)
		var refreshing sync.WaitGroup
		refreshing.Go(func() { cache.Run(ctx) })

		time.Sleep(2 * time.Minute)
		synctest.Wait()
		cancel()
		refreshing.Wait()

		wantGaps := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
		if gaps := gapsBetween(attemptTimes); !slices.Equal(gaps, wantGaps) {
			t.Errorf("gaps between fetch attempts during two minutes of outage: got %v, want %v", gaps, wantGaps)
		}
	})
}

func TestLimits_UnknownUntilFirstSuccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var attemptTimes []time.Time
		failuresLeft := 3
		cache := newCacheWithSource(func(ctx context.Context) (int, error) {
			attemptTimes = append(attemptTimes, time.Now())
			if failuresLeft > 0 {
				failuresLeft--
				return 0, errors.New("limit source down")
			}
			return 5, nil
		})
		ctx, cancel := context.WithCancel(t.Context())
		cache.Init(ctx)
		_, knownAfterInit := cache.MaxActive()
		var refreshing sync.WaitGroup
		refreshing.Go(func() { cache.Run(ctx) })

		time.Sleep(70 * time.Second)
		synctest.Wait()
		maxActive, known := cache.MaxActive()
		cancel()
		refreshing.Wait()

		if knownAfterInit {
			t.Error("MaxActive after Init with the source down: got known, want unknown")
		}
		if maxActive != 5 || !known {
			t.Errorf("MaxActive once the source answers again: got %d, known %t, want 5, known", maxActive, known)
		}
		wantGaps := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, time.Minute}
		if gaps := gapsBetween(attemptTimes); !slices.Equal(gaps, wantGaps) {
			t.Errorf("gaps between fetch attempts, three failures then a recovery: got %v, want %v", gaps, wantGaps)
		}
	})
}

func gapsBetween(times []time.Time) []time.Duration {
	var gaps []time.Duration
	for index := 1; index < len(times); index++ {
		gaps = append(gaps, times[index].Sub(times[index-1]))
	}
	return gaps
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
	mutex            sync.Mutex
	loadedValue      int
	loadFound        bool
	loadErr          error
	saveErr          error
	saveFailuresLeft int
	savedValues      []int
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
	if store.saveFailuresLeft > 0 {
		store.saveFailuresLeft--
		return errors.New("service unavailable: save max_parallel_chunks: disk is full")
	}
	return store.saveErr
}
