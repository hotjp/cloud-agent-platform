// Package cache provides unit tests for Redis cache and distributed lock.
package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestLogger(t *testing.T) *zap.Logger {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	return logger
}

func newTestRedisClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return client, mr
}

func TestRedisCache_Get_Set(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewRedisCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Test Set and Get
	err = cache.Set(ctx, "test:entity:123", map[string]any{"name": "test"}, time.Hour)
	require.NoError(t, err)

	var result map[string]any
	err = cache.Get(ctx, "test:entity:123", &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["name"])

	// Test CacheMiss
	err = cache.Get(ctx, "test:entity:non-existent", &result)
	assert.True(t, errors.Is(err, ErrCacheMiss))
}

func TestRedisCache_Delete(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewRedisCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = cache.Set(ctx, "test:entity:123", "value", time.Hour)
	require.NoError(t, err)

	err = cache.Delete(ctx, "test:entity:123")
	require.NoError(t, err)

	var result string
	err = cache.Get(ctx, "test:entity:123", &result)
	assert.True(t, errors.Is(err, ErrCacheMiss))
}

func TestRedisCache_Invalidate(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewRedisCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Set multiple keys
	err = cache.Set(ctx, "test:entity:1", "v1", time.Hour)
	require.NoError(t, err)
	err = cache.Set(ctx, "test:entity:2", "v2", time.Hour)
	require.NoError(t, err)
	err = cache.Set(ctx, "test:other:3", "v3", time.Hour)
	require.NoError(t, err)

	// Invalidate by pattern
	err = cache.Invalidate(ctx, "test:entity:*")
	require.NoError(t, err)

	// Verify only matching keys are deleted
	var result string
	err = cache.Get(ctx, "test:entity:1", &result)
	assert.True(t, errors.Is(err, ErrCacheMiss))
	err = cache.Get(ctx, "test:entity:2", &result)
	assert.True(t, errors.Is(err, ErrCacheMiss))

	// Other key should still exist
	err = cache.Get(ctx, "test:other:3", &result)
	require.NoError(t, err)
	assert.Equal(t, "v3", result)
}

func TestRedisCache_Set_TTL_Validation(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewRedisCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = cache.Set(ctx, "test:entity:123", "value", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ttl must be greater than 0")

	err = cache.Set(ctx, "test:entity:123", "value", -time.Second)
	assert.Error(t, err)
}

func TestRedisCache_Ping(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewRedisCache(client, logger)
	require.NoError(t, err)

	err = cache.Ping(context.Background())
	require.NoError(t, err)
}

func TestDistributedLock_Acquire_Release(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Acquire lock
	lock, err := dl.Acquire(ctx, "test:entity:lock1", time.Hour)
	require.NoError(t, err)
	require.NotNil(t, lock)

	// Release lock
	err = lock.Release(ctx)
	require.NoError(t, err)

	// Another acquire should succeed now
	lock2, err := dl.Acquire(ctx, "test:entity:lock1", time.Hour)
	require.NoError(t, err)
	require.NotNil(t, lock2)
	lock2.Release(ctx)
}

func TestDistributedLock_Acquire_Contention(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// First acquire
	lock1, err := dl.Acquire(ctx, "test:entity:lock2", time.Hour)
	require.NoError(t, err)

	// Second acquire should fail
	lock2, err := dl.Acquire(ctx, "test:entity:lock2", time.Hour)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLockAcquireFailed))
	assert.Nil(t, lock2)

	// Release first lock
	lock1.Release(ctx)

	// Now second acquire should succeed
	lock2, err = dl.Acquire(ctx, "test:entity:lock2", time.Hour)
	require.NoError(t, err)
	require.NotNil(t, lock2)
	lock2.Release(ctx)
}

func TestDistributedLock_Extend(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	lock, err := dl.Acquire(ctx, "test:entity:lock3", time.Hour)
	require.NoError(t, err)

	// Extend lock
	err = lock.Extend(ctx, 2*time.Hour)
	require.NoError(t, err)

	// Release should still work
	err = lock.Release(ctx)
	require.NoError(t, err)
}

func TestDistributedLock_Extend_After_Release(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	lock, err := dl.Acquire(ctx, "test:entity:lock4", time.Hour)
	require.NoError(t, err)

	// Release first
	lock.Release(ctx)

	// Extend should fail
	err = lock.Extend(ctx, 2*time.Hour)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLockNotHeld))
}

func TestDistributedLock_Release_Not_Held(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	lock, err := dl.Acquire(ctx, "test:entity:lock5", time.Hour)
	require.NoError(t, err)

	// Manually expire the key via miniredis
	mr.FastForward(time.Hour + time.Second)

	// Release should fail
	err = lock.Release(ctx)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLockNotHeld))
}

func TestDistributedLock_Concurrent_Acquire(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()
	key := "test:entity:concurrent-lock"

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	acquired := make(chan int, 1) // Buffer to catch first winner
	errs := make([]error, 0, goroutines-1)
	var mu sync.Mutex

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			lock, err := dl.Acquire(ctx, key, time.Hour)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}
			acquired <- id
			time.Sleep(10 * time.Millisecond)
			lock.Release(ctx)
		}(i)
	}

	wg.Wait()
	close(acquired)

	// Only one goroutine should have acquired the lock
	assert.Len(t, acquired, 1)
	// Others should have received ErrLockAcquireFailed
	for _, err := range errs {
		assert.True(t, errors.Is(err, ErrLockAcquireFailed))
	}
}

func TestDistributedLock_TTL_Expiration(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	lock, err := dl.Acquire(ctx, "test:entity:ttl-lock", 50*time.Millisecond)
	require.NoError(t, err)

	// Advance miniredis time to trigger TTL expiration
	mr.FastForward(100 * time.Millisecond)

	// Another acquire should succeed since first lock expired
	lock2, err := dl.Acquire(ctx, "test:entity:ttl-lock", time.Hour)
	require.NoError(t, err)
	require.NotNil(t, lock2)
	lock.Release(ctx) // Release first lock (its TTL already expired)
	lock2.Release(ctx)
}

func TestNewRedisCache_InvalidArgs(t *testing.T) {
	logger := newTestLogger(t)

	_, err := NewRedisCache(nil, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis client is required")

	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	_, err = NewRedisCache(client, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger is required")
	client.Close()
}

func TestNewDistributedLock_InvalidArgs(t *testing.T) {
	logger := newTestLogger(t)

	_, err := NewDistributedLock(nil, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis client is required")

	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	_, err = NewDistributedLock(client, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger is required")
	client.Close()
}

func TestDistributedLock_Acquire_InvalidArgs(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	dl, err := NewDistributedLock(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = dl.Acquire(ctx, "", time.Hour)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key is required")

	_, err = dl.Acquire(ctx, "test:entity:123", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ttl must be greater than 0")

	_, err = dl.Acquire(ctx, "test:entity:123", -time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ttl must be greater than 0")
}