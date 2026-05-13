package outbox

import (
	"context"
	"sync/atomic"

	"github.com/cloud-agent-platform/cap/ent"
)

// MockOutboxEventForwarder is a mock implementation of OutboxEventForwarder
// for testing purposes.
type MockOutboxEventForwarder struct {
	// ForwardedEvents stores all events that were forwarded
	ForwardedEvents []*ent.OutboxEvent

	// ForwardFunc is an optional custom forward function
	ForwardFunc func(ctx context.Context, event *ent.OutboxEvent) error

	// ShouldFail indicates whether Forward should return an error
	ShouldFail atomic.Bool

	// FailError is the error to return when ShouldFail is true
	FailError atomic.Value // type: error

	// CallCount tracks how many times Forward was called
	CallCount atomic.Int64
}

// NewMockOutboxEventForwarder creates a new MockOutboxEventForwarder.
func NewMockOutboxEventForwarder() *MockOutboxEventForwarder {
	return &MockOutboxEventForwarder{
		ForwardedEvents: make([]*ent.OutboxEvent, 0),
	}
}

// Forward implements OutboxEventForwarder.
func (m *MockOutboxEventForwarder) Forward(ctx context.Context, event *ent.OutboxEvent) error {
	m.CallCount.Add(1)

	if m.ForwardFunc != nil {
		return m.ForwardFunc(ctx, event)
	}

	if m.ShouldFail.Load() {
		err, _ := m.FailError.Load().(error)
		if err == nil {
			err = &MockForwardError{Message: "mock forward error"}
		}
		return err
	}

	m.ForwardedEvents = append(m.ForwardedEvents, event)
	return nil
}

// MockForwardError is a simple error type for mocking.
type MockForwardError struct {
	Message string
}

func (e *MockForwardError) Error() string {
	return e.Message
}