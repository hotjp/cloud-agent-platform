// Package domain implements L2-Domain layer: domain entities, state machines,
// event collection (Outbox), and business invariants.
// This layer has ZERO external dependencies except oklog/ulid for ID generation.
package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// ----------------------------------------------------------------------------
// DomainEvent
// ----------------------------------------------------------------------------

// DomainEvent represents a domain event for the Outbox pattern.
// Each event is associated with an aggregate root and carries the data necessary
// to rebuild the aggregate state or notify interested parties.
type DomainEvent struct {
	EventID        string    `json:"event_id"`        // ULID, globally unique
	AggregateType  string    `json:"aggregate_type"`  // aggregate type, e.g. "Task", "Subtask"
	AggregateID    string    `json:"aggregate_id"`    // aggregate root ID
	EventType      string    `json:"event_type"`     // format: {AggregateType}{Action}V{Version}, e.g. TaskCreatedV1
	Payload        []byte    `json:"payload"`        // JSON-serialized event data
	OccurredAt     time.Time `json:"occurred_at"`    // UTC time when the event occurred
	IdempotencyKey string    `json:"idempotency_key"` // format: {aggregate_id}:{event_type}:{version}
	Version        int       `json:"version"`        // aggregate root version (optimistic lock)
}

// eventTypePattern validates EventType format: {AggregateType}{Action}V{Version}
// where AggregateType and Action are non-empty alphanumeric strings, and Version
// is a positive integer.
var eventTypePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*[a-zA-Z0-9]V(\d+)$`)

// ValidateEventType checks that the EventType matches the required format.
// Returns nil if valid, otherwise returns an error with code L2EventSerialization.
func ValidateEventType(eventType string) error {
	if eventType == "" {
		return &AppError{
			Code:    CodeL2EventSerialization,
			Message: "event_type cannot be empty",
			Layer:   LayerDomain,
		}
	}
	if !eventTypePattern.MatchString(eventType) {
		return &AppError{
			Code:    CodeL2EventSerialization,
			Message: fmt.Sprintf("event_type %q does not match pattern {AggregateType}{Action}V{Version}, e.g. TaskCreatedV1", eventType),
			Layer:   LayerDomain,
			Details: map[string]any{"event_type": eventType},
		}
	}
	return nil
}

// ValidateIdempotencyKey checks that the IdempotencyKey matches the required format.
// Format: {aggregate_id}:{event_type}:{version}
// Returns nil if valid, otherwise returns an error with code L2EventSerialization.
func ValidateIdempotencyKey(key string) error {
	if key == "" {
		return &AppError{
			Code:    CodeL2EventSerialization,
			Message: "idempotency_key cannot be empty",
			Layer:   LayerDomain,
		}
	}
	// Format: {aggregate_id}:{event_type}:{version}
	parts := splitIdempotencyKey(key)
	if len(parts) != 3 {
		return &AppError{
			Code:    CodeL2EventSerialization,
			Message: fmt.Sprintf("idempotency_key %q does not match format {aggregate_id}:{event_type}:{version}", key),
			Layer:   LayerDomain,
			Details: map[string]any{"idempotency_key": key},
		}
	}
	return nil
}

var idempotencyKeySep = regexp.MustCompile(`:`)

func splitIdempotencyKey(key string) []string {
	return idempotencyKeySep.Split(key, -1)
}

// NewDomainEvent creates a new DomainEvent with a generated ULID and current timestamp.
// It validates the EventType and IdempotencyKey formats.
// Returns an error with code L2EventSerialization if validation fails.
func NewDomainEvent(aggregateType, aggregateID, eventType string, payload []byte, version int) (*DomainEvent, error) {
	if err := ValidateEventType(eventType); err != nil {
		return nil, err
	}

	occurredAt := time.Now().UTC()
	idempotencyKey := fmt.Sprintf("%s:%s:%d", aggregateID, eventType, version)

	if err := ValidateIdempotencyKey(idempotencyKey); err != nil {
		return nil, err
	}

	return &DomainEvent{
		EventID:        NewULID(),
		AggregateType:  aggregateType,
		AggregateID:    aggregateID,
		EventType:      eventType,
		Payload:        payload,
		OccurredAt:     occurredAt,
		IdempotencyKey: idempotencyKey,
		Version:        version,
	}, nil
}

// NewDomainEventWithID creates a DomainEvent with a provided eventID.
// Use this for testing or when the event ID is already known.
func NewDomainEventWithID(eventID, aggregateType, aggregateID, eventType string, payload []byte, occurredAt time.Time, version int) (*DomainEvent, error) {
	if err := ValidateEventType(eventType); err != nil {
		return nil, err
	}

	idempotencyKey := fmt.Sprintf("%s:%s:%d", aggregateID, eventType, version)
	if err := ValidateIdempotencyKey(idempotencyKey); err != nil {
		return nil, err
	}

	return &DomainEvent{
		EventID:        eventID,
		AggregateType:  aggregateType,
		AggregateID:    aggregateID,
		EventType:      eventType,
		Payload:        payload,
		OccurredAt:     occurredAt,
		IdempotencyKey: idempotencyKey,
		Version:        version,
	}, nil
}

// ----------------------------------------------------------------------------
// OutboxWriter
// ----------------------------------------------------------------------------

// OutboxWriter is the interface for writing domain events to the Outbox table
// within the same transaction as the aggregate root update.
// Implemented by L1-Storage (ent).
type OutboxWriter interface {
	// Write inserts the event into the Outbox table within the given transaction.
	// The event will be published to the message broker by the Outbox relay process.
	Write(ctx context.Context, tx interface{}, event *DomainEvent) error
}

// Verify OutboxWriter is implemented correctly at compile time.
var _ OutboxWriter = (*OutboxWriterFunc)(nil)

// OutboxWriterFunc is a function type that satisfies OutboxWriter.
// It allows plain functions to be used as OutboxWriter without defining a struct.
type OutboxWriterFunc func(ctx context.Context, tx interface{}, event *DomainEvent) error

// Write implements OutboxWriter by delegating to the function.
func (f OutboxWriterFunc) Write(ctx context.Context, tx interface{}, event *DomainEvent) error {
	return f(ctx, tx, event)
}

// ----------------------------------------------------------------------------
// EventSerializer
// ----------------------------------------------------------------------------

// EventSerializer defines the interface for serializing domain events to and
// from JSON. Implemented by domain events to support JSON marshaling.
type EventSerializer interface {
	// Serialize returns the JSON representation of the event payload.
	Serialize() ([]byte, error)

	// Deserialize populates the event from its JSON representation.
	Deserialize(data []byte) error
}

// Serialize returns the JSON representation of the DomainEvent.
// Returns an error with code L2EventSerialization if marshaling fails.
func (e *DomainEvent) Serialize() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, &AppError{
			Code:    CodeL2EventSerialization,
			Message: fmt.Sprintf("failed to serialize domain event: %s", e.EventType),
			Layer:   LayerDomain,
			Details: map[string]any{"event_type": e.EventType, "event_id": e.EventID},
			Cause:   err,
		}
	}
	return data, nil
}

// Deserialize populates the DomainEvent from its JSON representation.
// Implements EventSerializer.
func (e *DomainEvent) Deserialize(data []byte) error {
	if err := json.Unmarshal(data, e); err != nil {
		return &AppError{
			Code:    CodeL2EventSerialization,
			Message: "failed to deserialize domain event",
			Layer:   LayerDomain,
			Cause:   err,
		}
	}
	return nil
}

// DeserializeEvent deserializes a DomainEvent from JSON data.
// Returns an error with code L2EventSerialization if unmarshaling fails.
func DeserializeEvent(data []byte) (*DomainEvent, error) {
	var event DomainEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, &AppError{
			Code:    CodeL2EventSerialization,
			Message: "failed to deserialize domain event",
			Layer:   LayerDomain,
			Cause:   err,
		}
	}
	return &event, nil
}

// PayloadSerializer provides utilities for serializing event payloads.
type PayloadSerializer struct{}

// NewPayloadSerializer creates a new PayloadSerializer.
func NewPayloadSerializer() *PayloadSerializer {
	return &PayloadSerializer{}
}

// Serialize serializes a value to JSON bytes.
// Implements EventSerializer.
func (s *PayloadSerializer) Serialize(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, &AppError{
			Code:    CodeL2EventSerialization,
			Message: fmt.Sprintf("failed to serialize payload: %v", v),
			Layer:   LayerDomain,
			Cause:   err,
		}
	}
	return data, nil
}

// Deserialize deserializes JSON bytes into a value.
// Implements EventSerializer.
func (s *PayloadSerializer) Deserialize(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return &AppError{
			Code:    CodeL2EventSerialization,
			Message: "failed to deserialize payload",
			Layer:   LayerDomain,
			Cause:   err,
		}
	}
	return nil
}

// ToJSON is an alias for Serialize for backward compatibility.
func (s *PayloadSerializer) ToJSON(v any) ([]byte, error) {
	return s.Serialize(v)
}

// FromJSON is an alias for Deserialize for backward compatibility.
func (s *PayloadSerializer) FromJSON(data []byte, v any) error {
	return s.Deserialize(data, v)
}

// Verify EventSerializer interface is satisfied.
var _ EventSerializer = (*DomainEvent)(nil)

// ----------------------------------------------------------------------------
// Errors
// ----------------------------------------------------------------------------

// ErrEventTypeInvalid is returned when EventType format validation fails.
var ErrEventTypeInvalid = errors.New("invalid event type format")

// ErrIdempotencyKeyInvalid is returned when IdempotencyKey format validation fails.
var ErrIdempotencyKeyInvalid = errors.New("invalid idempotency key format")
