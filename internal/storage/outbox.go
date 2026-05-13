// Package storage implements L1-Storage layer: Ent ORM implementation,
// PostgreSQL + Redis connections, transaction management, and Outbox polling.
package storage

import (
	"context"
	"fmt"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/internal/domain"

	"go.uber.org/zap"
)

// Outbox event status constants.
const (
	// StatusPending indicates the event is waiting to be published.
	StatusPending = "pending"
	// StatusPublished indicates the event has been successfully published to Redis Stream.
	StatusPublished = "published"
	// StatusFailed indicates the event failed to publish and will be retried.
	StatusFailed = "failed"
)

// OutboxWriter writes domain events to the outbox table within the same database
// transaction as the aggregate root update, ensuring atomicity.
type OutboxWriter struct {
	client *ent.Client
	logger *zap.Logger
}

// NewOutboxWriter creates a new OutboxWriter that writes events within database transactions.
//
// The returned OutboxWriter implements domain.OutboxWriter interface.
// It writes events to the outbox_events table with status="pending" and retry_count=0.
// Events are forwarded to Redis Stream by the OutboxPoller background process.
func NewOutboxWriter(client *ent.Client, logger *zap.Logger) (*OutboxWriter, error) {
	if client == nil {
		return nil, fmt.Errorf("ent client is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	return &OutboxWriter{
		client: client,
		logger: logger,
	}, nil
}

// Write inserts the given domain event into the outbox table within the provided transaction.
// This method implements the domain.OutboxWriter interface.
//
// The event is written with status="pending" and will be picked up by the OutboxPoller
// for forwarding to Redis Stream.
//
// txInterface must be *ent.Tx or storage.TransactionManager.
//
// Returns an error if:
//   - event is nil
//   - txInterface is not a valid transaction type
//   - the event cannot be inserted into the database
func (w *OutboxWriter) Write(ctx context.Context, txInterface interface{}, event *domain.DomainEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Extract *ent.Tx from the interface
	var tx *ent.Tx
	switch v := txInterface.(type) {
	case *ent.Tx:
		tx = v
	case TransactionManager:
		tx = v.Tx()
	default:
		return fmt.Errorf("tx must be *ent.Tx or storage.TransactionManager, got %T", txInterface)
	}

	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}

	// Create the outbox event entity within the transaction
	create := tx.OutboxEvent.Create().
		SetID(event.EventID).
		SetAggregateType(event.AggregateType).
		SetAggregateID(event.AggregateID).
		SetEventType(event.EventType).
		SetPayload(event.Payload).
		SetOccurredAt(event.OccurredAt.UnixNano()).
		SetIdempotencyKey(event.IdempotencyKey).
		SetStatus(StatusPending)

	if err := create.Exec(ctx); err != nil {
		// Check for unique constraint violation (idempotency)
		if ent.IsConstraintError(err) {
			w.logger.Debug("outbox event already exists (idempotent)",
				zap.String("event_id", event.EventID),
				zap.String("idempotency_key", event.IdempotencyKey),
			)
			return nil
		}
		w.logger.Error("failed to write outbox event",
			zap.String("event_id", event.EventID),
			zap.String("aggregate_type", event.AggregateType),
			zap.String("aggregate_id", event.AggregateID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to write outbox event: %w", err)
	}

	w.logger.Debug("outbox event written",
		zap.String("event_id", event.EventID),
		zap.String("event_type", event.EventType),
		zap.String("idempotency_key", event.IdempotencyKey),
	)
	return nil
}

// Verify interface at compile time.
var _ domain.OutboxWriter = (*OutboxWriter)(nil)