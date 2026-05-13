// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
package llmrouter

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitStateClosed CircuitState = iota
	CircuitStateOpen
	CircuitStateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitStateClosed:
		return "closed"
	case CircuitStateOpen:
		return "open"
	case CircuitStateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for the circuit breaker.
type CircuitBreakerConfig struct {
	// ErrorRateThreshold is the error rate threshold (0-1) to open the circuit.
	ErrorRateThreshold float64
	// MinRequests is the minimum number of requests before calculating error rate.
	MinRequests int
	// HalfOpenMaxRequests is the maximum requests allowed in half-open state.
	HalfOpenMaxRequests int
	// OpenTimeoutMs is the time in ms before attempting to close the circuit.
	OpenTimeoutMs int64
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	config     CircuitBreakerConfig
	mu         sync.RWMutex
	state      CircuitState
	totalReqs  int64
	failedReqs int64
	lastStateChange time.Time
	halfOpenReqs int64
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config:          config,
		state:           CircuitStateClosed,
		lastStateChange: time.Now(),
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Allow checks if a request is allowed through the circuit breaker.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitStateClosed:
		return true

	case CircuitStateOpen:
		// Check if timeout has passed
		timeout := time.Duration(cb.config.OpenTimeoutMs) * time.Millisecond
		if time.Since(cb.lastStateChange) >= timeout {
			cb.toHalfOpenLocked()
			return true
		}
		return false

	case CircuitStateHalfOpen:
		// Allow limited requests in half-open state
		if cb.halfOpenReqs < int64(cb.config.HalfOpenMaxRequests) {
			cb.halfOpenReqs++
			return true
		}
		return false

	default:
		return false
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalReqs++
	cb.updateStateLocked()
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalReqs++
	cb.failedReqs++
	cb.updateStateLocked()
}

// ErrorRate returns the current error rate.
func (cb *CircuitBreaker) ErrorRate() float64 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.totalReqs == 0 {
		return 0
	}
	return float64(cb.failedReqs) / float64(cb.totalReqs)
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalReqs = 0
	cb.failedReqs = 0
	cb.halfOpenReqs = 0
	cb.state = CircuitStateClosed
	cb.lastStateChange = time.Now()
}

// updateStateLocked updates the circuit state.
// Caller must hold the lock.
func (cb *CircuitBreaker) updateStateLocked() {
	switch cb.state {
	case CircuitStateClosed:
		// Check if we should open
		if cb.totalReqs >= int64(cb.config.MinRequests) {
			errorRate := float64(cb.failedReqs) / float64(cb.totalReqs)
			if errorRate >= cb.config.ErrorRateThreshold {
				cb.toOpenLocked()
			}
		}

	case CircuitStateOpen:
		// Will transition to half-open after timeout

	case CircuitStateHalfOpen:
		// If we have failures in half-open, go back to open
		if cb.failedReqs > 0 {
			cb.toOpenLocked()
		} else if cb.halfOpenReqs >= int64(cb.config.HalfOpenMaxRequests) {
			// All requests succeeded, close the circuit
			cb.toClosedLocked()
		}
	}
}

// toOpenLocked transitions to open state.
// Caller must hold the lock.
func (cb *CircuitBreaker) toOpenLocked() {
	if cb.state != CircuitStateOpen {
		cb.state = CircuitStateOpen
		cb.lastStateChange = time.Now()
		cb.halfOpenReqs = 0
	}
}

// toHalfOpenLocked transitions to half-open state.
// Caller must hold the lock.
func (cb *CircuitBreaker) toHalfOpenLocked() {
	cb.state = CircuitStateHalfOpen
	cb.lastStateChange = time.Now()
	cb.halfOpenReqs = 0
	cb.failedReqs = 0
	cb.totalReqs = 0
}

// toClosedLocked transitions to closed state.
// Caller must hold the lock.
func (cb *CircuitBreaker) toClosedLocked() {
	cb.state = CircuitStateClosed
	cb.lastStateChange = time.Now()
	cb.halfOpenReqs = 0
	cb.failedReqs = 0
	cb.totalReqs = 0
}

// SetStateForTest sets the circuit state directly (for testing only).
// This method is NOT thread-safe for production use.
func (cb *CircuitBreaker) SetStateForTest(state CircuitState) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = state
}
