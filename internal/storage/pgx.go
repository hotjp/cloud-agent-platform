// Package storage implements L1-Storage layer: Ent ORM implementation,
// PostgreSQL + Redis connections, transaction management, and Outbox polling.
package storage

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/internal/config"

	"go.uber.org/zap"
)

// New creates a new Storage instance with the given configuration.
// It sets up the PostgreSQL connection pool using pgx/v5.
func New(cfg *config.Config, logger *zap.Logger) (*Storage, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Create ent driver from DSN.
	// The DSN is passed directly to pgx/v5 stdlib which handles connection pooling internally.
	drv, err := entsql.Open(dialect.Postgres, cfg.Database.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure ent's internal database/sql.DB settings.
	// Note: entsql.Open uses database/sql internally with pgx driver.
	// Pool configuration is done via DSN parameters or by using the stdlib directly.
	db := drv.DB()

	// Apply connection pool settings
	db.SetMaxOpenConns(cfg.Database.MaxOpen)
	db.SetMaxIdleConns(cfg.Database.MaxIdle)
	db.SetConnMaxLifetime(cfg.Database.MaxLifetime)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("pgx connection pool established",
		zap.String("layer", "L1"),
		zap.Int("max_open", cfg.Database.MaxOpen),
		zap.Int("max_idle", cfg.Database.MaxIdle),
	)

	// Create ent client
	client := ent.NewClient(ent.Driver(drv))

	return &Storage{
		cfg:    cfg,
		db:     db,
		client: client,
		logger: logger,
	}, nil
}
