package domain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
)

func TestDomainEvent(t *testing.T) {
	t.Run("should create DomainEvent with valid ULID", func(t *testing.T) {
		payload := []byte(`{"name":"test"}`)
		event, err := NewDomainEvent("Task", "task-123", "TaskCreatedV1", payload, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// EventID must be a valid ULID
		id, err := ulid.Parse(event.EventID)
		if err != nil {
			t.Fatalf("EventID %q is not a valid ULID: %v", event.EventID, err)
		}
		if id.String() != event.EventID {
			t.Fatalf("ULID string representation mismatch: got %q, want %q", id.String(), event.EventID)
		}

		if event.AggregateType != "Task" {
			t.Fatalf("AggregateType = %q, want %q", event.AggregateType, "Task")
		}
		if event.AggregateID != "task-123" {
			t.Fatalf("AggregateID = %q, want %q", event.AggregateID, "task-123")
		}
		if event.EventType != "TaskCreatedV1" {
			t.Fatalf("EventType = %q, want %q", event.EventType, "TaskCreatedV1")
		}
		if event.Version != 1 {
			t.Fatalf("Version = %d, want %d", event.Version, 1)
		}
	})

	t.Run("should generate correct IdempotencyKey", func(t *testing.T) {
		payload := []byte(`{}`)
		event, err := NewDomainEvent("Subtask", "sub-456", "SubtaskCompletedV2", payload, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := "sub-456:SubtaskCompletedV2:3"
		if event.IdempotencyKey != expected {
			t.Fatalf("IdempotencyKey = %q, want %q", event.IdempotencyKey, expected)
		}
	})

	t.Run("should set OccurredAt to current UTC time", func(t *testing.T) {
		payload := []byte(`{}`)
		before := time.Now().UTC().Add(-time.Second)
		event, err := NewDomainEvent("Task", "t1", "TaskStartedV1", payload, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		after := time.Now().UTC().Add(time.Second)

		if event.OccurredAt.Before(before) || event.OccurredAt.After(after) {
			t.Fatalf("OccurredAt = %v, want time between %v and %v", event.OccurredAt, before, after)
		}
	})
}

func TestValidateEventType(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid TaskCreatedV1",
			eventType: "TaskCreatedV1",
			wantErr:   false,
		},
		{
			name:      "valid SubtaskCompletedV2",
			eventType: "SubtaskCompletedV2",
			wantErr:   false,
		},
		{
			name:      "valid AgentAssignedV10",
			eventType: "AgentAssignedV10",
			wantErr:   false,
		},
		{
			name:      "valid two char type with action",
			eventType: "TaV1",
			wantErr:   false,
		},
		{
			name:      "empty string",
			eventType: "",
			wantErr:   true,
			errMsg:    "cannot be empty",
		},
		{
			name:      "missing V and version",
			eventType: "TaskCreated",
			wantErr:   true,
			errMsg:    "does not match pattern",
		},
		{
			name:      "lowercase V instead of uppercase",
			eventType: "TaskCreatedv1",
			wantErr:   true,
			errMsg:    "does not match pattern",
		},
		{
			name:      "missing version number",
			eventType: "TaskCreatedV",
			wantErr:   true,
			errMsg:    "does not match pattern",
		},
		{
			name:      "negative version",
			eventType: "TaskCreatedV-1",
			wantErr:   true,
			errMsg:    "does not match pattern",
		},
		{
			name:      "starts with number",
			eventType: "2TaskCreatedV1",
			wantErr:   true,
			errMsg:    "does not match pattern",
		},
		{
			name:      "no version suffix",
			eventType: "TaskCreated",
			wantErr:   true,
			errMsg:    "does not match pattern",
		},
		{
			name:      "only version suffix",
			eventType: "V1",
			wantErr:   true,
			errMsg:    "does not match pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEventType(tt.eventType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil for eventType %q", tt.eventType)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("error message = %q, want containing %q", err.Error(), tt.errMsg)
				}
				// Verify it's an AppError with correct code
				var appErr *AppError
				if !errors.As(err, &appErr) {
					t.Fatalf("expected *AppError, got %T", err)
				}
				if appErr.Code != CodeL2EventSerialization {
					t.Fatalf("error code = %q, want %q", appErr.Code, CodeL2EventSerialization)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateIdempotencyKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "valid key",
			key:  "task-123:TaskCreatedV1:1",
			want: true,
		},
		{
			name: "valid key with complex aggregate ID",
			key:  "01ARZ3NDEKTSV4RRFFQ69G5FAV:SubtaskCompletedV2:99",
			want: true,
		},
		{
			name: "empty key",
			key:  "",
			want: false,
		},
		{
			name: "missing version",
			key:  "task-123:TaskCreatedV1",
			want: false,
		},
		{
			name: "missing event type",
			key:  "task-123",
			want: false,
		},
		{
			name: "extra colon",
			key:  "task-123:TaskCreatedV1:1:extra",
			want: false,
		},
		{
			name: "only one colon - missing version",
			key:  "task-123:TaskCreatedV1",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdempotencyKey(tt.key)
			if tt.want {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error but got nil for key %q", tt.key)
				}
				var appErr *AppError
				if !errors.As(err, &appErr) {
					t.Fatalf("expected *AppError, got %T", err)
				}
				if appErr.Code != CodeL2EventSerialization {
					t.Fatalf("error code = %q, want %q", appErr.Code, CodeL2EventSerialization)
				}
			}
		})
	}
}

func TestDomainEventSerialize(t *testing.T) {
	t.Run("should serialize and deserialize DomainEvent correctly", func(t *testing.T) {
		payload := []byte(`{"result":"success","count":42}`)
		event, err := NewDomainEvent("Task", "task-789", "TaskCompletedV1", payload, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Serialize
		data, err := event.Serialize()
		if err != nil {
			t.Fatalf("Serialize() error = %v", err)
		}

		// Deserialize
		deserialized, err := DeserializeEvent(data)
		if err != nil {
			t.Fatalf("DeserializeEvent() error = %v", err)
		}

		if deserialized.EventID != event.EventID {
			t.Fatalf("EventID mismatch: got %q, want %q", deserialized.EventID, event.EventID)
		}
		if deserialized.AggregateType != event.AggregateType {
			t.Fatalf("AggregateType mismatch: got %q, want %q", deserialized.AggregateType, event.AggregateType)
		}
		if deserialized.AggregateID != event.AggregateID {
			t.Fatalf("AggregateID mismatch: got %q, want %q", deserialized.AggregateID, event.AggregateID)
		}
		if deserialized.EventType != event.EventType {
			t.Fatalf("EventType mismatch: got %q, want %q", deserialized.EventType, event.EventType)
		}
		if deserialized.Version != event.Version {
			t.Fatalf("Version mismatch: got %d, want %d", deserialized.Version, event.Version)
		}
		if deserialized.IdempotencyKey != event.IdempotencyKey {
			t.Fatalf("IdempotencyKey mismatch: got %q, want %q", deserialized.IdempotencyKey, event.IdempotencyKey)
		}
	})

	t.Run("should serialize payload as JSON bytes", func(t *testing.T) {
		// Create event with JSON payload
		payload := []byte(`{"key":"value","number":123}`)
		event, err := NewDomainEvent("Subtask", "s1", "SubtaskCreatedV1", payload, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify payload is preserved as-is
		if string(event.Payload) != string(payload) {
			t.Fatalf("Payload = %q, want %q", string(event.Payload), string(payload))
		}
	})

	t.Run("should deserialize payload correctly", func(t *testing.T) {
		type PayloadData struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}

		original := PayloadData{Name: "test", Value: 42}
		payloadBytes, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("failed to marshal test payload: %v", err)
		}

		event, err := NewDomainEvent("Task", "t1", "TaskCreatedV1", payloadBytes, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Serialize the event
		data, err := event.Serialize()
		if err != nil {
			t.Fatalf("Serialize() error = %v", err)
		}

		// Deserialize and check payload
		deserialized, err := DeserializeEvent(data)
		if err != nil {
			t.Fatalf("DeserializeEvent() error = %v", err)
		}

		var loaded PayloadData
		if err := json.Unmarshal(deserialized.Payload, &loaded); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}

		if loaded.Name != original.Name {
			t.Fatalf("Payload.Name = %q, want %q", loaded.Name, original.Name)
		}
		if loaded.Value != original.Value {
			t.Fatalf("Payload.Value = %d, want %d", loaded.Value, original.Value)
		}
	})
}

func TestPayloadSerializer(t *testing.T) {
	serializer := NewPayloadSerializer()

	t.Run("should serialize struct to JSON", func(t *testing.T) {
		data := struct {
			Key   string `json:"key"`
			Count int    `json:"count"`
		}{Key: "value", Count: 10}

		bytes, err := serializer.ToJSON(data)
		if err != nil {
			t.Fatalf("ToJSON() error = %v", err)
		}

		var result map[string]any
		if err := json.Unmarshal(bytes, &result); err != nil {
			t.Fatalf("result is not valid JSON: %v", err)
		}

		if result["key"] != "value" {
			t.Fatalf("key = %q, want %q", result["key"], "value")
		}
		if result["count"].(float64) != 10 {
			t.Fatalf("count = %v, want %d", result["count"], 10)
		}
	})

	t.Run("should deserialize JSON to struct", func(t *testing.T) {
		data := []byte(`{"name":"test","active":true}`)

		var result struct {
			Name   string `json:"name"`
			Active bool   `json:"active"`
		}

		err := serializer.FromJSON(data, &result)
		if err != nil {
			t.Fatalf("FromJSON() error = %v", err)
		}

		if result.Name != "test" {
			t.Fatalf("Name = %q, want %q", result.Name, "test")
		}
		if !result.Active {
			t.Fatalf("Active = false, want true")
		}
	})
}

func TestNewDomainEventWithID(t *testing.T) {
	t.Run("should create event with provided ID", func(t *testing.T) {
		eventID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		occurredAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
		payload := []byte(`{}`)

		event, err := NewDomainEventWithID(
			eventID,
			"Task",
			"task-001",
			"TaskCreatedV1",
			payload,
			occurredAt,
			1,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if event.EventID != eventID {
			t.Fatalf("EventID = %q, want %q", event.EventID, eventID)
		}
		if !event.OccurredAt.Equal(occurredAt) {
			t.Fatalf("OccurredAt = %v, want %v", event.OccurredAt, occurredAt)
		}
	})

	t.Run("should reject invalid EventType", func(t *testing.T) {
		_, err := NewDomainEventWithID(
			"01ARZ3NDEKTSV4RRFFQ69G5FAV",
			"Task",
			"task-001",
			"InvalidFormat",
			[]byte(`{}`),
			time.Now().UTC(),
			1,
		)
		if err == nil {
			t.Fatal("expected error for invalid EventType, got nil")
		}
	})
}

// Ensure OutboxWriter interface is implemented.
var _ OutboxWriter = (*mockOutboxWriter)(nil)

type mockOutboxWriter struct{}

func (m *mockOutboxWriter) Write(ctx context.Context, tx interface{}, event *DomainEvent) error {
	return nil
}

func TestOutboxWriterInterface(t *testing.T) {
	var writer OutboxWriter = &mockOutboxWriter{}
	if writer == nil {
		t.Fatal("OutboxWriter should not be nil")
	}
}

func TestDomainEventJSON(t *testing.T) {
	t.Run("DomainEvent should serialize to valid JSON", func(t *testing.T) {
		payload := []byte(`{"data":"test"}`)
		event, err := NewDomainEvent("Task", "t1", "TaskCreatedV1", payload, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		// Verify it's valid JSON
		var decoded map[string]any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		if decoded["event_id"] != event.EventID {
			t.Fatalf("event_id = %q, want %q", decoded["event_id"], event.EventID)
		}
		if decoded["aggregate_type"] != "Task" {
			t.Fatalf("aggregate_type = %q, want %q", decoded["aggregate_type"], "Task")
		}
	})
}
