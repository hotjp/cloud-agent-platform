// Package storage implements L1-Storage layer: cleanup worker for expired objects.
package storage

import (
	"context"
	"sync"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"

	"go.uber.org/zap"
)

// CleanupConfig holds configuration for the cleanup worker.
type CleanupConfig struct {
	// Interval is the interval between cleanup runs.
	Interval time.Duration
	// ObjectTTL is the TTL for objects before they are considered expired.
	ObjectTTL time.Duration
	// BatchSize is the number of objects to delete per cleanup cycle.
	BatchSize int
}

// DefaultCleanupConfig returns the default cleanup configuration (90 day TTL, 1 hour interval).
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		Interval:  1 * time.Hour,
		ObjectTTL: 90 * 24 * time.Hour, // 90 days
		BatchSize: 100,
	}
}

// Validate validates the cleanup configuration.
func (c CleanupConfig) Validate() error {
	if c.Interval <= 0 {
		return &ConfigError{Field: "Interval", Message: "must be positive"}
	}
	if c.ObjectTTL <= 0 {
		return &ConfigError{Field: "ObjectTTL", Message: "must be positive"}
	}
	if c.BatchSize <= 0 {
		return &ConfigError{Field: "BatchSize", Message: "must be positive"}
	}
	return nil
}

// ConfigError represents a configuration validation error.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "invalid cleanup config: " + e.Field + " " + e.Message
}

// CleanupWorker is a background worker that cleans up expired objects from cold storage.
type CleanupWorker struct {
	storage domain.ObjectStorage
	logger  *zap.Logger
	config  CleanupConfig

	// Internal state
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCleanupWorker creates a new cleanup worker.
func NewCleanupWorker(storage domain.ObjectStorage, logger *zap.Logger, config CleanupConfig) (*CleanupWorker, error) {
	if storage == nil {
		return nil, &ConfigError{Field: "storage", Message: "is required"}
	}
	if logger == nil {
		return nil, &ConfigError{Field: "logger", Message: "is required"}
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &CleanupWorker{
		storage: storage,
		logger:  logger,
		config:  config,
	}, nil
}

// Start begins the cleanup worker in a background goroutine.
// It returns immediately. Use the context passed for shutdown signaling.
func (w *CleanupWorker) Start(ctx context.Context) {
	w.ctx, w.cancel = context.WithCancel(ctx)

	w.wg.Add(1)
	go w.run()

	w.logger.Info("cleanup worker started",
		zap.Duration("interval", w.config.Interval),
		zap.Duration("object_ttl", w.config.ObjectTTL),
		zap.Int("batch_size", w.config.BatchSize),
		zap.String("bucket", w.storage.BucketName()),
	)
}

// Stop gracefully stops the cleanup worker, waiting for any in-flight deletions to complete.
func (w *CleanupWorker) Stop() error {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	w.logger.Info("cleanup worker stopped")
	return nil
}

// run is the main cleanup loop.
func (w *CleanupWorker) run() {
	defer w.wg.Done()

	// Run immediately on start, then on interval
	w.cleanup()

	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			w.logger.Debug("cleanup worker received shutdown signal")
			return
		case <-ticker.C:
			w.cleanup()
		}
	}
}

// cleanup performs a single cleanup cycle.
func (w *CleanupWorker) cleanup() {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Minute)
	defer cancel()

	// Find expired objects
	expiredKeys, err := w.storage.ListExpiredObjects(ctx, w.config.ObjectTTL)
	if err != nil {
		w.logger.Error("failed to list expired objects",
			zap.Duration("object_ttl", w.config.ObjectTTL),
			zap.Error(err),
		)
		return
	}

	if len(expiredKeys) == 0 {
		w.logger.Debug("no expired objects found")
		return
	}

	w.logger.Info("found expired objects to delete",
		zap.Int("count", len(expiredKeys)),
	)

	// Delete in batches
	var deleted int
	var failed int

	for i := 0; i < len(expiredKeys); i += w.config.BatchSize {
		end := i + w.config.BatchSize
		if end > len(expiredKeys) {
			end = len(expiredKeys)
		}

		batch := expiredKeys[i:end]
		for _, key := range batch {
			if err := w.storage.Delete(ctx, key); err != nil {
				w.logger.Warn("failed to delete expired object",
					zap.String("key", key),
					zap.Error(err),
				)
				failed++
				continue
			}
			deleted++
		}
	}

	w.logger.Info("cleanup cycle completed",
		zap.Int("deleted", deleted),
		zap.Int("failed", failed),
		zap.Int("total", len(expiredKeys)),
	)
}

// Verify interface implementation at compile time.
var _ = (interface {
	Start(ctx context.Context)
	Stop() error
})((*CleanupWorker)(nil))
