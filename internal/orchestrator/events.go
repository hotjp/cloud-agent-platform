// Package orchestrator implements L4 orchestration: task scheduling, agent session
// management, and event-driven workflow coordination.
package orchestrator

import (
	"encoding/json"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
)

// ----------------------------------------------------------------------------
// Orchestration Event Types
// ----------------------------------------------------------------------------

// Event types for orchestration (format: {Aggregate}{Action}V{Version})
const (
	// Task orchestration events
	EventTypeTaskAssigned      = "TaskAssignedV1"
	EventTypeTaskStarted       = "TaskStartedV1"
	EventTypeTaskCompleted     = "TaskCompletedV1"
	EventTypeTaskFailed       = "TaskFailedV1"
	EventTypeAgentSessionStarted  = "AgentSessionStartedV1"
	EventTypeAgentSessionEnded    = "AgentSessionEndedV1"
)

// ----------------------------------------------------------------------------
// Event Payloads
// ----------------------------------------------------------------------------

// TaskAssignedPayload is the payload for TaskAssignedV1 event.
type TaskAssignedPayload struct {
	TaskID         string    `json:"task_id"`
	SubtaskID      string    `json:"subtask_id,omitempty"`
	AgentTemplate  string    `json:"agent_template,omitempty"`
	SessionID      string    `json:"session_id"`
	AssignedAt     time.Time `json:"assigned_at"`
}

// TaskStartedPayload is the payload for TaskStartedV1 event.
type TaskStartedPayload struct {
	TaskID       string    `json:"task_id"`
	SubtaskID    string    `json:"subtask_id,omitempty"`
	SessionID    string    `json:"session_id"`
	StartedAt    time.Time `json:"started_at"`
}

// TaskCompletedPayload is the payload for TaskCompletedV1 event.
type TaskCompletedPayload struct {
	TaskID           string                 `json:"task_id"`
	SubtaskID        string                 `json:"subtask_id,omitempty"`
	SessionID        string                 `json:"session_id"`
	Summary          string                 `json:"summary"`
	Artifacts        []domain.ArtifactRef   `json:"artifacts,omitempty"`
	TokensUsed       int                    `json:"tokens_used"`
	ExecutionSeconds float64                `json:"execution_seconds"`
	CompletedAt      time.Time              `json:"completed_at"`
}

// TaskFailedPayload is the payload for TaskFailedV1 event.
type TaskFailedPayload struct {
	TaskID       string    `json:"task_id"`
	SubtaskID    string    `json:"subtask_id,omitempty"`
	SessionID    string    `json:"session_id"`
	Reason       string    `json:"reason"`
	ErrorCode    string    `json:"error_code,omitempty"`
	FailedAt     time.Time `json:"failed_at"`
}

// AgentSessionStartedPayload is the payload for AgentSessionStartedV1 event.
type AgentSessionStartedPayload struct {
	SessionID     string    `json:"session_id"`
	TaskID        string    `json:"task_id"`
	SubtaskID     string    `json:"subtask_id,omitempty"`
	AgentTemplate string    `json:"agent_template,omitempty"`
	AgentRunner   string    `json:"agent_runner"`
	StartedAt     time.Time `json:"started_at"`
}

// AgentSessionEndedPayload is the payload for AgentSessionEndedV1 event.
type AgentSessionEndedPayload struct {
	SessionID     string    `json:"session_id"`
	TaskID        string    `json:"task_id"`
	SubtaskID     string    `json:"subtask_id,omitempty"`
	Status        string    `json:"status"` // "completed", "failed", "cancelled"
	Summary       string    `json:"summary,omitempty"`
	ErrorReason   string    `json:"error_reason,omitempty"`
	TokensUsed    int       `json:"tokens_used"`
	DurationMs    int64     `json:"duration_ms"`
	EndedAt       time.Time `json:"ended_at"`
}

// ----------------------------------------------------------------------------
// Event Payload Serialization Helpers
// ----------------------------------------------------------------------------

// MarshalTaskAssignedPayload serializes a TaskAssignedPayload to JSON bytes.
func MarshalTaskAssignedPayload(payload *TaskAssignedPayload) ([]byte, error) {
	return json.Marshal(payload)
}

// MarshalTaskStartedPayload serializes a TaskStartedPayload to JSON bytes.
func MarshalTaskStartedPayload(payload *TaskStartedPayload) ([]byte, error) {
	return json.Marshal(payload)
}

// MarshalTaskCompletedPayload serializes a TaskCompletedPayload to JSON bytes.
func MarshalTaskCompletedPayload(payload *TaskCompletedPayload) ([]byte, error) {
	return json.Marshal(payload)
}

// MarshalTaskFailedPayload serializes a TaskFailedPayload to JSON bytes.
func MarshalTaskFailedPayload(payload *TaskFailedPayload) ([]byte, error) {
	return json.Marshal(payload)
}

// MarshalAgentSessionStartedPayload serializes an AgentSessionStartedPayload to JSON bytes.
func MarshalAgentSessionStartedPayload(payload *AgentSessionStartedPayload) ([]byte, error) {
	return json.Marshal(payload)
}

// MarshalAgentSessionEndedPayload serializes an AgentSessionEndedPayload to JSON bytes.
func MarshalAgentSessionEndedPayload(payload *AgentSessionEndedPayload) ([]byte, error) {
	return json.Marshal(payload)
}
