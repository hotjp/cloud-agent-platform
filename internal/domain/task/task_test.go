package task

import (
	"testing"
)

// ----------------------------------------------------------------------------
// TaskStatus Tests
// ----------------------------------------------------------------------------

func TestTaskStatusIsValid(t *testing.T) {
	tests := []struct {
		status TaskStatus
		valid  bool
	}{
		{TaskStatusPending, true},
		{TaskStatusSubmitted, true},
		{TaskStatusDecomposing, true},
		{TaskStatusAssigned, true},
		{TaskStatusRunning, true},
		{TaskStatusReviewing, true},
		{TaskStatusCompleted, true},
		{TaskStatusFailed, true},
		{TaskStatusCancelled, true},
		{TaskStatus("invalid"), false},
		{TaskStatus(""), false},
	}

	for _, tt := range tests {
		if got := tt.status.IsValid(); got != tt.valid {
			t.Errorf("TaskStatus(%q).IsValid() = %v, want %v", tt.status, got, tt.valid)
		}
	}
}

func TestTaskStatusIsTerminal(t *testing.T) {
	tests := []struct {
		status   TaskStatus
		terminal bool
	}{
		{TaskStatusPending, false},
		{TaskStatusSubmitted, false},
		{TaskStatusDecomposing, false},
		{TaskStatusAssigned, false},
		{TaskStatusRunning, false},
		{TaskStatusReviewing, false},
		{TaskStatusCompleted, true},
		{TaskStatusFailed, true},
		{TaskStatusCancelled, true},
	}

	for _, tt := range tests {
		if got := tt.status.IsTerminal(); got != tt.terminal {
			t.Errorf("TaskStatus(%q).IsTerminal() = %v, want %v", tt.status, got, tt.terminal)
		}
	}
}

// ----------------------------------------------------------------------------
// StateMachine Tests
// ----------------------------------------------------------------------------

func TestNewStateMachine(t *testing.T) {
	sm := NewStateMachine("Task", "task-123", TaskStatusPending)

	if sm.EntityType != "Task" {
		t.Errorf("EntityType = %q, want %q", sm.EntityType, "Task")
	}
	if sm.EntityID != "task-123" {
		t.Errorf("EntityID = %q, want %q", sm.EntityID, "task-123")
	}
	if sm.State != TaskStatusPending {
		t.Errorf("State = %q, want %q", sm.State, TaskStatusPending)
	}
	if sm.Version != 1 {
		t.Errorf("Version = %d, want %d", sm.Version, 1)
	}
}

func TestStateMachineTrigger(t *testing.T) {
	sm := BuildTaskStateMachine("task-123")

	// pending -> submitted (Submit)
	newState, err := sm.Trigger("Submit")
	if err != nil {
		t.Fatalf("Trigger(Submit) error = %v", err)
	}
	if newState != TaskStatusSubmitted {
		t.Errorf("newState = %q, want %q", newState, TaskStatusSubmitted)
	}
	if sm.State != TaskStatusSubmitted {
		t.Errorf("sm.State = %q, want %q", sm.State, TaskStatusSubmitted)
	}
	if sm.Version != 2 {
		t.Errorf("Version = %d, want %d", sm.Version, 2)
	}
}

func TestStateMachineTriggerInvalidTransition(t *testing.T) {
	sm := BuildTaskStateMachine("task-123")

	// Try an invalid transition (StartExecution from pending is not valid)
	_, err := sm.Trigger("StartExecution")
	if err == nil {
		t.Fatal("expected error for invalid transition, got nil")
	}

	// State should not change
	if sm.State != TaskStatusPending {
		t.Errorf("State = %q, want %q", sm.State, TaskStatusPending)
	}
}

func TestStateMachineCanTransition(t *testing.T) {
	sm := BuildTaskStateMachine("task-123")

	if !sm.CanTransition("Submit") {
		t.Error("Should be able to transition with Submit from pending")
	}
	if sm.CanTransition("StartExecution") {
		t.Error("Should NOT be able to transition with StartExecution from pending")
	}
}

func TestStateMachineAvailableEvents(t *testing.T) {
	sm := BuildTaskStateMachine("task-123")

	events := sm.AvailableEvents()
	if len(events) == 0 {
		t.Error("Should have available events from pending state")
	}

	found := false
	for _, e := range events {
		if e == "Submit" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Submit should be in available events from pending")
	}
}

func TestStateMachineReset(t *testing.T) {
	sm := BuildTaskStateMachine("task-123")
	sm.State = TaskStatusRunning
	sm.Version = 5

	sm.Reset()

	if sm.State != TaskStatusPending {
		t.Errorf("State = %q, want %q", sm.State, TaskStatusPending)
	}
	if sm.Version != 1 {
		t.Errorf("Version = %d, want %d", sm.Version, 1)
	}
}

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		state    TaskStatus
		terminal bool
	}{
		{TaskStatusPending, false},
		{TaskStatusSubmitted, false},
		{TaskStatusDecomposing, false},
		{TaskStatusAssigned, false},
		{TaskStatusRunning, false},
		{TaskStatusReviewing, false},
		{TaskStatusCompleted, true},
		{TaskStatusFailed, true},
		{TaskStatusCancelled, true},
	}

	for _, tt := range tests {
		if IsTerminalState(tt.state) != tt.terminal {
			t.Errorf("IsTerminalState(%q) = %v, want %v", tt.state, !tt.terminal, tt.terminal)
		}
	}
}

// ----------------------------------------------------------------------------
// Task Entity Tests
// ----------------------------------------------------------------------------

func TestNewTask(t *testing.T) {
	task := NewTask("task-123", "Implement login", "https://github.com/example/repo", "main", "client-abc")

	if task.ID != "task-123" {
		t.Errorf("ID = %q, want %q", task.ID, "task-123")
	}
	if task.Status != TaskStatusPending {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusPending)
	}
	if task.Version != 1 {
		t.Errorf("Version = %d, want %d", task.Version, 1)
	}
	if task.Goal != "Implement login" {
		t.Errorf("Goal = %q, want %q", task.Goal, "Implement login")
	}
	if task.Priority != 5 {
		t.Errorf("Priority = %d, want %d", task.Priority, 5)
	}
	if task.ResultBranch != "main/agent/task-123" {
		t.Errorf("ResultBranch = %q, want %q", task.ResultBranch, "main/agent/task-123")
	}
}

func TestTaskSubmit(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")

	// Valid transition: pending -> submitted
	err := task.Submit()
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if task.Status != TaskStatusSubmitted {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusSubmitted)
	}

	// Check domain event was recorded
	events := task.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "TaskSubmittedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "TaskSubmittedV1")
	}
}

func TestTaskSubmitFromNonPendingFails(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusSubmitted

	err := task.Submit()
	if err == nil {
		t.Fatal("expected error when submitting non-pending task, got nil")
	}
}

func TestTaskStartDecomposition(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")

	// Move to submitted first
	if err := task.Submit(); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	// Flush events from Submit
	task.FlushEvents()

	// Valid transition: submitted -> decomposing
	err := task.StartDecomposition()
	if err != nil {
		t.Fatalf("StartDecomposition() error = %v", err)
	}
	if task.Status != TaskStatusDecomposing {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusDecomposing)
	}

	events := task.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "TaskDecompositionStartedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "TaskDecompositionStartedV1")
	}
}

func TestTaskStartDecompositionFromNonSubmittedFails(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	// task is still pending

	err := task.StartDecomposition()
	if err == nil {
		t.Fatal("expected error when starting decomposition from non-submitted state, got nil")
	}
}

func TestTaskCompleteDecomposition(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")

	// pending -> submitted -> decomposing
	if err := task.Submit(); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	task.FlushEvents()
	if err := task.StartDecomposition(); err != nil {
		t.Fatalf("StartDecomposition() error = %v", err)
	}
	task.FlushEvents()

	// Valid transition: decomposing -> assigned
	err := task.CompleteDecomposition()
	if err != nil {
		t.Fatalf("CompleteDecomposition() error = %v", err)
	}
	if task.Status != TaskStatusAssigned {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusAssigned)
	}

	events := task.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "TaskDecompositionCompletedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "TaskDecompositionCompletedV1")
	}
}

func TestTaskStartExecution(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")

	// pending -> submitted -> decomposing -> assigned
	if err := task.Submit(); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	task.FlushEvents()
	if err := task.StartDecomposition(); err != nil {
		t.Fatalf("StartDecomposition() error = %v", err)
	}
	task.FlushEvents()
	if err := task.CompleteDecomposition(); err != nil {
		t.Fatalf("CompleteDecomposition() error = %v", err)
	}
	task.FlushEvents()

	// Valid transition: assigned -> running
	err := task.StartExecution()
	if err != nil {
		t.Fatalf("StartExecution() error = %v", err)
	}
	if task.Status != TaskStatusRunning {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusRunning)
	}
	if task.StartedAt == nil {
		t.Error("StartedAt should be set")
	}

	events := task.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "TaskExecutionStartedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "TaskExecutionStartedV1")
	}
}

func TestTaskStartExecutionFromNonAssignedFails(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusDecomposing

	err := task.StartExecution()
	if err == nil {
		t.Fatal("expected error when starting execution from non-assigned state, got nil")
	}
}

func TestTaskCompleteExecution(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")

	// Move to running
	if err := task.Submit(); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	task.FlushEvents()
	if err := task.StartDecomposition(); err != nil {
		t.Fatalf("StartDecomposition() error = %v", err)
	}
	task.FlushEvents()
	if err := task.CompleteDecomposition(); err != nil {
		t.Fatalf("CompleteDecomposition() error = %v", err)
	}
	task.FlushEvents()
	if err := task.StartExecution(); err != nil {
		t.Fatalf("StartExecution() error = %v", err)
	}
	task.FlushEvents()

	// Valid transition: running -> reviewing
	err := task.CompleteExecution()
	if err != nil {
		t.Fatalf("CompleteExecution() error = %v", err)
	}
	if task.Status != TaskStatusReviewing {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusReviewing)
	}

	events := task.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "TaskExecutionCompletedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "TaskExecutionCompletedV1")
	}
}

func TestTaskReviewPassed(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")

	// Move to reviewing
	task.Status = TaskStatusReviewing

	// Valid transition: reviewing -> completed
	err := task.ReviewPassed()
	if err != nil {
		t.Fatalf("ReviewPassed() error = %v", err)
	}
	if task.Status != TaskStatusCompleted {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusCompleted)
	}
	if task.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	events := task.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "TaskReviewPassedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "TaskReviewPassedV1")
	}
}

func TestTaskReviewFailedRetry(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusReviewing

	// Valid transition: reviewing -> running
	err := task.ReviewFailedRetry()
	if err != nil {
		t.Fatalf("ReviewFailedRetry() error = %v", err)
	}
	if task.Status != TaskStatusRunning {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusRunning)
	}

	events := task.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "TaskReviewFailedRetryV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "TaskReviewFailedRetryV1")
	}
}

func TestTaskReviewFailedFatal(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusReviewing

	// Valid transition: reviewing -> failed
	err := task.ReviewFailedFatal()
	if err != nil {
		t.Fatalf("ReviewFailedFatal() error = %v", err)
	}
	if task.Status != TaskStatusFailed {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusFailed)
	}
	if task.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	events := task.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "TaskReviewFailedFatalV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "TaskReviewFailedFatalV1")
	}
}

func TestTaskCancelFromPending(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")

	err := task.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if task.Status != TaskStatusCancelled {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusCancelled)
	}
	if task.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	events := task.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "TaskCancelledV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "TaskCancelledV1")
	}
}

func TestTaskCancelFromSubmitted(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusSubmitted

	err := task.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if task.Status != TaskStatusCancelled {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusCancelled)
	}
}

func TestTaskCancelFromDecomposing(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusDecomposing

	err := task.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if task.Status != TaskStatusCancelled {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusCancelled)
	}
}

func TestTaskCancelFromAssigned(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusAssigned

	err := task.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if task.Status != TaskStatusCancelled {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusCancelled)
	}
}

func TestTaskCancelFromRunning(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusRunning

	err := task.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if task.Status != TaskStatusCancelled {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusCancelled)
	}
}

func TestTaskCancelFromReviewing(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusReviewing

	err := task.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if task.Status != TaskStatusCancelled {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusCancelled)
	}
}

func TestTaskCancelFromTerminalStateFails(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusCompleted

	err := task.Cancel()
	if err == nil {
		t.Fatal("expected error when cancelling from completed state, got nil")
	}

	task.Status = TaskStatusFailed
	err = task.Cancel()
	if err == nil {
		t.Fatal("expected error when cancelling from failed state, got nil")
	}

	task.Status = TaskStatusCancelled
	err = task.Cancel()
	if err == nil {
		t.Fatal("expected error when cancelling from cancelled state, got nil")
	}
}

func TestTaskFailDecomposition(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusDecomposing

	err := task.FailDecomposition()
	if err != nil {
		t.Fatalf("FailDecomposition() error = %v", err)
	}
	if task.Status != TaskStatusFailed {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusFailed)
	}
}

func TestTaskFailExecution(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusRunning

	err := task.FailExecution()
	if err != nil {
		t.Fatalf("FailExecution() error = %v", err)
	}
	if task.Status != TaskStatusFailed {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusFailed)
	}
	if task.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestTaskFailNoAgentAvailable(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Status = TaskStatusAssigned

	err := task.FailNoAgentAvailable()
	if err != nil {
		t.Fatalf("FailNoAgentAvailable() error = %v", err)
	}
	if task.Status != TaskStatusFailed {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusFailed)
	}
}

func TestTaskAvailableEvents(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")

	events := task.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("pending should have 2 available events, got %d", len(events))
	}

	// Move to submitted
	task.Status = TaskStatusSubmitted
	events = task.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("submitted should have 2 available events, got %d", len(events))
	}

	// Move to decomposing
	task.Status = TaskStatusDecomposing
	events = task.AvailableEvents()
	if len(events) != 3 {
		t.Errorf("decomposing should have 3 available events, got %d", len(events))
	}
}

func TestTaskCanMethods(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")

	if !task.CanSubmit() {
		t.Error("pending task should be able to submit")
	}
	if task.CanStartDecomposition() {
		t.Error("pending task should NOT be able to start decomposition")
	}
	if !task.CanCancel() {
		t.Error("pending task should be able to cancel")
	}

	task.Status = TaskStatusSubmitted
	if task.CanSubmit() {
		t.Error("submitted task should NOT be able to submit again")
	}
	if !task.CanStartDecomposition() {
		t.Error("submitted task should be able to start decomposition")
	}
	if !task.CanCancel() {
		t.Error("submitted task should be able to cancel")
	}

	task.Status = TaskStatusCompleted
	if task.CanCancel() {
		t.Error("completed task should NOT be able to cancel")
	}
}

func TestTaskRecordTaskCreated(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
	task.Tags = []string{"urgent", "feature"}

	err := task.RecordTaskCreated()
	if err != nil {
		t.Fatalf("RecordTaskCreated() error = %v", err)
	}

	events := task.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "TaskCreatedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "TaskCreatedV1")
	}
	if events[0].AggregateID != "task-123" {
		t.Errorf("AggregateID = %q, want %q", events[0].AggregateID, "task-123")
	}
}

// ----------------------------------------------------------------------------
// Subtask Entity Tests
// ----------------------------------------------------------------------------

func TestNewSubtask(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")

	if subtask.ID != "sub-123" {
		t.Errorf("ID = %q, want %q", subtask.ID, "sub-123")
	}
	if subtask.TaskID != "task-456" {
		t.Errorf("TaskID = %q, want %q", subtask.TaskID, "task-456")
	}
	if subtask.Type != SubtaskTypeCoding {
		t.Errorf("Type = %q, want %q", subtask.Type, SubtaskTypeCoding)
	}
	if subtask.Status != TaskStatusPending {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusPending)
	}
	if subtask.Version != 1 {
		t.Errorf("Version = %d, want %d", subtask.Version, 1)
	}
}

func TestSubtaskAssign(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")

	err := subtask.Assign("agent-instance-789")
	if err != nil {
		t.Fatalf("Assign() error = %v", err)
	}
	if subtask.Status != TaskStatusAssigned {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusAssigned)
	}
	if subtask.AgentInstance == nil || *subtask.AgentInstance != "agent-instance-789" {
		t.Error("AgentInstance should be set to agent-instance-789")
	}

	events := subtask.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "SubtaskAssignedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "SubtaskAssignedV1")
	}
}

func TestSubtaskAssignFromNonPendingFails(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusAssigned

	err := subtask.Assign("agent-instance-789")
	if err == nil {
		t.Fatal("expected error when assigning non-pending subtask, got nil")
	}
}

func TestSubtaskStartExecution(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusAssigned

	err := subtask.StartExecution()
	if err != nil {
		t.Fatalf("StartExecution() error = %v", err)
	}
	if subtask.Status != TaskStatusRunning {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusRunning)
	}
	if subtask.StartedAt == nil {
		t.Error("StartedAt should be set")
	}

	events := subtask.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "SubtaskExecutionStartedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "SubtaskExecutionStartedV1")
	}
}

func TestSubtaskStartExecutionFromNonAssignedFails(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	// subtask is pending

	err := subtask.StartExecution()
	if err == nil {
		t.Fatal("expected error when starting execution from non-assigned state, got nil")
	}
}

func TestSubtaskCompleteExecution(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusRunning

	err := subtask.CompleteExecution()
	if err != nil {
		t.Fatalf("CompleteExecution() error = %v", err)
	}
	if subtask.Status != TaskStatusReviewing {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusReviewing)
	}

	events := subtask.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "SubtaskExecutionCompletedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "SubtaskExecutionCompletedV1")
	}
}

func TestSubtaskReviewPassed(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusReviewing

	err := subtask.ReviewPassed()
	if err != nil {
		t.Fatalf("ReviewPassed() error = %v", err)
	}
	if subtask.Status != TaskStatusCompleted {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusCompleted)
	}
	if subtask.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	events := subtask.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "SubtaskReviewPassedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "SubtaskReviewPassedV1")
	}
}

func TestSubtaskReviewFailedRetry(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusReviewing

	err := subtask.ReviewFailedRetry()
	if err != nil {
		t.Fatalf("ReviewFailedRetry() error = %v", err)
	}
	if subtask.Status != TaskStatusRunning {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusRunning)
	}

	events := subtask.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "SubtaskReviewFailedRetryV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "SubtaskReviewFailedRetryV1")
	}
}

func TestSubtaskReviewFailedFatal(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusReviewing

	err := subtask.ReviewFailedFatal()
	if err != nil {
		t.Fatalf("ReviewFailedFatal() error = %v", err)
	}
	if subtask.Status != TaskStatusFailed {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusFailed)
	}
	if subtask.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	events := subtask.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "SubtaskReviewFailedFatalV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "SubtaskReviewFailedFatalV1")
	}
}

func TestSubtaskFailExecution(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusRunning

	err := subtask.FailExecution()
	if err != nil {
		t.Fatalf("FailExecution() error = %v", err)
	}
	if subtask.Status != TaskStatusFailed {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusFailed)
	}
	if subtask.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	events := subtask.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "SubtaskExecutionFailedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "SubtaskExecutionFailedV1")
	}
}

func TestSubtaskFailNoAgentAvailable(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusAssigned

	err := subtask.FailNoAgentAvailable()
	if err != nil {
		t.Fatalf("FailNoAgentAvailable() error = %v", err)
	}
	if subtask.Status != TaskStatusFailed {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusFailed)
	}

	events := subtask.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "SubtaskNoAgentAvailableV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "SubtaskNoAgentAvailableV1")
	}
}

func TestSubtaskCancelFromPending(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")

	err := subtask.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if subtask.Status != TaskStatusCancelled {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusCancelled)
	}
	if subtask.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	events := subtask.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "SubtaskCancelledV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "SubtaskCancelledV1")
	}
}

func TestSubtaskCancelFromAssigned(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusAssigned

	err := subtask.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if subtask.Status != TaskStatusCancelled {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusCancelled)
	}
}

func TestSubtaskCancelFromRunning(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusRunning

	err := subtask.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if subtask.Status != TaskStatusCancelled {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusCancelled)
	}
}

func TestSubtaskCancelFromReviewing(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Status = TaskStatusReviewing

	err := subtask.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if subtask.Status != TaskStatusCancelled {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusCancelled)
	}
}

func TestSubtaskCancelFromTerminalStateFails(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")

	subtask.Status = TaskStatusCompleted
	err := subtask.Cancel()
	if err == nil {
		t.Fatal("expected error when cancelling from completed state, got nil")
	}

	subtask.Status = TaskStatusFailed
	err = subtask.Cancel()
	if err == nil {
		t.Fatal("expected error when cancelling from failed state, got nil")
	}

	subtask.Status = TaskStatusCancelled
	err = subtask.Cancel()
	if err == nil {
		t.Fatal("expected error when cancelling from cancelled state, got nil")
	}
}

func TestSubtaskAvailableEvents(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")

	events := subtask.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("pending should have 2 available events, got %d", len(events))
	}

	subtask.Status = TaskStatusAssigned
	events = subtask.AvailableEvents()
	if len(events) != 3 {
		t.Errorf("assigned should have 3 available events, got %d", len(events))
	}

	subtask.Status = TaskStatusRunning
	events = subtask.AvailableEvents()
	if len(events) != 3 {
		t.Errorf("running should have 3 available events, got %d", len(events))
	}

	subtask.Status = TaskStatusReviewing
	events = subtask.AvailableEvents()
	if len(events) != 4 {
		t.Errorf("reviewing should have 4 available events, got %d", len(events))
	}
}

func TestSubtaskCanMethods(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")

	if !subtask.CanAssign() {
		t.Error("pending subtask should be able to assign")
	}
	if subtask.CanStartExecution() {
		t.Error("pending subtask should NOT be able to start execution")
	}
	if !subtask.CanCancel() {
		t.Error("pending subtask should be able to cancel")
	}

	subtask.Status = TaskStatusAssigned
	if subtask.CanAssign() {
		t.Error("assigned subtask should NOT be able to assign again")
	}
	if !subtask.CanStartExecution() {
		t.Error("assigned subtask should be able to start execution")
	}
	if !subtask.CanCancel() {
		t.Error("assigned subtask should be able to cancel")
	}

	subtask.Status = TaskStatusCompleted
	if subtask.CanCancel() {
		t.Error("completed subtask should NOT be able to cancel")
	}
}

func TestSubtaskRecordSubtaskCreated(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")
	subtask.Dependencies = []string{"sub-001", "sub-002"}

	err := subtask.RecordSubtaskCreated()
	if err != nil {
		t.Fatalf("RecordSubtaskCreated() error = %v", err)
	}

	events := subtask.FlushEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "SubtaskCreatedV1" {
		t.Errorf("EventType = %q, want %q", events[0].EventType, "SubtaskCreatedV1")
	}
	if events[0].AggregateID != "sub-123" {
		t.Errorf("AggregateID = %q, want %q", events[0].AggregateID, "sub-123")
	}
}

// ----------------------------------------------------------------------------
// SubtaskType Tests
// ----------------------------------------------------------------------------

func TestSubtaskTypeIsValid(t *testing.T) {
	tests := []struct {
		subType SubtaskType
		valid   bool
	}{
		{SubtaskTypeAnalysis, true},
		{SubtaskTypeCoding, true},
		{SubtaskTypeReview, true},
		{SubtaskTypeTesting, true},
		{SubtaskTypeResearch, true},
		{SubtaskType("invalid"), false},
	}

	for _, tt := range tests {
		if got := tt.subType.IsValid(); got != tt.valid {
			t.Errorf("SubtaskType(%q).IsValid() = %v, want %v", tt.subType, got, tt.valid)
		}
	}
}

// ----------------------------------------------------------------------------
// AgentRole Tests
// ----------------------------------------------------------------------------

func TestAgentRoleIsValid(t *testing.T) {
	tests := []struct {
		role   AgentRole
		valid  bool
	}{
		{AgentRoleObserver, true},
		{AgentRoleStrategist, true},
		{AgentRoleExecutor, true},
		{AgentRoleGuardian, true},
		{AgentRoleTester, true},
		{AgentRoleResearcher, true},
		{AgentRole("invalid"), false},
	}

	for _, tt := range tests {
		if got := tt.role.IsValid(); got != tt.valid {
			t.Errorf("AgentRole(%q).IsValid() = %v, want %v", tt.role, got, tt.valid)
		}
	}
}

// ----------------------------------------------------------------------------
// DomainEvent Tests
// ----------------------------------------------------------------------------

func TestNewDomainEvent(t *testing.T) {
	event := NewDomainEvent("Task", "task-123", "TaskCreatedV1", []byte(`{"goal":"test"}`), 1)

	if event.EventID == "" {
		t.Error("EventID should not be empty")
	}
	if event.AggregateType != "Task" {
		t.Errorf("AggregateType = %q, want %q", event.AggregateType, "Task")
	}
	if event.AggregateID != "task-123" {
		t.Errorf("AggregateID = %q, want %q", event.AggregateID, "task-123")
	}
	if event.EventType != "TaskCreatedV1" {
		t.Errorf("EventType = %q, want %q", event.EventType, "TaskCreatedV1")
	}
	if event.Version != 1 {
		t.Errorf("Version = %d, want %d", event.Version, 1)
	}
	if event.OccurredAt.IsZero() {
		t.Error("OccurredAt should not be zero")
	}
	expectedKey := "task-123:TaskCreatedV1:1"
	if event.IdempotencyKey != expectedKey {
		t.Errorf("IdempotencyKey = %q, want %q", event.IdempotencyKey, expectedKey)
	}
}

func TestAggregateRootRecordAndFlushEvents(t *testing.T) {
	task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")

	task.RecordTaskCreated()
	if len(task.events) != 1 {
		t.Errorf("expected 1 event, got %d", len(task.events))
	}

	events := task.FlushEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event after flush, got %d", len(events))
	}

	// After flush, events should be empty
	events = task.FlushEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events after second flush, got %d", len(events))
	}
}

// ----------------------------------------------------------------------------
// Full Task Lifecycle Tests
// ----------------------------------------------------------------------------

func TestTaskFullHappyPath(t *testing.T) {
	task := NewTask("task-123", "Implement full feature", "https://github.com/example/repo", "main", "client-abc")

	// 1. Submit
	if err := task.Submit(); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if task.Status != TaskStatusSubmitted {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusSubmitted)
	}

	// 2. StartDecomposition
	if err := task.StartDecomposition(); err != nil {
		t.Fatalf("StartDecomposition() error = %v", err)
	}
	if task.Status != TaskStatusDecomposing {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusDecomposing)
	}

	// 3. CompleteDecomposition
	if err := task.CompleteDecomposition(); err != nil {
		t.Fatalf("CompleteDecomposition() error = %v", err)
	}
	if task.Status != TaskStatusAssigned {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusAssigned)
	}

	// 4. StartExecution
	if err := task.StartExecution(); err != nil {
		t.Fatalf("StartExecution() error = %v", err)
	}
	if task.Status != TaskStatusRunning {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusRunning)
	}

	// 5. CompleteExecution
	if err := task.CompleteExecution(); err != nil {
		t.Fatalf("CompleteExecution() error = %v", err)
	}
	if task.Status != TaskStatusReviewing {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusReviewing)
	}

	// 6. ReviewPassed
	if err := task.ReviewPassed(); err != nil {
		t.Fatalf("ReviewPassed() error = %v", err)
	}
	if task.Status != TaskStatusCompleted {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusCompleted)
	}
	if task.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestSubtaskFullHappyPath(t *testing.T) {
	subtask := NewSubtask("sub-123", "task-456", SubtaskTypeCoding, "Implement feature X", "executor")

	// 1. Assign
	if err := subtask.Assign("agent-instance-789"); err != nil {
		t.Fatalf("Assign() error = %v", err)
	}
	if subtask.Status != TaskStatusAssigned {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusAssigned)
	}

	// 2. StartExecution
	if err := subtask.StartExecution(); err != nil {
		t.Fatalf("StartExecution() error = %v", err)
	}
	if subtask.Status != TaskStatusRunning {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusRunning)
	}

	// 3. CompleteExecution
	if err := subtask.CompleteExecution(); err != nil {
		t.Fatalf("CompleteExecution() error = %v", err)
	}
	if subtask.Status != TaskStatusReviewing {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusReviewing)
	}

	// 4. ReviewPassed
	if err := subtask.ReviewPassed(); err != nil {
		t.Fatalf("ReviewPassed() error = %v", err)
	}
	if subtask.Status != TaskStatusCompleted {
		t.Errorf("Status = %q, want %q", subtask.Status, TaskStatusCompleted)
	}
	if subtask.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestTaskReviewFailedRetryPath(t *testing.T) {
	task := NewTask("task-123", "Implement feature", "https://github.com/example/repo", "main", "client-abc")

	// Move to reviewing
	task.Status = TaskStatusReviewing

	// Review failed, retry
	if err := task.ReviewFailedRetry(); err != nil {
		t.Fatalf("ReviewFailedRetry() error = %v", err)
	}
	if task.Status != TaskStatusRunning {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusRunning)
	}

	// Complete execution again
	if err := task.CompleteExecution(); err != nil {
		t.Fatalf("CompleteExecution() error = %v", err)
	}
	if task.Status != TaskStatusReviewing {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusReviewing)
	}

	// Review passed this time
	if err := task.ReviewPassed(); err != nil {
		t.Fatalf("ReviewPassed() error = %v", err)
	}
	if task.Status != TaskStatusCompleted {
		t.Errorf("Status = %q, want %q", task.Status, TaskStatusCompleted)
	}
}

func TestTaskCancelAtEachNonTerminalState(t *testing.T) {
	states := []TaskStatus{
		TaskStatusPending,
		TaskStatusSubmitted,
		TaskStatusDecomposing,
		TaskStatusAssigned,
		TaskStatusRunning,
		TaskStatusReviewing,
	}

	for _, state := range states {
		task := NewTask("task-123", "Test task", "https://github.com/example/repo", "main", "client-abc")
		task.Status = state

		err := task.Cancel()
		if err != nil {
			t.Errorf("Cancel() from %s error = %v", state, err)
		}
		if task.Status != TaskStatusCancelled {
			t.Errorf("Status after cancel from %s = %q, want %q", state, task.Status, TaskStatusCancelled)
		}
	}
}