// Package cache implements L1-Storage Redis client and caching primitives.
// Uses go-redis/v9 for Redis connectivity.
package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ErrLockNotHeld is returned when trying to release or extend a lock not held by the caller.
var ErrLockNotHeld = errors.New("lock not held")

// ErrLockAcquireFailed is returned when failing to acquire a lock.
var ErrLockAcquireFailed = errors.New("failed to acquire lock")

// DistributedLock provides distributed locking using Redis SET NX with expiration.
// Implements the Redlock-like single-instance pattern.
type DistributedLock struct {
	client *redis.Client
	logger *zap.Logger
}

// NewDistributedLock creates a new DistributedLock instance.
func NewDistributedLock(client *redis.Client, logger *zap.Logger) (*DistributedLock, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	return &DistributedLock{
		client: client,
		logger:  logger,
	}, nil
}

// Lock represents an acquired lock with its token for release/extend.
type Lock struct {
	key   string
	token string
	dl    *DistributedLock
}

// keyPrefix is the prefix for all lock keys.
const lockKeyPrefix = "lock:"

// Acquire attempts to acquire a lock with the given key and TTL.
// Returns a Lock handle if successful, or ErrLockAcquireFailed if the lock is already held.
// The key should follow category:entity:qualifier format.
func (dl *DistributedLock) Acquire(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("ttl must be greater than 0")
	}

	lockKey := lockKeyPrefix + key
	token := uuid.New().String()

	// SET NX with expiration (SET key value NX EX seconds)
	ok, err := dl.client.SetNX(ctx, lockKey, token, ttl).Result()
	if err != nil {
		dl.logger.Warn("lock acquire error",
			zap.String("key", key),
			zap.Error(err),
		)
		return nil, fmt.Errorf("lock acquire: %w", err)
	}

	if !ok {
		dl.logger.Debug("lock already held",
			zap.String("key", key),
		)
		return nil, ErrLockAcquireFailed
	}

	dl.logger.Debug("lock acquired",
		zap.String("key", key),
		zap.String("token", token),
		zap.Duration("ttl", ttl),
	)

	return &Lock{
		key:   key,
		token: token,
		dl:    dl,
	}, nil
}

// Release releases the lock if it is still held by this token.
// Returns ErrLockNotHeld if the lock is no longer held.
func (l *Lock) Release(ctx context.Context) error {
	lockKey := lockKeyPrefix + l.key

	// Use Lua script to ensure atomic check-and-delete
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`)

	result, err := script.Run(ctx, l.dl.client, []string{lockKey}, l.token).Int64()
	if err != nil {
		l.dl.logger.Warn("lock release error",
			zap.String("key", l.key),
			zap.Error(err),
		)
		return fmt.Errorf("lock release: %w", err)
	}

	if result == 0 {
		l.dl.logger.Debug("lock not held",
			zap.String("key", l.key),
		)
		return ErrLockNotHeld
	}

	l.dl.logger.Debug("lock released",
		zap.String("key", l.key),
	)

	return nil
}

// Extend extends the TTL of the lock if it is still held by this token.
// Returns ErrLockNotHeld if the lock is no longer held.
func (l *Lock) Extend(ctx context.Context, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("ttl must be greater than 0")
	}

	lockKey := lockKeyPrefix + l.key

	// Use Lua script to ensure atomic check-and-extend
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)

	result, err := script.Run(ctx, l.dl.client, []string{lockKey}, l.token, ttl.Milliseconds()).Int64()
	if err != nil {
		l.dl.logger.Warn("lock extend error",
			zap.String("key", l.key),
			zap.Error(err),
		)
		return fmt.Errorf("lock extend: %w", err)
	}

	if result == 0 {
		l.dl.logger.Debug("lock not held",
			zap.String("key", l.key),
		)
		return ErrLockNotHeld
	}

	l.dl.logger.Debug("lock extended",
		zap.String("key", l.key),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// Verify interface implementation at compile time.
var _ = (*DistributedLock)(nil)