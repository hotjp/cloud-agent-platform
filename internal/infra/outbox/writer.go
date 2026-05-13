// Package outbox implements the transactional outbox pattern for reliable event publishing.
package outbox

import (
	"context"
	"fmt"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/storage"

	"go.uber.org/zap"
)

// OutboxWriterImpl implements the domain.OutboxWriter interface.
// It writes domain events to the outbox table within the same database transaction
// as the aggregate root update, ensuring atomicity.
type OutboxWriterImpl struct {
	client *ent.Client
	logger *zap.Logger
}

// NewOutboxWriter creates a new OutboxWriterImpl.
//
// The OutboxWriterImpl writes events to the outbox_events table within the same
// transaction as the business data, ensuring that either both succeed or both fail.
// This guarantees "at-least-once" delivery semantics.
func NewOutboxWriter(client *ent.Client, logger *zap.Logger) *OutboxWriterImpl {
	if logger == nil {
		panic("logger is required")
	}
	return &OutboxWriterImpl{
		client: client,
		logger: logger,
	}
}

// Write inserts the given domain event into the outbox table within the provided transaction.
// This method implements the domain.OutboxWriter interface.
//
// The event is written with status="pending" and will be picked up by the OutboxPoller
// for forwarding to Redis Stream.
//
// Returns an error if:
//   - tx is not a *ent.Tx
//   - the event cannot be inserted into the database
func (w *OutboxWriterImpl) Write(ctx context.Context, txInterface interface{}, event *domain.DomainEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Extract *ent.Tx from the interface
	var tx *ent.Tx
	switch v := txInterface.(type) {
	case *ent.Tx:
		tx = v
	case storage.TransactionManager:
		tx = v.Tx()
	default:
		return fmt.Errorf("tx must be *ent.Tx or storage.TransactionManager, got %T", txInterface)
	}

	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}

	// Create the outbox event entity
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
		if isUniqueConstraintError(err) {
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

// isUniqueConstraintError checks if the given error is a unique constraint violation.
// This is used to detect duplicate events (idempotency).
func isUniqueConstraintError(err error) bool {
	return ent.IsConstraintError(err)
}

// EntTxOutboxWriter is an OutboxWriter that accepts *ent.Tx directly.
// This is a convenience type for code that works directly with ent transactions.
type EntTxOutboxWriter struct {
	writer *OutboxWriterImpl
}

// NewEntTxOutboxWriter creates a new EntTxOutboxWriter.
func NewEntTxOutboxWriter(client *ent.Client, logger *zap.Logger) *EntTxOutboxWriter {
	return &EntTxOutboxWriter{
		writer: NewOutboxWriter(client, logger),
	}
}

// Write implements domain.OutboxWriter by delegating to the underlying writer.
func (w *EntTxOutboxWriter) Write(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
	return w.writer.Write(ctx, tx, event)
}

// Verify interface implementations at compile time.
var (
	_ domain.OutboxWriter = (*OutboxWriterImpl)(nil)
	_ domain.OutboxWriter = (*EntTxOutboxWriter)(nil)
)
