// Package outbox implements the transactional outbox pattern for reliable event publishing.
package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"sync"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/ent/outboxevent"
	"github.com/cloud-agent-platform/cap/internal/observability/tracing"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// OutboxEventForwarder is the interface for forwarding events to a message broker.
// It is implemented by the Redis Stream forwarder.
type OutboxEventForwarder interface {
	// Forward sends the event to the message broker.
	// Returns nil on success, error on failure.
	Forward(ctx context.Context, event *ent.OutboxEvent) error
}

// RedisStreamForwarder implements OutboxEventForwarder using Redis Streams.
// It forwards domain events to a Redis Stream for consumption by downstream systems.
type RedisStreamForwarder struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisStreamForwarder creates a new RedisStreamForwarder.
func NewRedisStreamForwarder(client *redis.Client, logger *zap.Logger) *RedisStreamForwarder {
	if logger == nil {
		panic("logger is required")
	}
	return &RedisStreamForwarder{
		client: client,
		logger: logger,
	}
}

// Forward publishes the outbox event to a Redis Stream.
// The stream key is "stream:domain_events" and the event data is stored as hash fields.
func (f *RedisStreamForwarder) Forward(ctx context.Context, event *ent.OutboxEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Start outbox.publish span
	tracer := tracing.NewSpanHelper()
	ctx, span := tracer.StartOutboxPublish(ctx, event.EventType, event.ID)

	// Build the event data as a map of field names to values
	fields := map[string]interface{}{
		"id":               event.ID,
		"aggregate_type":   event.AggregateType,
		"aggregate_id":     event.AggregateID,
		"event_type":       event.EventType,
		"payload":          string(event.Payload),
		"occurred_at":      strconv.FormatInt(event.OccurredAt, 10),
		"idempotency_key":  event.IdempotencyKey,
		"status":           event.Status,
		"retry_count":      strconv.Itoa(event.RetryCount),
		"last_error":       event.LastError,
		"created_at":       strconv.FormatInt(event.CreatedAt, 10),
	}

	// Use XADD to append the event to the Redis Stream
	result, err := f.client.XAdd(ctx, &redis.XAddArgs{
		Stream: DomainEventStreamKey,
		ID:     "*", // Auto-generate ID
		Values: fields,
	}).Result()

	if err != nil {
		f.logger.Error("failed to forward event to Redis Stream",
			zap.String("event_id", event.ID),
			zap.String("event_type", event.EventType),
			zap.Error(err),
		)
		tracing.EndSpanWithError(span, err)
		return fmt.Errorf("failed to XADD to stream: %w", err)
	}

	f.logger.Debug("event forwarded to Redis Stream",
		zap.String("event_id", event.ID),
		zap.String("stream_id", result),
		zap.String("event_type", event.EventType),
	)
	tracing.EndSpan(span)
	return nil
}

// Verify interface implementation at compile time.
var _ OutboxEventForwarder = (*RedisStreamForwarder)(nil)

// OutboxPoller polls the outbox table for pending events and forwards them to Redis Stream.
// It runs as a background process and supports graceful shutdown via context cancellation.
type OutboxPoller struct {
	client    *ent.Client
	db        *sql.DB
	forwarder OutboxEventForwarder
	logger    *zap.Logger
	config    PollerConfig

	// Internal state
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewOutboxPoller creates a new OutboxPoller with the given configuration.
func NewOutboxPoller(client *ent.Client, db *sql.DB, forwarder OutboxEventForwarder, logger *zap.Logger, config PollerConfig) (*OutboxPoller, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid poller config: %w", err)
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	return &OutboxPoller{
		client:    client,
		db:        db,
		forwarder: forwarder,
		logger:    logger,
		config:    config,
	}, nil
}

// Start begins polling for pending events in a background goroutine.
// It returns immediately. Use the context passed to Run() for shutdown signaling.
func (p *OutboxPoller) Start(ctx context.Context) {
	p.ctx, p.cancel = context.WithCancel(ctx)

	p.wg.Add(1)
	go p.run()
	p.logger.Info("outbox poller started",
		zap.Duration("poll_interval", p.config.PollInterval),
		zap.Int("batch_size", p.config.BatchSize),
		zap.Int("max_retries", p.config.MaxRetries),
	)
}

// Stop gracefully stops the poller, waiting for any in-flight events to complete.
func (p *OutboxPoller) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	p.logger.Info("outbox poller stopped")
	return nil
}

// run is the main polling loop.
func (p *OutboxPoller) run() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			p.logger.Debug("outbox poller received shutdown signal")
			return
		case <-ticker.C:
			if err := p.pollAndForward(p.ctx); err != nil {
				p.logger.Error("poll cycle failed",
					zap.Error(err),
				)
			}
		}
	}
}

// pollAndForward fetches pending events and forwards them to the message broker.
func (p *OutboxPoller) pollAndForward(ctx context.Context) error {
	// Fetch pending events using FOR UPDATE SKIP LOCKED to prevent duplicate processing
	events, err := p.fetchPendingEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch pending events: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	p.logger.Debug("fetched pending events",
		zap.Int("count", len(events)),
	)

	var lastErr error
	for _, event := range events {
		if err := p.processEvent(ctx, event); err != nil {
			lastErr = err
			p.logger.Error("failed to process event",
				zap.String("event_id", event.ID),
				zap.String("event_type", event.EventType),
				zap.Int("retry_count", event.RetryCount),
				zap.Error(err),
			)
			continue
		}
	}

	return lastErr
}

// fetchPendingEvents retrieves pending events from the outbox table.
// It uses SELECT ... FOR UPDATE SKIP LOCKED to handle concurrent poller instances.
func (p *OutboxPoller) fetchPendingEvents(ctx context.Context) ([]*ent.OutboxEvent, error) {
	// Query pending events ordered by created_at
	// We use raw SQL to implement FOR UPDATE SKIP LOCKED
	query := `
		SELECT id, aggregate_type, aggregate_id, event_type, payload,
		       occurred_at, idempotency_key, status, retry_count, last_error, created_at
		FROM outbox_events
		WHERE status = $1
		  AND retry_count < $2
		ORDER BY created_at ASC
		LIMIT $3
		FOR UPDATE SKIP LOCKED
	`

	rows, err := p.db.QueryContext(ctx, query, StatusPending, p.config.MaxRetries, p.config.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var events []*ent.OutboxEvent
	for rows.Next() {
		var e ent.OutboxEvent
		var payload []byte
		var lastError entsql.NullString

		err := rows.Scan(
			&e.ID,
			&e.AggregateType,
			&e.AggregateID,
			&e.EventType,
			&payload,
			&e.OccurredAt,
			&e.IdempotencyKey,
			&e.Status,
			&e.RetryCount,
			&lastError,
			&e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		e.Payload = payload
		if lastError.Valid {
			e.LastError = lastError.String
		}
		events = append(events, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return events, nil
}

// processEvent forwards a single event to the message broker and updates its status.
func (p *OutboxPoller) processEvent(ctx context.Context, event *ent.OutboxEvent) error {
	// Forward to Redis Stream
	if err := p.forwarder.Forward(ctx, event); err != nil {
		// Mark as failed and increment retry count
		return p.markFailed(ctx, event, err.Error())
	}

	// Mark as published
	if err := p.markPublished(ctx, event.ID); err != nil {
		p.logger.Error("failed to mark event as published",
			zap.String("event_id", event.ID),
			zap.Error(err),
		)
		// Event was forwarded successfully but we couldn't mark it as published.
		// This is acceptable - on next poll, it will be a duplicate forward.
	}

	return nil
}

// markPublished updates the event status to "published".
func (p *OutboxPoller) markPublished(ctx context.Context, eventID string) error {
	affected, err := p.client.OutboxEvent.Update().
		Where(outboxevent.ID(eventID)).
		SetStatus(StatusPublished).
		Save(ctx)

	if err != nil {
		return fmt.Errorf("failed to update status to published: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("event not found: %s", eventID)
	}

	p.logger.Debug("event marked as published",
		zap.String("event_id", eventID),
	)
	return nil
}

// markFailed updates the event status to "failed" and increments the retry count.
// When retry_count >= 3, it sleeps for exponential backoff before returning,
// so the next poll cycle will respect the backoff delay.
func (p *OutboxPoller) markFailed(ctx context.Context, event *ent.OutboxEvent, errMsg string) error {
	newRetryCount := event.RetryCount + 1

	// Check if we've exceeded max retries
	if newRetryCount >= p.config.MaxRetries {
		_, err := p.client.OutboxEvent.Update().
			Where(outboxevent.ID(event.ID)).
			SetStatus(StatusFailed).
			SetRetryCount(newRetryCount).
			SetLastError(errMsg).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to mark as failed: %w", err)
		}
		p.logger.Warn("event marked as permanently failed",
			zap.String("event_id", event.ID),
			zap.Int("retry_count", newRetryCount),
			zap.String("last_error", errMsg),
		)
		return nil
	}

	// Apply exponential backoff when retry_count >= 3 before returning.
	// This ensures the next poll cycle respects the backoff delay.
	// Formula: sleep = min(2^retry_count * 100ms, 30s)
	if event.RetryCount >= 3 {
		backoff := PollerRetryBackoff(event.RetryCount)
		p.logger.Debug("applying exponential backoff before next retry",
			zap.String("event_id", event.ID),
			zap.Int("retry_count", newRetryCount),
			zap.Duration("backoff", backoff),
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}

	// Just increment retry count and update last error
	_, err := p.client.OutboxEvent.Update().
		Where(outboxevent.ID(event.ID)).
		SetRetryCount(newRetryCount).
		SetLastError(errMsg).
		Save(ctx)

	if err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	return nil
}

// QueryPendingCount returns the number of pending events (for monitoring).
func (p *OutboxPoller) QueryPendingCount(ctx context.Context) (int, error) {
	return p.client.OutboxEvent.Query().
		Where(outboxevent.Status(StatusPending)).
		Count(ctx)
}

// QueryFailedCount returns the number of failed events (for monitoring).
func (p *OutboxPoller) QueryFailedCount(ctx context.Context) (int, error) {
	return p.client.OutboxEvent.Query().
		Where(outboxevent.Status(StatusFailed)).
		Count(ctx)
}

// PurgePublished removes published events older than the given cutoff time.
// This is a maintenance operation that should be called periodically.
func (p *OutboxPoller) PurgePublished(ctx context.Context, olderThan time.Time) (int, error) {
	return p.client.OutboxEvent.Delete().
		Where(
			outboxevent.Status(StatusPublished),
			outboxevent.CreatedAtLTE(olderThan.UnixNano()),
		).
		Exec(ctx)
}
