package domain

import (
	"testing"
)

func TestEntityCreation(t *testing.T) {
	entity := Entity{
		ID:      "test_123",
		Version: 1,
	}

	if entity.ID != "test_123" {
		t.Errorf("expected ID 'test_123', got '%s'", entity.ID)
	}
	if entity.Version != 1 {
		t.Errorf("expected Version 1, got %d", entity.Version)
	}
}

func TestAggregateRootRecordEvent(t *testing.T) {
	ar := &AggregateRoot{}
	ar.Entity.ID = "test_aggregate"
	ar.Entity.Version = 1

	event := DomainEvent{
		EventID:       "event_001",
		AggregateType: "Task",
		AggregateID:   "test_aggregate",
		EventType:     "TaskCreatedV1",
		Payload:       map[string]interface{}{"name": "test"},
		Version:       1,
	}

	ar.RecordEvent(event)

	events := ar.FlushEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	// FlushEvents should clear events
	events2 := ar.FlushEvents()
	if len(events2) != 0 {
		t.Errorf("expected 0 events after flush, got %d", len(events2))
	}
}
