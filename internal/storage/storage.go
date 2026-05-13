// Package storage implements L1-Storage layer: Ent ORM implementation,
// PostgreSQL + Redis connections, transaction management, and Outbox polling.
package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/cloud-agent-platform/cap/ent"

	"github.com/cloud-agent-platform/cap/internal/config"

	"go.uber.org/zap"
)

// Storage handles database connections and transactions.
type Storage struct {
	cfg    *config.Config
	db     *sql.DB
	client *ent.Client
	logger *zap.Logger
	tx     *ent.Tx
}

// New creates a new Storage instance with the given configuration.
// It sets up the PostgreSQL connection pool using pgx/v5.
func (s *Storage) Connect(ctx context.Context) error {
	// This method is kept for backward compatibility.
	// Actual connection is established through New(cfg, logger).
	return nil
}

// Client returns the ent client.
func (s *Storage) Client() *ent.Client {
	return s.client
}

// Close closes database connections.
func (s *Storage) Close(ctx context.Context) error {
	if s.db != nil {
		s.db.Close()
		s.logger.Info("pgx connection pool closed", zap.String("layer", "L1"))
	}
	if s.client != nil {
		s.client.Close()
	}
	return nil
}

// BeginTx starts a new transaction and stores it in the Storage.
// The returned TransactionManager must be used for the transaction lifecycle.
func (s *Storage) BeginTx(ctx context.Context) (TransactionManager, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	s.tx = tx
	return &PgTransactionManager{
		tx:         tx,
		logger:     s.logger,
		commited:   false,
		rolledBack: false,
	}, nil
}

// Commit commits the current transaction.
func (s *Storage) Commit(ctx context.Context) error {
	if s.tx == nil {
		return fmt.Errorf("no active transaction")
	}
	err := s.tx.Commit()
	s.tx = nil
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// Rollback rolls back the current transaction.
func (s *Storage) Rollback(ctx context.Context) error {
	if s.tx == nil {
		return fmt.Errorf("no active transaction")
	}
	err := s.tx.Rollback()
	s.tx = nil
	if err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	return nil
}

// TransactionManager defines the interface for transaction operations.
// It is used to manage the lifecycle of a database transaction.
type TransactionManager interface {
	// Commit commits the transaction.
	Commit(ctx context.Context) error

	// Rollback rolls back the transaction.
	Rollback(ctx context.Context) error

	// Tx returns the underlying ent transaction.
	Tx() *ent.Tx
}

// PgTransactionManager implements TransactionManager using ent transactions.
type PgTransactionManager struct {
	tx         *ent.Tx
	logger     *zap.Logger
	commited   bool
	rolledBack bool
}

// Commit commits the transaction.
func (tm *PgTransactionManager) Commit(ctx context.Context) error {
	if tm.commited {
		return fmt.Errorf("transaction already committed")
	}
	if tm.rolledBack {
		return fmt.Errorf("transaction already rolled back")
	}
	if tm.tx == nil {
		return fmt.Errorf("no active transaction")
	}

	if err := tm.tx.Commit(); err != nil {
		tm.logger.Error("transaction commit failed",
			zap.String("layer", "L1"),
			zap.Error(err),
		)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	tm.commited = true
	tm.logger.Debug("transaction committed", zap.String("layer", "L1"))
	return nil
}

// Rollback rolls back the transaction.
func (tm *PgTransactionManager) Rollback(ctx context.Context) error {
	if tm.commited {
		return fmt.Errorf("transaction already committed")
	}
	if tm.rolledBack {
		return fmt.Errorf("transaction already rolled back")
	}
	if tm.tx == nil {
		return fmt.Errorf("no active transaction")
	}

	if err := tm.tx.Rollback(); err != nil {
		tm.logger.Error("transaction rollback failed",
			zap.String("layer", "L1"),
			zap.Error(err),
		)
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}

	tm.rolledBack = true
	tm.logger.Debug("transaction rolled back", zap.String("layer", "L1"))
	return nil
}

// Tx returns the underlying ent transaction.
func (tm *PgTransactionManager) Tx() *ent.Tx {
	return tm.tx
}

// Verify interface implementations at compile time.
var (
	_ TransactionManager = (*PgTransactionManager)(nil)
)
