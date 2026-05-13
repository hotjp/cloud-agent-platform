// Package domain implements L2-Domain layer: domain entities, state machines,
// event collection (Outbox), and business invariants.
// This layer has ZERO external dependencies except oklog/ulid for ID generation.
package domain

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

// ----------------------------------------------------------------------------
// TaskStatus — 9 task/subtask states
// ----------------------------------------------------------------------------

// TaskStatus represents the state of a Task or Subtask.
// Corresponds to the 9 states defined in Cloud-Agent-Platform.md §三.
type TaskStatus string

const (
	TaskStatusPending     TaskStatus = "pending"
	TaskStatusDecomposing TaskStatus = "decomposing"
	TaskStatusDispatched  TaskStatus = "dispatched"
	TaskStatusRunning     TaskStatus = "running"
	TaskStatusReviewing   TaskStatus = "reviewing"
	TaskStatusConfirming  TaskStatus = "confirming"
	TaskStatusCompleted   TaskStatus = "completed"
	TaskStatusFailed      TaskStatus = "failed"
	TaskStatusCancelled   TaskStatus = "cancelled"
)

// String returns the string representation of TaskStatus.
func (s TaskStatus) String() string { return string(s) }

// IsValid reports whether s is a known TaskStatus.
func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusPending, TaskStatusDecomposing, TaskStatusDispatched,
		TaskStatusRunning, TaskStatusReviewing, TaskStatusConfirming,
		TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return true
	}
	return false
}

// IsTerminal reports whether s is a terminal (final) state.
func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusCompleted || s == TaskStatusFailed || s == TaskStatusCancelled
}

// ----------------------------------------------------------------------------
// SubtaskType — 5 subtask types
// ----------------------------------------------------------------------------

// SubtaskType represents the type of a Subtask.
// Corresponds to the 5 types defined in Cloud-Agent-Platform.md §三.
type SubtaskType string

const (
	SubtaskTypeAnalysis SubtaskType = "analysis"
	SubtaskTypeCoding   SubtaskType = "coding"
	SubtaskTypeReview   SubtaskType = "review"
	SubtaskTypeTesting  SubtaskType = "testing"
	SubtaskTypeResearch SubtaskType = "research"
)

// String returns the string representation of SubtaskType.
func (t SubtaskType) String() string { return string(t) }

// IsValid reports whether t is a known SubtaskType.
func (t SubtaskType) IsValid() bool {
	switch t {
	case SubtaskTypeAnalysis, SubtaskTypeCoding, SubtaskTypeReview,
		SubtaskTypeTesting, SubtaskTypeResearch:
		return true
	}
	return false
}

// ----------------------------------------------------------------------------
// AgentRole — 6 agent roles
// ----------------------------------------------------------------------------

// AgentRole represents the role of an Agent in the platform.
// Corresponds to the 6 roles defined in Cloud-Agent-Platform.md §三.
type AgentRole string

const (
	AgentRoleObserver    AgentRole = "observer"
	AgentRoleStrategist  AgentRole = "strategist"
	AgentRoleExecutor    AgentRole = "executor"
	AgentRoleGuardian    AgentRole = "guardian"
	AgentRoleTester      AgentRole = "tester"
	AgentRoleResearcher  AgentRole = "researcher"
)

// String returns the string representation of AgentRole.
func (r AgentRole) String() string { return string(r) }

// IsValid reports whether r is a known AgentRole.
func (r AgentRole) IsValid() bool {
	switch r {
	case AgentRoleObserver, AgentRoleStrategist, AgentRoleExecutor,
		AgentRoleGuardian, AgentRoleTester, AgentRoleResearcher:
		return true
	}
	return false
}

// ----------------------------------------------------------------------------
// ULID generation
// ----------------------------------------------------------------------------

// NewULID generates a new ULID string using crypto/rand as the entropy source.
// The returned string is 26 characters long and sorts lexicographically by
// creation time.
func NewULID() string {
	entropy := ulid.Monotonic(randEntropy{}, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// randEntropy wraps crypto/rand.Reader so it satisfies ulid.Entropy.
type randEntropy struct{}

func (randEntropy) Read(p []byte) (n int, err error) {
	return rand.Read(p)
}
