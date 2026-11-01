package limits

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

type Store interface {
	LoadMaxParallelChunks(ctx context.Context) (int, bool, error)
	SaveMaxParallelChunks(ctx context.Context, maxParallelChunks int) error
}

type Config struct {
	SourceURL       string
	FetchTimeout    time.Duration
	RefreshInterval time.Duration
}

type Cache struct {
	fetchMaxParallelChunks func(ctx context.Context) (int, error)
	refreshInterval        time.Duration
	store                  Store
	logger                 *slog.Logger
	maxParallelChunks      atomic.Int64
}

func New(config Config, store Store, logger *slog.Logger) *Cache {
	return &Cache{
		fetchMaxParallelChunks: newLimitFetcher(config.SourceURL, config.FetchTimeout).fetchMaxParallelChunks,
		refreshInterval:        config.RefreshInterval,
		store:                  store,
		logger:                 logger,
	}
}

func (cache *Cache) MaxActive() (int, bool) {
	maxParallelChunks := cache.maxParallelChunks.Load()
	return int(maxParallelChunks), maxParallelChunks > 0
}

func (cache *Cache) Init(ctx context.Context) {
	savedValue, found, err := cache.store.LoadMaxParallelChunks(ctx)
	switch {
	case err != nil:
		cache.logger.Warn("saved limit not loaded", "error", err)
	case found:
		cache.maxParallelChunks.Store(int64(savedValue))
	}
	cache.refresh(ctx)
}

func (cache *Cache) Run(ctx context.Context) {
	refreshTicker := time.NewTicker(cache.refreshInterval)
	defer refreshTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-refreshTicker.C:
			cache.refresh(ctx)
		}
	}
}

func (cache *Cache) refresh(ctx context.Context) {
	maxParallelChunks, err := cache.fetchMaxParallelChunks(ctx)
	if err != nil {
		cache.logger.Warn("limit not refreshed", "error", err)
		return
	}
	previousValue := cache.maxParallelChunks.Swap(int64(maxParallelChunks))
	if previousValue == int64(maxParallelChunks) {
		return
	}
	cache.logger.Info("limit updated", "max_parallel_chunks", maxParallelChunks)
	if err := cache.store.SaveMaxParallelChunks(ctx, maxParallelChunks); err != nil {
		cache.logger.Warn("limit not saved", "error", err)
	}
}
