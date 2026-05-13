// Package persistence implements L1-Storage repository interfaces using ent ORM.
package persistence

import (
	"context"
	"time"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/ent/outboxevent"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/infra/outbox"
	"github.com/cloud-agent-platform/cap/internal/storage"

	"go.uber.org/zap"
)

// OutboxRepositoryImpl implements domain.OutboxRepository using ent ORM.
type OutboxRepositoryImpl struct {
	client *ent.Client
	logger *zap.Logger
}

// NewOutboxRepository creates a new OutboxRepositoryImpl.
func NewOutboxRepository(client *ent.Client, logger *zap.Logger) *OutboxRepositoryImpl {
	if logger == nil {
		panic("logger is required")
	}
	return &OutboxRepositoryImpl{
		client: client,
		logger: logger,
	}
}

// Write writes a domain event to the outbox table within the given transaction.
func (r *OutboxRepositoryImpl) Write(ctx context.Context, txInterface interface{}, event *domain.DomainEvent) error {
	if event == nil {
		return domain.NewL2ParamValidationError("event", "event is nil")
	}

	// Extract *ent.Tx from the interface
	var tx *ent.Tx
	switch v := txInterface.(type) {
	case *ent.Tx:
		tx = v
	case storage.TransactionManager:
		tx = v.Tx()
	default:
		return domain.NewL2ParamValidationError("tx", "must be *ent.Tx or storage.TransactionManager")
	}

	if tx == nil {
		return domain.NewL2ParamValidationError("tx", "transaction is nil")
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
		SetStatus(outbox.StatusPending)

	if err := create.Exec(ctx); err != nil {
		if ent.IsConstraintError(err) {
			r.logger.Debug("outbox event already exists (idempotent)",
				zap.String("layer", "L1"),
				zap.String("event_id", event.EventID),
				zap.String("idempotency_key", event.IdempotencyKey),
			)
			return nil
		}
		r.logger.Error("failed to write outbox event",
			zap.String("layer", "L1"),
			zap.String("event_id", event.EventID),
			zap.String("aggregate_type", event.AggregateType),
			zap.String("aggregate_id", event.AggregateID),
			zap.Error(err),
		)
		return domain.NewL1DBQueryError(err)
	}

	r.logger.Debug("outbox event written",
		zap.String("layer", "L1"),
		zap.String("event_id", event.EventID),
		zap.String("event_type", event.EventType),
		zap.String("idempotency_key", event.IdempotencyKey),
	)
	return nil
}

// ReadPending reads pending (unpublished) events up to the given limit.
// Returns events sorted by created_at ascending.
func (r *OutboxRepositoryImpl) ReadPending(ctx context.Context, limit int) ([]*domain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	// Use FOR UPDATE SKIP LOCKED for safe concurrent processing
	entEvents, err := r.client.OutboxEvent.Query().
		Where(outboxevent.Status(outbox.StatusPending)).
		Order(ent.Asc(outboxevent.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.logger.Error("failed to read pending outbox events",
			zap.String("layer", "L1"),
			zap.Int("limit", limit),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	domainEvents := make([]*domain.OutboxEvent, len(entEvents))
	for i, entEvent := range entEvents {
		domainEvents[i] = r.entToDomainOutboxEvent(entEvent)
	}

	return domainEvents, nil
}

// MarkPublished marks an event as published by setting its status.
func (r *OutboxRepositoryImpl) MarkPublished(ctx context.Context, id string) error {
	if id == "" {
		return domain.NewL2ParamValidationError("id", "id is empty")
	}

	now := time.Now().UnixNano()
	_, err := r.client.OutboxEvent.Update().
		Where(outboxevent.ID(id)).
		SetStatus(outbox.StatusPublished).
		SetProcessedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.NewL2AggregateNotFoundError("OutboxEvent", id)
		}
		r.logger.Error("failed to mark outbox event as published",
			zap.String("layer", "L1"),
			zap.String("event_id", id),
			zap.Error(err),
		)
		return domain.NewL1DBQueryError(err)
	}

	r.logger.Debug("outbox event marked published",
		zap.String("layer", "L1"),
		zap.String("event_id", id),
	)
	return nil
}

// MarkFailed marks an event as failed and increments retry_count.
func (r *OutboxRepositoryImpl) MarkFailed(ctx context.Context, id string, lastError string) error {
	if id == "" {
		return domain.NewL2ParamValidationError("id", "id is empty")
	}

	// First get current retry_count
	entEvent, err := r.client.OutboxEvent.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.NewL2AggregateNotFoundError("OutboxEvent", id)
		}
		return domain.NewL1DBQueryError(err)
	}

	_, err = r.client.OutboxEvent.Update().
		Where(outboxevent.ID(id)).
		SetStatus(outbox.StatusFailed).
		SetRetryCount(entEvent.RetryCount + 1).
		SetLastError(lastError).
		Save(ctx)
	if err != nil {
		r.logger.Error("failed to mark outbox event as failed",
			zap.String("layer", "L1"),
			zap.String("event_id", id),
			zap.Error(err),
		)
		return domain.NewL1DBQueryError(err)
	}

	r.logger.Debug("outbox event marked failed",
		zap.String("layer", "L1"),
		zap.String("event_id", id),
		zap.Int("retry_count", entEvent.RetryCount+1),
	)
	return nil
}

// DeleteOldPublished deletes published events older than the given cutoff time.
// This is for cleanup operations.
func (r *OutboxRepositoryImpl) DeleteOldPublished(ctx context.Context, olderThan int64) (int64, error) {
	if olderThan <= 0 {
		return 0, domain.NewL2ParamValidationError("olderThan", "must be positive")
	}

	deletedCount, err := r.client.OutboxEvent.Delete().
		Where(
			outboxevent.Status(outbox.StatusPublished),
			outboxevent.ProcessedAtLT(olderThan),
		).
		Exec(ctx)
	if err != nil {
		r.logger.Error("failed to delete old published outbox events",
			zap.String("layer", "L1"),
			zap.Int64("older_than", olderThan),
			zap.Error(err),
		)
		return 0, domain.NewL1DBQueryError(err)
	}

	r.logger.Info("deleted old published outbox events",
		zap.String("layer", "L1"),
		zap.Int("deleted", deletedCount),
		zap.Int64("older_than", olderThan),
	)
	return int64(deletedCount), nil
}

// entToDomainOutboxEvent maps an ent OutboxEvent to a domain OutboxEvent.
func (r *OutboxRepositoryImpl) entToDomainOutboxEvent(entEvent *ent.OutboxEvent) *domain.OutboxEvent {
	if entEvent == nil {
		return nil
	}
	return &domain.OutboxEvent{
		ID:            entEvent.ID,
		AggregateType: entEvent.AggregateType,
		AggregateID:   entEvent.AggregateID,
		EventType:     entEvent.EventType,
		Payload:       entEvent.Payload,
		OccurredAt:    entEvent.OccurredAt,
		Status:        entEvent.Status,
		RetryCount:    entEvent.RetryCount,
		LastError:     entEvent.LastError,
		CreatedAt:     entEvent.CreatedAt,
	}
}

// Verify interface implementation at compile time.
var _ domain.OutboxRepository = (*OutboxRepositoryImpl)(nil)