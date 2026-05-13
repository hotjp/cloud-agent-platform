// Package domain implements L2-Domain layer: domain entities, state machines,
// event collection (Outbox), and business invariants.
// This layer has ZERO external dependencies - pure Go structs + standard library.
package domain

// Entity represents a domain entity with ULID-based ID.
type Entity struct {
	ID      string
	Version int64
}

// AggregateRoot is the base for all aggregate roots.
// It collects domain events that will be published via the Outbox pattern.
type AggregateRoot struct {
	Entity
	events []*DomainEvent
}

// RecordEvent records a domain event for later publishing via Outbox.
func (a *AggregateRoot) RecordEvent(event *DomainEvent) {
	a.events = append(a.events, event)
}

// FlushEvents returns and clears recorded events.
func (a *AggregateRoot) FlushEvents() []*DomainEvent {
	events := a.events
	a.events = nil
	return events
}
