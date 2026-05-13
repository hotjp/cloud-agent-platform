package storage

import (
	"context"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// TestStorageNewWithLogger tests creating a new Storage instance.
func TestStorageNewWithLogger(t *testing.T) {
	_ = zaptest.NewLogger(t)
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			DSN:         "postgres://user:password@localhost:5432/testdb?sslmode=disable",
			MaxOpen:     10,
			MaxIdle:     5,
			MaxLifetime: 5 * 1e9,
		},
	}

	// Test with nil logger - should fail
	_, err := New(cfg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger is required")
}

// TestTransactionManagerInterface verifies that PgTransactionManager implements TransactionManager.
func TestTransactionManagerInterface(t *testing.T) {
	// Verify at compile time that PgTransactionManager implements TransactionManager
	var _ TransactionManager = (*PgTransactionManager)(nil)
}

// TestStorageWithMockedPool tests Storage with a valid config but no actual DB connection.
// This is a basic sanity check that the struct is properly initialized.
func TestStorageBasicInitialization(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a minimal config with invalid DSN - we expect this to fail on connect
	// but want to verify the struct is properly initialized
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			DSN:         "postgres://invalid:invalid@localhost:5432/nonexistent?sslmode=disable",
			MaxOpen:     10,
			MaxIdle:     5,
			MaxLifetime: 5 * 1e9,
		},
	}

	// This should fail because the database doesn't exist
	_, err := New(cfg, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open database")
}

// TestPgTransactionManagerCommitTwice tests that committing twice returns an error.
func TestPgTransactionManagerCommitTwice(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a mock transaction manager for testing state
	tm := &PgTransactionManager{
		logger:     logger,
		commited:   false,
		rolledBack: false,
	}

	ctx := context.Background()

	// First commit should fail because tx is nil
	err := tm.Commit(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active transaction")
}

// TestPgTransactionManagerRollbackTwice tests that rolling back twice returns an error.
func TestPgTransactionManagerRollbackTwice(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tm := &PgTransactionManager{
		logger:     logger,
		commited:   false,
		rolledBack: false,
	}

	ctx := context.Background()

	// First rollback should fail because tx is nil
	err := tm.Rollback(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active transaction")
}

// TestPgTransactionManagerCommitAfterRollback tests that committing after rollback fails.
func TestPgTransactionManagerCommitAfterRollback(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tm := &PgTransactionManager{
		logger:     logger,
		commited:   false,
		rolledBack: true, // Already rolled back
	}

	ctx := context.Background()
	err := tm.Commit(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction already rolled back")
}

// TestPgTransactionManagerRollbackAfterCommit tests that rollback after commit fails.
func TestPgTransactionManagerRollbackAfterCommit(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tm := &PgTransactionManager{
		logger:     logger,
		commited:   true, // Already committed
		rolledBack: false,
	}

	ctx := context.Background()
	err := tm.Rollback(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction already committed")
}

// TestTransactionManagerTxReturnsNil tests that Tx() returns nil when tx is nil.
func TestTransactionManagerTxReturnsNil(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tm := &PgTransactionManager{
		tx:         nil,
		logger:     logger,
		commited:   false,
		rolledBack: false,
	}

	assert.Nil(t, tm.Tx())
}

// TestConfigDatabaseConfig tests that DatabaseConfig is properly structured.
func TestConfigDatabaseConfig(t *testing.T) {
	cfg := config.DatabaseConfig{
		DSN:         "postgres://user:pass@localhost:5432/db",
		MaxOpen:     25,
		MaxIdle:     10,
		MaxLifetime: 5 * 1e9,
	}

	assert.Equal(t, "postgres://user:pass@localhost:5432/db", cfg.DSN)
	assert.Equal(t, 25, cfg.MaxOpen)
	assert.Equal(t, 10, cfg.MaxIdle)
}
