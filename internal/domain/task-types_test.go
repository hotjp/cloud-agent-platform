package domain

import (
	"testing"

	"github.com/oklog/ulid/v2"
)

func TestTaskStatus_String(t *testing.T) {
	tests := []struct {
		status   TaskStatus
		expected string
	}{
		{TaskStatusPending, "pending"},
		{TaskStatusDecomposing, "decomposing"},
		{TaskStatusDispatched, "dispatched"},
		{TaskStatusRunning, "running"},
		{TaskStatusReviewing, "reviewing"},
		{TaskStatusConfirming, "confirming"},
		{TaskStatusCompleted, "completed"},
		{TaskStatusFailed, "failed"},
		{TaskStatusCancelled, "cancelled"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.expected {
			t.Errorf("TaskStatus(%s).String() = %q, want %q", tt.status, got, tt.expected)
		}
	}
}

func TestTaskStatus_IsValid(t *testing.T) {
	valid := []TaskStatus{
		TaskStatusPending, TaskStatusDecomposing, TaskStatusDispatched,
		TaskStatusRunning, TaskStatusReviewing, TaskStatusConfirming,
		TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled,
	}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("TaskStatus(%s).IsValid() = false, want true", s)
		}
	}

	invalid := []TaskStatus{"", "unknown", "PENDING", "Completed"}
	for _, s := range invalid {
		if TaskStatus(s).IsValid() {
			t.Errorf("TaskStatus(%q).IsValid() = true, want false", s)
		}
	}
}

func TestTaskStatus_IsTerminal(t *testing.T) {
	terminals := []TaskStatus{TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled}
	nonTerminals := []TaskStatus{
		TaskStatusPending, TaskStatusDecomposing, TaskStatusDispatched,
		TaskStatusRunning, TaskStatusReviewing, TaskStatusConfirming,
	}
	for _, s := range terminals {
		if !s.IsTerminal() {
			t.Errorf("TaskStatus(%s).IsTerminal() = false, want true", s)
		}
	}
	for _, s := range nonTerminals {
		if s.IsTerminal() {
			t.Errorf("TaskStatus(%s).IsTerminal() = true, want false", s)
		}
	}
}

func TestSubtaskType_String(t *testing.T) {
	tests := []struct {
		typ     SubtaskType
		expected string
	}{
		{SubtaskTypeAnalysis, "analysis"},
		{SubtaskTypeCoding, "coding"},
		{SubtaskTypeReview, "review"},
		{SubtaskTypeTesting, "testing"},
		{SubtaskTypeResearch, "research"},
	}
	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.expected {
			t.Errorf("SubtaskType(%s).String() = %q, want %q", tt.typ, got, tt.expected)
		}
	}
}

func TestSubtaskType_IsValid(t *testing.T) {
	valid := []SubtaskType{
		SubtaskTypeAnalysis, SubtaskTypeCoding,
		SubtaskTypeReview, SubtaskTypeTesting, SubtaskTypeResearch,
	}
	for _, st := range valid {
		if !st.IsValid() {
			t.Errorf("SubtaskType(%s).IsValid() = false, want true", st)
		}
	}

	invalid := []SubtaskType{"", "code", "analyze", "ANALYSIS", "unknown"}
	for _, s := range invalid {
		if SubtaskType(s).IsValid() {
			t.Errorf("SubtaskType(%q).IsValid() = true, want false", s)
		}
	}
}

func TestAgentRole_String(t *testing.T) {
	tests := []struct {
		role     AgentRole
		expected string
	}{
		{AgentRoleObserver, "observer"},
		{AgentRoleStrategist, "strategist"},
		{AgentRoleExecutor, "executor"},
		{AgentRoleGuardian, "guardian"},
		{AgentRoleTester, "tester"},
		{AgentRoleResearcher, "researcher"},
	}
	for _, tt := range tests {
		if got := tt.role.String(); got != tt.expected {
			t.Errorf("AgentRole(%s).String() = %q, want %q", tt.role, got, tt.expected)
		}
	}
}

func TestAgentRole_IsValid(t *testing.T) {
	valid := []AgentRole{
		AgentRoleObserver, AgentRoleStrategist, AgentRoleExecutor,
		AgentRoleGuardian, AgentRoleTester, AgentRoleResearcher,
	}
	for _, r := range valid {
		if !r.IsValid() {
			t.Errorf("AgentRole(%s).IsValid() = false, want true", r)
		}
	}

	invalid := []AgentRole{"", "admin", "ADMIN", "executor ", "Observer", "unknown"}
	for _, s := range invalid {
		if AgentRole(s).IsValid() {
			t.Errorf("AgentRole(%q).IsValid() = true, want false", s)
		}
	}
}

func TestNewULID(t *testing.T) {
	id := NewULID()
	if len(id) != 26 {
		t.Errorf("NewULID() len = %d, want 26", len(id))
	}

	// Must be a valid ULID
	_, err := ulid.Parse(id)
	if err != nil {
		t.Errorf("NewULID() produced invalid ULID %q: %v", id, err)
	}

	// Two calls should produce different IDs (different timestamps)
	id2 := NewULID()
	if id == id2 {
		t.Errorf("NewULID() returned same ID twice: %q", id)
	}
}

func TestNewULID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewULID()
		if ids[id] {
			t.Errorf("NewULID() generated duplicate ID: %q", id)
		}
		ids[id] = true
	}
}
