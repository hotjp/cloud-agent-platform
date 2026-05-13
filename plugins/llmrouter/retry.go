// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
package llmrouter

import (
	"context"
	"math/rand"
	"time"
)

// RetryConfig holds configuration for retry behavior.
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts.
	MaxAttempts int
	// InitialDelayMs is the initial delay in milliseconds.
	InitialDelayMs int64
	// MaxDelayMs is the maximum delay in milliseconds.
	MaxDelayMs int64
	// Multiplier is the exponential multiplier.
	Multiplier float64
	// JitterPercent is the jitter percentage (0-100).
	JitterPercent int
}

// RetryableError wraps an error with retry information.
type RetryableError struct {
	Err       error
	Retryable bool
}

// Error implements error.
func (e *RetryableError) Error() string {
	return e.Err.Error()
}

// Unwrap returns the wrapped error.
func (e *RetryableError) Unwrap() error {
	return e.Err
}

// IsRetryable returns whether the error is retryable.
func IsRetryable(err error) bool {
	if re, ok := err.(*RetryableError); ok {
		return re.Retryable
	}
	return false
}

// IsNonRetryable returns true for errors that should not be retried.
func IsNonRetryable(err error) bool {
	return !IsRetryable(err)
}

// RetryPolicy determines when to retry.
type RetryPolicy struct {
	config RetryConfig
}

// NewRetryPolicy creates a new retry policy.
func NewRetryPolicy(config RetryConfig) *RetryPolicy {
	return &RetryPolicy{config: config}
}

// ShouldRetry returns whether a retry should be attempted.
func (p *RetryPolicy) ShouldRetry(attempt int, err error) bool {
	if attempt >= p.config.MaxAttempts {
		return false
	}
	// Only retry on retryable errors
	return IsRetryable(err)
}

// NextDelay returns the delay before the next retry.
func (p *RetryPolicy) NextDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// Calculate exponential delay
	delay := float64(p.config.InitialDelayMs) * powF(p.config.Multiplier, float64(attempt-1))

	// Add jitter first (before cap, so jitter can push over cap)
	if p.config.JitterPercent > 0 {
		// Use the pre-cap delay as the base for jitter calculation
		jitterRange := delay * float64(p.config.JitterPercent) / 100.0
		jitter := (rand.Float64()*2 - 1) * jitterRange
		delay = delay + jitter
	}

	// Apply cap after jitter
	if delay > float64(p.config.MaxDelayMs) {
		delay = float64(p.config.MaxDelayMs)
	}

	if delay < 0 {
		delay = 0
	}
	return time.Duration(delay) * time.Millisecond
}

// powF computes base^exp as float64.
func powF(base float64, exp float64) float64 {
	result := 1.0
	for exp > 0 {
		if exp >= 1 {
			result *= base
			exp--
		}
		exp /= 2
		base *= base
	}
	return result
}

// DoWithRetry executes a function with retry logic.
func DoWithRetry(ctx context.Context, policy *RetryPolicy, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < policy.config.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		// Check if we should retry for the next attempt
		// ShouldRetry checks against attempt+1 since that's what the next attempt will be
		if !policy.ShouldRetry(attempt+1, err) {
			return err
		}

		delay := policy.NextDelay(attempt)
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return lastErr
}
