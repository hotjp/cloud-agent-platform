// Package cache implements L1-Storage Redis client and caching primitives.
// Uses go-redis/v9 for Redis connectivity.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domaincontext "github.com/cloud-agent-platform/cap/internal/domain/context"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TaskContextCache provides hot-layer caching for TaskContext using Redis.
// It stores TaskContext with automatic TTL management to prevent memory leaks.
type TaskContextCache struct {
	client *redis.Client
	logger *zap.Logger
}

// NewTaskContextCache creates a new TaskContextCache instance.
func NewTaskContextCache(client *redis.Client, logger *zap.Logger) (*TaskContextCache, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	return &TaskContextCache{
		client: client,
		logger:  logger,
	}, nil
}

// contextTaskKey generates the Redis key for a task context.
// Format: context:task:{taskID}
func contextTaskKey(taskID string) string {
	return fmt.Sprintf("context:task:%s", taskID)
}

// Get retrieves a TaskContext from cache by taskID.
// Returns ErrCacheMiss if the key does not exist.
func (tc *TaskContextCache) Get(ctx context.Context, taskID string) (*domaincontext.TaskContext, error) {
	if taskID == "" {
		return nil, fmt.Errorf("taskID is required")
	}

	key := contextTaskKey(taskID)
	val, err := tc.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		tc.logger.Warn("task context cache get error",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("task context cache get: %w", err)
	}

	var taskCtx domaincontext.TaskContext
	if err := json.Unmarshal([]byte(val), &taskCtx); err != nil {
		tc.logger.Warn("task context cache unmarshal error",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("task context cache unmarshal: %w", err)
	}

	tc.logger.Debug("task context cache hit",
		zap.String("task_id", taskID),
	)

	return &taskCtx, nil
}

// Set stores a TaskContext in cache with the specified TTL.
// TTL must be greater than 0. All writes are automatically set with TTL.
func (tc *TaskContextCache) Set(ctx context.Context, taskCtx *domaincontext.TaskContext, ttl time.Duration) error {
	if taskCtx == nil {
		return fmt.Errorf("task context is required")
	}
	if taskCtx.TaskID == "" {
		return fmt.Errorf("task context taskID is required")
	}
	if ttl <= 0 {
		return fmt.Errorf("ttl must be greater than 0")
	}

	key := contextTaskKey(taskCtx.TaskID)
	raw, err := json.Marshal(taskCtx)
	if err != nil {
		return fmt.Errorf("task context marshal: %w", err)
	}

	if err := tc.client.Set(ctx, key, raw, ttl).Err(); err != nil {
		tc.logger.Warn("task context cache set error",
			zap.String("task_id", taskCtx.TaskID),
			zap.Error(err),
		)
		return fmt.Errorf("task context cache set: %w", err)
	}

	tc.logger.Debug("task context cache set",
		zap.String("task_id", taskCtx.TaskID),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// Delete removes a TaskContext from cache.
func (tc *TaskContextCache) Delete(ctx context.Context, taskID string) error {
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}

	key := contextTaskKey(taskID)
	if err := tc.client.Del(ctx, key).Err(); err != nil {
		tc.logger.Warn("task context cache delete error",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return fmt.Errorf("task context cache delete: %w", err)
	}

	tc.logger.Debug("task context cache deleted",
		zap.String("task_id", taskID),
	)

	return nil
}

// Invalidate removes multiple TaskContext entries matching the given taskID pattern.
// Uses SCAN instead of KEYS for production safety.
// Pattern should be a taskID prefix or partial match.
func (tc *TaskContextCache) Invalidate(ctx context.Context, taskIDPattern string) error {
	if taskIDPattern == "" {
		return fmt.Errorf("taskID pattern is required")
	}

	pattern := contextTaskKey(taskIDPattern) + "*"
	var cursor uint64
	var deleted int64

	for {
		keys, nextCursor, err := tc.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			tc.logger.Warn("task context cache scan error",
				zap.String("pattern", pattern),
				zap.Error(err),
			)
			return fmt.Errorf("task context cache scan: %w", err)
		}

		if len(keys) > 0 {
			n, err := tc.client.Del(ctx, keys...).Result()
			if err != nil {
				tc.logger.Warn("task context cache delete error during scan",
					zap.String("pattern", pattern),
					zap.Error(err),
				)
			} else {
				deleted += n
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if deleted > 0 {
		tc.logger.Debug("task context cache invalidated",
			zap.String("pattern", pattern),
			zap.Int64("deleted", deleted),
		)
	}

	return nil
}

// Ping checks the Redis connection.
func (tc *TaskContextCache) Ping(ctx context.Context) error {
	return tc.client.Ping(ctx).Err()
}

// Verify interface implementation at compile time.
var _ = (*TaskContextCache)(nil)
