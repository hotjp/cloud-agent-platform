// Package cache implements L1-Storage Redis client and caching primitives.
// Uses go-redis/v9 for Redis connectivity.
package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ErrWatchdogNotStarted is returned when trying to stop a watchdog that hasn't started.
var ErrWatchdogNotStarted = errors.New("watchdog not started")

// WatchdogLock wraps a Lock with automatic TTL renewal.
// It periodically extends the lock before it expires, ensuring long-running
// operations don't lose their lock due to TTL expiration.
type WatchdogLock struct {
	*Lock
	client *redis.Client
	logger *zap.Logger

	mu       sync.Mutex
	stopCh   chan struct{}
	stopped  bool
	stoppedCh chan struct{}
}

// NewWatchdogLock creates a new WatchdogLock wrapping the provided Lock.
// The lock must have been acquired via DistributedLock.Acquire.
func NewWatchdogLock(lock *Lock, client *redis.Client, logger *zap.Logger) (*WatchdogLock, error) {
	if lock == nil {
		return nil, fmt.Errorf("lock is required")
	}
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	return &WatchdogLock{
		Lock:     lock,
		client:  client,
		logger:  logger,
		stopCh:  make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}, nil
}

// StartWatchdog starts a background goroutine that automatically extends the lock.
// It extends the lock every `interval` duration. The watchdog stops when:
// - Stop() is called
// - The context is cancelled
// - The lock is no longer held (TTL expired without renewal)
//
// The interval should be less than half the original TTL to ensure
// the lock is always extended before it expires.
func (wl *WatchdogLock) StartWatchdog(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be greater than 0")
	}

	wl.mu.Lock()
	if wl.stopped {
		wl.mu.Unlock()
		return fmt.Errorf("watchdog already stopped")
	}
	wl.mu.Unlock()

	go wl.runWatchdog(ctx, interval)

	wl.logger.Debug("watchdog started",
		zap.String("key", wl.key),
		zap.Duration("interval", interval),
	)

	return nil
}

// runWatchdog is the internal watchdog loop.
func (wl *WatchdogLock) runWatchdog(ctx context.Context, interval time.Duration) {
	defer close(wl.stoppedCh)

	for {
		select {
		case <-ctx.Done():
			wl.logger.Debug("watchdog stopped due to context cancellation",
				zap.String("key", wl.key),
			)
			return
		case <-wl.stopCh:
			wl.logger.Debug("watchdog stopped explicitly",
				zap.String("key", wl.key),
			)
			return
		case <-time.After(interval):
			// Extend the lock
			wl.mu.Lock()
			if wl.stopped {
				wl.mu.Unlock()
				return
			}
			wl.mu.Unlock()

			// Try to extend the lock
			if err := wl.Extend(ctx, interval); err != nil {
				if errors.Is(err, ErrLockNotHeld) {
					wl.logger.Warn("watchdog: lock no longer held, stopping",
						zap.String("key", wl.key),
					)
				} else {
					wl.logger.Warn("watchdog: failed to extend lock",
						zap.String("key", wl.key),
						zap.Error(err),
					)
				}
				return
			}

			wl.logger.Debug("watchdog: lock extended",
				zap.String("key", wl.key),
				zap.Duration("interval", interval),
			)
		}
	}
}

// Stop stops the watchdog and releases the lock.
// This is equivalent to calling Release() but ensures the watchdog goroutine exits first.
func (wl *WatchdogLock) Stop(ctx context.Context) error {
	wl.mu.Lock()
	if wl.stopped {
		wl.mu.Unlock()
		return nil
	}
	wl.stopped = true
	wl.mu.Unlock()

	// Signal the watchdog to stop
	close(wl.stopCh)

	// Wait for the watchdog goroutine to exit
	select {
	case <-wl.stoppedCh:
		// Already exited
	case <-ctx.Done():
		return fmt.Errorf("watchdog stop: %w", ctx.Err())
	}

	// Release the lock
	return wl.Release(ctx)
}

// StopOnly stops the watchdog WITHOUT releasing the lock.
// Use this when you want to stop the auto-renewal but keep the lock.
func (wl *WatchdogLock) StopOnly() {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	if wl.stopped {
		return
	}
	wl.stopped = true
	close(wl.stopCh)
}

// Verify interface implementation at compile time.
var _ = (*WatchdogLock)(nil)
