// Package cache implements L1-Storage Redis client and caching primitives.
// Uses go-redis/v9 for Redis connectivity.
package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/config"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// NewRedisClient creates a new Redis client using go-redis/v9.
func NewRedisClient(cfg *config.RedisConfig, logger *zap.Logger) (*redis.Client, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if cfg == nil {
		return nil, fmt.Errorf("redis config is required")
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Verify connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	logger.Info("redis connection established",
		zap.String("layer", "L1"),
		zap.String("addr", cfg.Addr),
		zap.Int("db", cfg.DB),
	)

	return client, nil
}

// Verify interface implementation at compile time.
var _ = (*redis.Client)(nil)