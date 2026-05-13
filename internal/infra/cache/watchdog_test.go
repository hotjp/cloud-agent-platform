// Package cache provides unit tests for Redis cache and distributed lock.
package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchdogLock_StartWatchdog(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Acquire lock with short TTL
	lock, err := dl.Acquire(ctx, "watchdog:test:lock1", 100*time.Millisecond)
	require.NoError(t, err)

	wl, err := NewWatchdogLock(lock, client, logger)
	require.NoError(t, err)

	// Start watchdog with interval shorter than TTL
	err = wl.StartWatchdog(ctx, 50*time.Millisecond)
	require.NoError(t, err)

	// Wait for a few watchdog cycles
	time.Sleep(150 * time.Millisecond)

	// Lock should still be held (watchdog extended it)
	// We can verify by trying to acquire the same key - it should fail
	_, err = dl.Acquire(ctx, "watchdog:test:lock1", time.Hour)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLockAcquireFailed))

	// Stop the watchdog
	err = wl.Stop(ctx)
	require.NoError(t, err)

	// Now acquiring should succeed
	lock2, err := dl.Acquire(ctx, "watchdog:test:lock1", time.Hour)
	require.NoError(t, err)
	lock2.Release(ctx)
}

func TestWatchdogLock_Stop(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	lock, err := dl.Acquire(ctx, "watchdog:test:lock2", time.Hour)
	require.NoError(t, err)

	wl, err := NewWatchdogLock(lock, client, logger)
	require.NoError(t, err)

	err = wl.StartWatchdog(ctx, 50*time.Millisecond)
	require.NoError(t, err)

	// Stop should release the lock
	err = wl.Stop(ctx)
	require.NoError(t, err)

	// Lock should be released, new acquire should work
	lock2, err := dl.Acquire(ctx, "watchdog:test:lock2", time.Hour)
	require.NoError(t, err)
	lock2.Release(ctx)
}

func TestWatchdogLock_StopOnly(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	lock, err := dl.Acquire(ctx, "watchdog:test:lock3", time.Hour)
	require.NoError(t, err)

	wl, err := NewWatchdogLock(lock, client, logger)
	require.NoError(t, err)

	err = wl.StartWatchdog(ctx, 50*time.Millisecond)
	require.NoError(t, err)

	// StopOnly should stop watchdog but NOT release the lock
	wl.StopOnly()

	// Lock should still be held
	_, err = dl.Acquire(ctx, "watchdog:test:lock3", time.Hour)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLockAcquireFailed))

	// Release manually
	err = lock.Release(ctx)
	require.NoError(t, err)

	// Now acquire should work
	lock2, err := dl.Acquire(ctx, "watchdog:test:lock3", time.Hour)
	require.NoError(t, err)
	lock2.Release(ctx)
}

func TestWatchdogLock_ContextCancellation(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	lock, err := dl.Acquire(ctx, "watchdog:test:lock4", time.Hour)
	require.NoError(t, err)

	wl, err := NewWatchdogLock(lock, client, logger)
	require.NoError(t, err)

	err = wl.StartWatchdog(ctx, 50*time.Millisecond)
	require.NoError(t, err)

	// Wait for context to expire
	time.Sleep(200 * time.Millisecond)

	// Use a fresh context for verification since original context is cancelled
	verifyCtx := context.Background()

	// Lock should still be held since watchdog stopped but didn't release
	_, err = dl.Acquire(verifyCtx, "watchdog:test:lock4", time.Hour)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLockAcquireFailed))

	// Manual release should work
	err = lock.Release(verifyCtx)
	require.NoError(t, err)
}

func TestWatchdogLock_InvalidArgs(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	lock, err := dl.Acquire(ctx, "watchdog:test:lock5", time.Hour)
	require.NoError(t, err)

	// Test nil lock
	_, err = NewWatchdogLock(nil, client, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "lock is required")

	// Test nil client
	_, err = NewWatchdogLock(lock, nil, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis client is required")

	// Test nil logger
	_, err = NewWatchdogLock(lock, client, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger is required")

	lock.Release(ctx)
}

func TestWatchdogLock_StartWatchdog_InvalidInterval(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	lock, err := dl.Acquire(ctx, "watchdog:test:lock6", time.Hour)
	require.NoError(t, err)

	wl, err := NewWatchdogLock(lock, client, logger)
	require.NoError(t, err)

	// Test zero interval
	err = wl.StartWatchdog(ctx, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "interval must be greater than 0")

	// Test negative interval
	err = wl.StartWatchdog(ctx, -time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "interval must be greater than 0")

	lock.Release(ctx)
}

func TestWatchdogLock_ConcurrentStop(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	lock, err := dl.Acquire(ctx, "watchdog:test:lock7", time.Hour)
	require.NoError(t, err)

	wl, err := NewWatchdogLock(lock, client, logger)
	require.NoError(t, err)

	err = wl.StartWatchdog(ctx, 50*time.Millisecond)
	require.NoError(t, err)

	// Concurrent stop calls
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		wl.Stop(ctx)
	}()

	go func() {
		defer wg.Done()
		wl.Stop(ctx)
	}()

	wg.Wait()

	// Should not panic or error
}

func TestWatchdogLock_DoubleStop(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	lock, err := dl.Acquire(ctx, "watchdog:test:lock8", time.Hour)
	require.NoError(t, err)

	wl, err := NewWatchdogLock(lock, client, logger)
	require.NoError(t, err)

	err = wl.StartWatchdog(ctx, 50*time.Millisecond)
	require.NoError(t, err)

	// First stop
	err = wl.Stop(ctx)
	require.NoError(t, err)

	// Second stop should be no-op
	err = wl.Stop(ctx)
	require.NoError(t, err)
}
