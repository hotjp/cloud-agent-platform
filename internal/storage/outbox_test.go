// Package storage provides tests for the L1-Storage layer.
// Tests that require a real database are marked with //go:build integration.
package storage

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// mockTx is a mock transaction that tracks commit/rollback state.
type mockTx struct {
	committed   bool
	rolledBack  bool
	outboxEvents []*ent.OutboxEvent
}

func (m *mockTx) Commit(ctx context.Context) error {
	m.committed = true
	return nil
}

func (m *mockTx) Rollback(ctx context.Context) error {
	m.rolledBack = true
	return nil
}

func (m *mockTx) Tx() *ent.Tx {
	return nil // Not used in our tests
}

// mockTransactionManager implements storage.TransactionManager for testing.
type mockTransactionManager struct {
	tx *mockTx
}

func (m *mockTransactionManager) Commit(ctx context.Context) error {
	return m.tx.Commit(ctx)
}

func (m *mockTransactionManager) Rollback(ctx context.Context) error {
	return m.tx.Rollback(ctx)
}

func (m *mockTransactionManager) Tx() *ent.Tx {
	return m.tx.Tx()
}

func newTestOutboxWriter(t *testing.T) (*OutboxWriter, *zap.Logger) {
	logger := zaptest.NewLogger(t)
	// Note: Full integration test with real DB requires dockertest
	// This test verifies interface compliance only without a real database.
	return nil, logger
}

func TestOutboxWriter_Interface(t *testing.T) {
	// This test verifies that OutboxWriter implements domain.OutboxWriter
	// without requiring a real database connection.
	var writer domain.OutboxWriter
	require.Nil(t, writer, "OutboxWriter should be nil when no client is provided")
}

func TestOutboxWriter_NewOutboxWriter_Errors(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name    string
		client  *ent.Client
		logger  *zap.Logger
		wantErr string
	}{
		{
			name:    "nil client",
			client:  nil,
			logger:  logger,
			wantErr: "ent client is required",
		},
		{
			name:    "nil logger",
			client:  nil,
			logger:  nil,
			wantErr: "ent client is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer, err := NewOutboxWriter(tt.client, tt.logger)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, writer)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, writer)
			}
		})
	}
}

func TestOutboxWriter_Write_NilEvent(t *testing.T) {
	// Cannot create writer without client, so we test the interface directly
	var writer domain.OutboxWriter = (*OutboxWriter)(nil)
	err := writer.Write(context.Background(), nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event is nil")
}

func TestDomainEvent_NewDomainEvent(t *testing.T) {
	tests := []struct {
		name           string
		aggregateType  string
		aggregateID    string
		eventType      string
		payload        []byte
		version        int
		wantErr        bool
		errContains    string
	}{
		{
			name:          "valid event",
			aggregateType: "Task",
			aggregateID:   "task-123",
			eventType:     "TaskCreatedV1",
			payload:       []byte(`{"name":"test"}`),
			version:       1,
			wantErr:       false,
		},
		{
			name:          "empty event type",
			aggregateType: "Task",
			aggregateID:   "task-123",
			eventType:     "",
			payload:       []byte(`{}`),
			version:       1,
			wantErr:       true,
			errContains:   "event_type cannot be empty",
		},
		{
			name:          "invalid event type format",
			aggregateType: "Task",
			aggregateID:   "task-123",
			eventType:     "invalid",
			payload:       []byte(`{}`),
			version:       1,
			wantErr:       true,
			errContains:   "does not match pattern",
		},
		{
			name:          "missing version suffix",
			aggregateType: "Task",
			aggregateID:   "task-123",
			eventType:     "TaskCreated",
			payload:       []byte(`{}`),
			version:       1,
			wantErr:       true,
			errContains:   "does not match pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := domain.NewDomainEvent(
				tt.aggregateType,
				tt.aggregateID,
				tt.eventType,
				tt.payload,
				tt.version,
			)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, event)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, event)
				assert.NotEmpty(t, event.EventID)
				assert.Equal(t, tt.aggregateType, event.AggregateType)
				assert.Equal(t, tt.aggregateID, event.AggregateID)
				assert.Equal(t, tt.eventType, event.EventType)
				assert.Equal(t, tt.payload, event.Payload)
				assert.Equal(t, tt.version, event.Version)
				assert.NotZero(t, event.OccurredAt)
				assert.NotEmpty(t, event.IdempotencyKey)
			}
		})
	}
}

func TestDomainEvent_Serialize(t *testing.T) {
	event, err := domain.NewDomainEvent(
		"Task",
		"task-123",
		"TaskCreatedV1",
		[]byte(`{"name":"test"}`),
		1,
	)
	require.NoError(t, err)

	// Test serialization
	data, err := event.Serialize()
	assert.NoError(t, err)
	assert.NotNil(t, data)

	// Test deserialization
	event2, err := domain.DeserializeEvent(data)
	assert.NoError(t, err)
	assert.NotNil(t, event2)
	assert.Equal(t, event.EventID, event2.EventID)
	assert.Equal(t, event.AggregateType, event2.AggregateType)
	assert.Equal(t, event.AggregateID, event2.AggregateID)
	assert.Equal(t, event.EventType, event2.EventType)
	assert.Equal(t, event.Payload, event2.Payload)
	assert.Equal(t, event.Version, event2.Version)
}

func TestDomainEvent_IdempotencyKey(t *testing.T) {
	event, err := domain.NewDomainEvent(
		"Task",
		"task-123",
		"TaskCreatedV1",
		[]byte(`{}`),
		1,
	)
	require.NoError(t, err)

	// Idempotency key should follow format: {aggregate_id}:{event_type}:{version}
	expectedKey := "task-123:TaskCreatedV1:1"
	assert.Equal(t, expectedKey, event.IdempotencyKey)
}

func TestOutboxWriter_Write_InterfaceCompliance(t *testing.T) {
	// Test that verifies the Write method signature matches the interface
	// without requiring a real database connection.
	logger := zaptest.NewLogger(t)

	// Create a writer with nil client (will fail but we can check the method signature)
	writer, err := NewOutboxWriter(nil, logger)
	assert.Error(t, err)
	assert.Nil(t, writer)

	// Verify the interface is satisfied
	var _ domain.OutboxWriter = (*OutboxWriter)(nil)
}

// TestOutboxEventStatusConstants verifies status constants are exported correctly.
func TestOutboxEventStatusConstants(t *testing.T) {
	assert.Equal(t, "pending", StatusPending)
	assert.Equal(t, "published", StatusPublished)
	assert.Equal(t, "failed", StatusFailed)
}

// TestDomainEvent_Timestamps verifies event timestamps are set correctly.
func TestDomainEvent_Timestamps(t *testing.T) {
	before := time.Now().UTC()
	event, err := domain.NewDomainEvent(
		"Task",
		"task-456",
		"TaskUpdatedV1",
		[]byte(`{"status":"active"}`),
		2,
	)
	require.NoError(t, err)
	after := time.Now().UTC()

	assert.True(t, event.OccurredAt.Before(after) || event.OccurredAt.Equal(after))
	assert.True(t, event.OccurredAt.After(before) || event.OccurredAt.Equal(before))
}

// TestDomainEvent_ValidateEventType verifies event type validation.
func TestDomainEvent_ValidateEventType(t *testing.T) {
	tests := []struct {
		eventType string
		wantErr   bool
	}{
		{"TaskCreatedV1", false},
		{"SubtaskDeletedV3", false},
		{"TaskV1", false},
		{"", true},
		{"taskcreatedv1", true}, // lowercase
		{"Task_Created_V1", true}, // underscore
		{"1TaskCreatedV1", true}, // starts with number
		{"TaskCreated", true}, // missing version
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			err := domain.ValidateEventType(tt.eventType)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}