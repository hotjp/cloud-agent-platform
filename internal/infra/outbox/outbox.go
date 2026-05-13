// Package outbox implements the transactional outbox pattern for reliable event publishing.
// It consists of OutboxWriter (writes events to DB within the same transaction as business data)
// and OutboxPoller (background process that forwards events to Redis Stream).
package outbox

import (
	"fmt"
	"time"
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

// Redis Stream key for domain events.
const DomainEventStreamKey = "stream:domain_events"

// PollerConfig holds configuration for the OutboxPoller.
type PollerConfig struct {
	// PollInterval is the interval between polling cycles.
	PollInterval time.Duration
	// BatchSize is the maximum number of events to process per poll cycle.
	BatchSize int
	// MaxRetries is the maximum number of retry attempts before marking an event as failed.
	MaxRetries int
	// LockDuration is how long a single poller instance holds the lock before releasing it.
	LockDuration time.Duration
	// LockKey is the Redis key used for distributed locking.
	LockKey string
}

// DefaultPollerConfig returns the default poller configuration.
func DefaultPollerConfig() PollerConfig {
	return PollerConfig{
		PollInterval: 1 * time.Second,
		BatchSize:    100,
		MaxRetries:   5,
		LockDuration: 30 * time.Second,
		LockKey:      "lock:outbox_poller",
	}
}

// Validate checks that the PollerConfig is valid.
func (c PollerConfig) Validate() error {
	if c.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be positive: %v", c.PollInterval)
	}
	if c.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be positive: %d", c.BatchSize)
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be non-negative: %d", c.MaxRetries)
	}
	if c.LockDuration <= 0 {
		return fmt.Errorf("lock_duration must be positive: %v", c.LockDuration)
	}
	if c.LockKey == "" {
		return fmt.Errorf("lock_key is required")
	}
	return nil
}

// RetryBackoff calculates the exponential backoff duration for a given retry count.
// It uses a base delay of 1 second with exponential growth and a maximum of 5 minutes.
func RetryBackoff(retryCount int) time.Duration {
	baseDelay := 1 * time.Second
	maxDelay := 5 * time.Minute

	// Exponential backoff: baseDelay * 2^retryCount
	delay := baseDelay * time.Duration(1<<uint(retryCount))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

// PollerRetryBackoff calculates the exponential backoff for the outbox poller.
// Formula: sleep = min(2^retry_count * 100ms, 30s)
// This is used when retry_count >= 3 before the next retry attempt.
func PollerRetryBackoff(retryCount int) time.Duration {
	baseDelay := 100 * time.Millisecond
	maxDelay := 30 * time.Second

	// Exponential backoff: baseDelay * 2^retryCount
	delay := baseDelay * time.Duration(1<<uint(retryCount))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}