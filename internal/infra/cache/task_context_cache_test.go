// Package cache provides unit tests for Redis cache and distributed lock.
package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	domaincontext "github.com/cloud-agent-platform/cap/internal/domain/context"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskContextCache_Set_Get(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a task context
	taskCtx := domaincontext.NewTaskContext("task-123", "Test task goal")
	taskCtx.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval usage"))

	// Set in cache
	err = cache.Set(ctx, taskCtx, time.Hour)
	require.NoError(t, err)

	// Get from cache
	result, err := cache.Get(ctx, "task-123")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "task-123", result.TaskID)
	assert.Equal(t, "Test task goal", result.Goal)
	assert.Len(t, result.Constraints, 1)
	assert.Equal(t, "security", result.Constraints[0].Category)
}

func TestTaskContextCache_Get_CacheMiss(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Non-existent key
	_, err = cache.Get(ctx, "non-existent-task")
	assert.True(t, errors.Is(err, ErrCacheMiss))
}

func TestTaskContextCache_Get_InvalidArgs(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Empty taskID
	_, err = cache.Get(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "taskID is required")
}

func TestTaskContextCache_Delete(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create and set task context
	taskCtx := domaincontext.NewTaskContext("task-456", "Another test")
	err = cache.Set(ctx, taskCtx, time.Hour)
	require.NoError(t, err)

	// Delete
	err = cache.Delete(ctx, "task-456")
	require.NoError(t, err)

	// Should be cache miss now
	_, err = cache.Get(ctx, "task-456")
	assert.True(t, errors.Is(err, ErrCacheMiss))
}

func TestTaskContextCache_Delete_InvalidArgs(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Empty taskID
	err = cache.Delete(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "taskID is required")
}

func TestTaskContextCache_Set_InvalidArgs(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Nil task context
	err = cache.Set(ctx, nil, time.Hour)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task context is required")

	// Task context with empty taskID
	err = cache.Set(ctx, &domaincontext.TaskContext{}, time.Hour)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task context taskID is required")

	// Zero TTL
	taskCtx := domaincontext.NewTaskContext("task-789", "Test")
	err = cache.Set(ctx, taskCtx, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ttl must be greater than 0")

	// Negative TTL
	err = cache.Set(ctx, taskCtx, -time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ttl must be greater than 0")
}

func TestTaskContextCache_Set_Update(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Initial set
	taskCtx1 := domaincontext.NewTaskContext("task-update", "Original goal")
	err = cache.Set(ctx, taskCtx1, time.Hour)
	require.NoError(t, err)

	// Update with new data
	taskCtx2 := domaincontext.NewTaskContext("task-update", "Updated goal")
	taskCtx2.AddMessage(domaincontext.NewUserMessage("Hello"))
	err = cache.Set(ctx, taskCtx2, time.Hour)
	require.NoError(t, err)

	// Get should return updated version
	result, err := cache.Get(ctx, "task-update")
	require.NoError(t, err)
	assert.Equal(t, "Updated goal", result.Goal)
	assert.Len(t, result.Messages, 1)
}

func TestTaskContextCache_TTL(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Set with very short TTL
	taskCtx := domaincontext.NewTaskContext("task-ttl", "TTL test")
	err = cache.Set(ctx, taskCtx, 50*time.Millisecond)
	require.NoError(t, err)

	// Should be retrievable immediately
	result, err := cache.Get(ctx, "task-ttl")
	require.NoError(t, err)
	assert.Equal(t, "task-ttl", result.TaskID)

	// Fast forward time in miniredis
	mr.FastForward(100 * time.Millisecond)

	// Should be cache miss now
	_, err = cache.Get(ctx, "task-ttl")
	assert.True(t, errors.Is(err, ErrCacheMiss))
}

func TestTaskContextCache_Invalidate(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Set multiple task contexts
	tasks := []string{"task-inv-1", "task-inv-2", "task-other"}
	for _, id := range tasks {
		taskCtx := domaincontext.NewTaskContext(id, "Goal for "+id)
		err = cache.Set(ctx, taskCtx, time.Hour)
		require.NoError(t, err)
	}

	// Invalidate by pattern - should delete task-inv-1 and task-inv-2
	err = cache.Invalidate(ctx, "task-inv-")
	require.NoError(t, err)

	// task-inv-1 and task-inv-2 should be gone
	for _, id := range []string{"task-inv-1", "task-inv-2"} {
		_, err = cache.Get(ctx, id)
		assert.True(t, errors.Is(err, ErrCacheMiss), "expected cache miss for %s", id)
	}

	// task-other should still exist
	result, err := cache.Get(ctx, "task-other")
	require.NoError(t, err)
	assert.Equal(t, "task-other", result.TaskID)
}

func TestTaskContextCache_Invalidate_InvalidArgs(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Empty pattern
	err = cache.Invalidate(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "taskID pattern is required")
}

func TestTaskContextCache_Ping(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	cache, err := NewTaskContextCache(client, logger)
	require.NoError(t, err)

	err = cache.Ping(context.Background())
	require.NoError(t, err)
}

func TestNewTaskContextCache_InvalidArgs(t *testing.T) {
	logger := newTestLogger(t)

	// Nil client
	_, err := NewTaskContextCache(nil, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis client is required")

	// Nil logger
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	_, err = NewTaskContextCache(client, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger is required")
	client.Close()
}
