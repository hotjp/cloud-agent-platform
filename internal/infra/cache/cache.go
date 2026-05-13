// Package cache implements L1-Storage Redis client and caching primitives.
// Uses go-redis/v9 for Redis connectivity.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ErrCacheMiss is returned when a key does not exist in cache.
var ErrCacheMiss = errors.New("cache miss")

// Cache defines the interface for cache operations.
// All keys must follow the category:entity:qualifier format.
type Cache interface {
	// Get retrieves a value from cache by key.
	// Returns ErrCacheMiss if the key does not exist.
	Get(ctx context.Context, key string, dest any) error

	// Set stores a value in cache with the specified TTL.
	// TTL must be greater than 0.
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// Delete removes a key from cache.
	Delete(ctx context.Context, key string) error

	// Invalidate removes multiple keys matching the given pattern.
	// Uses SCAN instead of KEYS for production safety.
	// Pattern should follow category:entity:* format.
	Invalidate(ctx context.Context, pattern string) error

	// Ping checks the Redis connection.
	Ping(ctx context.Context) error
}

// RedisCache implements Cache using go-redis/v9.
type RedisCache struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisCache creates a new RedisCache instance.
func NewRedisCache(client *redis.Client, logger *zap.Logger) (*RedisCache, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	return &RedisCache{
		client: client,
		logger:  logger,
	}, nil
}

// Get retrieves a value from cache by key.
// Returns ErrCacheMiss if the key does not exist.
func (c *RedisCache) Get(ctx context.Context, key string, dest any) error {
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}
		c.logger.Warn("cache get error",
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("cache get: %w", err)
	}

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		c.logger.Warn("cache unmarshal error",
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("cache unmarshal: %w", err)
	}

	return nil
}

// Set stores a value in cache with the specified TTL.
// TTL must be greater than 0.
func (c *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("ttl must be greater than 0")
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}

	if err := c.client.Set(ctx, key, raw, ttl).Err(); err != nil {
		c.logger.Warn("cache set error",
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("cache set: %w", err)
	}

	return nil
}

// Delete removes a key from cache.
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.logger.Warn("cache delete error",
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("cache delete: %w", err)
	}
	return nil
}

// Invalidate removes multiple keys matching the given pattern.
// Uses SCAN instead of KEYS for production safety.
// Pattern should follow category:entity:* format.
func (c *RedisCache) Invalidate(ctx context.Context, pattern string) error {
	var cursor uint64
	var deleted int64

	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			c.logger.Warn("cache scan error",
				zap.String("pattern", pattern),
				zap.Error(err),
			)
			return fmt.Errorf("cache scan: %w", err)
		}

		if len(keys) > 0 {
			n, err := c.client.Del(ctx, keys...).Result()
			if err != nil {
				c.logger.Warn("cache delete error during scan",
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
		c.logger.Debug("cache invalidated",
			zap.String("pattern", pattern),
			zap.Int64("deleted", deleted),
		)
	}

	return nil
}

// Ping checks the Redis connection.
func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Verify interface implementation at compile time.
var _ Cache = (*RedisCache)(nil)