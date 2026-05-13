// Package outbox provides unit tests for the outbox pattern implementation.
package outbox

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/cloud-agent-platform/cap/ent"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestLogger(t *testing.T) *zap.Logger {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	return logger
}

func newTestRedisClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return client, mr
}

// ----------------------------------------------------------------------------
// PollerConfig tests
// ----------------------------------------------------------------------------

func TestPollerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  PollerConfig
		wantErr string
	}{
		{
			name: "valid config",
			config: PollerConfig{
				PollInterval: 1 * time.Second,
				BatchSize:    100,
				MaxRetries:   5,
				LockDuration: 30 * time.Second,
				LockKey:      "lock:outbox",
			},
			wantErr: "",
		},
		{
			name: "zero poll interval",
			config: PollerConfig{
				PollInterval: 0,
				BatchSize:    100,
				MaxRetries:   5,
				LockDuration: 30 * time.Second,
				LockKey:      "lock:outbox",
			},
			wantErr: "poll_interval must be positive",
		},
		{
			name: "negative poll interval",
			config: PollerConfig{
				PollInterval: -1 * time.Second,
				BatchSize:    100,
				MaxRetries:   5,
				LockDuration: 30 * time.Second,
				LockKey:      "lock:outbox",
			},
			wantErr: "poll_interval must be positive",
		},
		{
			name: "zero batch size",
			config: PollerConfig{
				PollInterval: 1 * time.Second,
				BatchSize:    0,
				MaxRetries:   5,
				LockDuration: 30 * time.Second,
				LockKey:      "lock:outbox",
			},
			wantErr: "batch_size must be positive",
		},
		{
			name: "negative max retries",
			config: PollerConfig{
				PollInterval: 1 * time.Second,
				BatchSize:    100,
				MaxRetries:   -1,
				LockDuration: 30 * time.Second,
				LockKey:      "lock:outbox",
			},
			wantErr: "max_retries must be non-negative",
		},
		{
			name: "zero lock duration",
			config: PollerConfig{
				PollInterval: 1 * time.Second,
				BatchSize:    100,
				MaxRetries:   5,
				LockDuration: 0,
				LockKey:      "lock:outbox",
			},
			wantErr: "lock_duration must be positive",
		},
		{
			name: "empty lock key",
			config: PollerConfig{
				PollInterval: 1 * time.Second,
				BatchSize:    100,
				MaxRetries:   5,
				LockDuration: 30 * time.Second,
				LockKey:      "",
			},
			wantErr: "lock_key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDefaultPollerConfig(t *testing.T) {
	cfg := DefaultPollerConfig()
	assert.Equal(t, 1*time.Second, cfg.PollInterval)
	assert.Equal(t, 100, cfg.BatchSize)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 30*time.Second, cfg.LockDuration)
	assert.Equal(t, "lock:outbox_poller", cfg.LockKey)
}

// ----------------------------------------------------------------------------
// RetryBackoff tests
// ----------------------------------------------------------------------------

func TestRetryBackoff(t *testing.T) {
	tests := []struct {
		retryCount int
		wantMin    time.Duration
		wantMax    time.Duration
	}{
		{0, 1 * time.Second, 2 * time.Second},
		{1, 2 * time.Second, 4 * time.Second},
		{2, 4 * time.Second, 8 * time.Second},
		{3, 8 * time.Second, 16 * time.Second},
		{4, 16 * time.Second, 32 * time.Second},
		{5, 32 * time.Second, 64 * time.Second},
		{10, 5 * time.Minute, 5 * time.Minute}, // Should cap at 5 minutes
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := RetryBackoff(tt.retryCount)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}

// ----------------------------------------------------------------------------
// RedisStreamForwarder tests
// ----------------------------------------------------------------------------

func TestRedisStreamForwarder_Forward_Success(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	forwarder := NewRedisStreamForwarder(client, logger)

	ctx := context.Background()

	// Create a test event using ent.OutboxEvent
	event := &ent.OutboxEvent{
		ID:             "test-event-1",
		AggregateType:  "Task",
		AggregateID:    "task-123",
		EventType:      "TaskCreatedV1",
		Payload:        []byte(`{"foo":"bar"}`),
		OccurredAt:     time.Now().UnixNano(),
		IdempotencyKey: "task-123:TaskCreatedV1:1",
		Status:         StatusPending,
		RetryCount:     0,
		LastError:      "",
		CreatedAt:      time.Now().UnixNano(),
	}

	err := forwarder.Forward(ctx, event)
	assert.NoError(t, err)

	// Verify the event was added to the stream
	result, err := client.XLen(ctx, DomainEventStreamKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), result)

	// Verify stream contents
	events, err := client.XRange(ctx, DomainEventStreamKey, "-", "+").Result()
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "test-event-1", events[0].Values["id"])
	assert.Equal(t, "Task", events[0].Values["aggregate_type"])
	assert.Equal(t, "TaskCreatedV1", events[0].Values["event_type"])
}

func TestRedisStreamForwarder_Forward_NilEvent(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	forwarder := NewRedisStreamForwarder(client, logger)

	ctx := context.Background()

	err := forwarder.Forward(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event is nil")
}

func TestRedisStreamForwarder_MultipleEvents(t *testing.T) {
	client, mr := newTestRedisClient(t)
	defer client.Close()
	defer mr.Close()

	logger := newTestLogger(t)
	forwarder := NewRedisStreamForwarder(client, logger)

	ctx := context.Background()

	// Forward multiple events
	for i := 0; i < 5; i++ {
		event := &ent.OutboxEvent{
			ID:             "event-" + string(rune('A'+i)),
			AggregateType:  "Task",
			AggregateID:    "task-123",
			EventType:      "TaskUpdatedV1",
			Payload:        []byte(`{}`),
			OccurredAt:     time.Now().UnixNano(),
			IdempotencyKey: "task-123:TaskUpdatedV1:" + string(rune('1'+i)),
			Status:         StatusPending,
			RetryCount:     0,
			LastError:      "",
			CreatedAt:      time.Now().UnixNano(),
		}
		err := forwarder.Forward(ctx, event)
		assert.NoError(t, err)
	}

	// Verify all events were added
	result, err := client.XLen(ctx, DomainEventStreamKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(5), result)
}

// ----------------------------------------------------------------------------
// PollerRetryBackoff tests
// ----------------------------------------------------------------------------

func TestPollerRetryBackoff(t *testing.T) {
	tests := []struct {
		retryCount int
		wantMin    time.Duration
		wantMax    time.Duration
	}{
		{0, 100 * time.Millisecond, 200 * time.Millisecond},
		{1, 200 * time.Millisecond, 400 * time.Millisecond},
		{2, 400 * time.Millisecond, 800 * time.Millisecond},
		{3, 800 * time.Millisecond, 1600 * time.Millisecond},
		{4, 1600 * time.Millisecond, 3200 * time.Millisecond},
		{5, 3200 * time.Millisecond, 6400 * time.Millisecond},
		{10, 30 * time.Second, 30 * time.Second}, // Should cap at 30 seconds
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := PollerRetryBackoff(tt.retryCount)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}

// ----------------------------------------------------------------------------
// OutboxPoller tests
// ----------------------------------------------------------------------------

// ----------------------------------------------------------------------------
// NewOutboxPoller validation tests
// ----------------------------------------------------------------------------

func TestNewOutboxPoller_InvalidConfig(t *testing.T) {
	validLogger := newTestLogger(t)

	tests := []struct {
		name    string
		db      *sql.DB
		logger  *zap.Logger
		config  PollerConfig
		wantErr string
	}{
		{
			name:    "zero poll interval",
			db:      nil,
			logger:  validLogger,
			config: PollerConfig{
				PollInterval: 0,
				BatchSize:    100,
				MaxRetries:   5,
				LockDuration: 30 * time.Second,
				LockKey:      "lock:outbox",
			},
			wantErr: "poll_interval must be positive",
		},
		{
			name:    "nil db",
			db:      nil,
			logger:  validLogger,
			config: PollerConfig{
				PollInterval: 1 * time.Second,
				BatchSize:    100,
				MaxRetries:   5,
				LockDuration: 30 * time.Second,
				LockKey:      "lock:outbox",
			},
			wantErr: "database connection is required",
		},
		{
			name:    "nil logger",
			db:      &sql.DB{},
			logger:  nil,
			config: PollerConfig{
				PollInterval: 1 * time.Second,
				BatchSize:    100,
				MaxRetries:   5,
				LockDuration: 30 * time.Second,
				LockKey:      "lock:outbox",
			},
			wantErr: "logger is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poller, err := NewOutboxPoller(nil, tt.db, NewMockOutboxEventForwarder(), tt.logger, tt.config)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, poller)
		})
	}
}

// ----------------------------------------------------------------------------
// OutboxPoller Start/Stop tests
// Note: Testing Start/Stop requires a real DB connection or extensive mocking.
// These tests verify that Stop() can be called safely without Start().
// ----------------------------------------------------------------------------

func TestOutboxPoller_StopWithoutStart(t *testing.T) {
	logger := newTestLogger(t)
	config := DefaultPollerConfig()

	poller := &OutboxPoller{
		logger:    logger,
		config:    config,
		forwarder: NewMockOutboxEventForwarder(),
	}

	// Stop without Start should not panic
	err := poller.Stop()
	assert.NoError(t, err)
}
