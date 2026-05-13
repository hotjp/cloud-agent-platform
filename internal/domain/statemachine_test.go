package domain

import (
	"errors"
	"testing"
)

// ----------------------------------------------------------------------------
// Test Utilities
// ----------------------------------------------------------------------------

// mockAuditLogger records transition calls for testing.
type mockAuditLogger struct {
	Calls []TransitionRecord
}

func (m *mockAuditLogger) LogStateTransition(entityType, entityID string, fromState, toState any, event string, version int64, success bool, err error) {
	m.Calls = append(m.Calls, TransitionRecord{
		EntityType:   entityType,
		EntityID:     entityID,
		FromState:    fromState,
		ToState:      toState,
		Event:        event,
		Version:      version,
		Success:      success,
		ErrorMessage: "",
	})
	if err != nil {
		m.Calls[len(m.Calls)-1].ErrorMessage = err.Error()
	}
}

func (m *mockAuditLogger) Reset() {
	m.Calls = nil
}

// ----------------------------------------------------------------------------
// Generic State Machine Tests
// ----------------------------------------------------------------------------

func TestNewStateMachineOf(t *testing.T) {
	t.Run("basic creation", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		if sm.EntityType != "Order" {
			t.Errorf("EntityType = %q, want %q", sm.EntityType, "Order")
		}
		if sm.EntityID != "order-123" {
			t.Errorf("EntityID = %q, want %q", sm.EntityID, "order-123")
		}
		if sm.State != "pending" {
			t.Errorf("State = %q, want %q", sm.State, "pending")
		}
		if sm.Version != 1 {
			t.Errorf("Version = %d, want %d", sm.Version, 1)
		}
	})

	t.Run("with audit logger", func(t *testing.T) {
		logger := &mockAuditLogger{}
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", logger)
		if sm.auditLogger != logger {
			t.Error("audit logger not set correctly")
		}
	})

	t.Run("nil audit logger uses no-op", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		if sm.auditLogger == nil {
			t.Error("audit logger should not be nil")
		}
		_, ok := sm.auditLogger.(NoOpAuditLogger)
		if !ok {
			t.Error("should use NoOpAuditLogger when nil")
		}
	})
}

func TestGenericStateMachine_AddTransition(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	t1 := sm.AddTransition("pending", "processing", "start")
	if t1 == nil {
		t.Fatal("AddTransition returned nil")
	}
	if t1.From != "pending" || t1.To != "processing" || t1.Event != "start" {
		t.Errorf("transition fields incorrect")
	}
}

func TestGenericStateMachine_WithGuard(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start").
		WithGuard(func(sm *StateMachine[string, string]) (bool, error) {
			return sm.Version > 0, nil
		})

	if !sm.CanTransition("start") {
		t.Error("Should be able to transition")
	}

	allowed, err := sm.TransitionAllowed("start")
	if err != nil {
		t.Fatalf("TransitionAllowed() error = %v", err)
	}
	if !allowed {
		t.Error("TransitionAllowed should return true")
	}
}

func TestGenericStateMachine_WithAction(t *testing.T) {
	var actionCalled bool
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start").
		WithAction(func(sm *StateMachine[string, string]) error {
			actionCalled = true
			return nil
		})

	newState, err := sm.Trigger("start")
	if err != nil {
		t.Fatalf("Trigger() error = %v", err)
	}
	if newState != "processing" {
		t.Errorf("newState = %q, want %q", newState, "processing")
	}
	if !actionCalled {
		t.Error("Action should have been called")
	}
}

func TestGenericStateMachine_Trigger(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start")
	sm.AddTransition("processing", "shipped", "ship")

	// First transition
	newState, err := sm.Trigger("start")
	if err != nil {
		t.Fatalf("Trigger(start) error = %v", err)
	}
	if newState != "processing" {
		t.Errorf("newState = %q, want %q", newState, "processing")
	}
	if sm.State != "processing" {
		t.Errorf("sm.State = %q, want %q", sm.State, "processing")
	}
	if sm.Version != 2 {
		t.Errorf("Version = %d, want %d", sm.Version, 2)
	}

	// Second transition
	newState, err = sm.Trigger("ship")
	if err != nil {
		t.Fatalf("Trigger(ship) error = %v", err)
	}
	if newState != "shipped" {
		t.Errorf("newState = %q, want %q", newState, "shipped")
	}
	if sm.Version != 3 {
		t.Errorf("Version = %d, want %d", sm.Version, 3)
	}
}

func TestGenericStateMachine_TriggerInvalidTransition(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start")

	// Try an invalid transition
	_, err := sm.Trigger("cancel")
	if err == nil {
		t.Fatal("expected error for invalid transition, got nil")
	}

	// State should not change
	if sm.State != "pending" {
		t.Errorf("State = %q, want %q", sm.State, "pending")
	}
	// Version should not change
	if sm.Version != 1 {
		t.Errorf("Version = %d, want %d", sm.Version, 1)
	}
}

func TestGenericStateMachine_CanTransition(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start")
	sm.AddTransition("pending", "cancelled", "cancel")

	if !sm.CanTransition("start") {
		t.Error("Should be able to transition with start")
	}
	if !sm.CanTransition("cancel") {
		t.Error("Should be able to transition with cancel")
	}
	if sm.CanTransition("ship") {
		t.Error("Should NOT be able to transition with ship")
	}
}

func TestGenericStateMachine_TransitionAllowed(t *testing.T) {
	t.Run("valid transition without guards", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start")

		allowed, err := sm.TransitionAllowed("start")
		if err != nil {
			t.Fatalf("TransitionAllowed() error = %v", err)
		}
		if !allowed {
			t.Error("TransitionAllowed should return true for valid transition")
		}
	})

	t.Run("transition not defined", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start")

		allowed, err := sm.TransitionAllowed("cancel")
		if err != nil {
			t.Fatalf("TransitionAllowed() error = %v", err)
		}
		if allowed {
			t.Error("TransitionAllowed should return false for undefined transition")
		}
	})

	t.Run("guard blocks transition", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithGuard(func(sm *StateMachine[string, string]) (bool, error) {
				return false, nil
			})

		allowed, err := sm.TransitionAllowed("start")
		if err != nil {
			t.Fatalf("TransitionAllowed() error = %v", err)
		}
		if allowed {
			t.Error("TransitionAllowed should return false when guard blocks")
		}
	})

	t.Run("guard returns error", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithGuard(func(sm *StateMachine[string, string]) (bool, error) {
				return false, errors.New("guard evaluation failed")
			})

		_, err := sm.TransitionAllowed("start")
		if err == nil {
			t.Error("TransitionAllowed should return error when guard errors")
		}
	})
}

func TestGenericStateMachine_Reset(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start")

	sm.Trigger("start")
	if sm.State != "processing" {
		t.Errorf("State = %q, want %q", sm.State, "processing")
	}
	if sm.Version != 2 {
		t.Errorf("Version = %d, want %d", sm.Version, 2)
	}

	sm.Reset()
	if sm.State != "pending" {
		t.Errorf("State after reset = %q, want %q", sm.State, "pending")
	}
	if sm.Version != 1 {
		t.Errorf("Version after reset = %d, want %d", sm.Version, 1)
	}
}

func TestGenericStateMachine_AvailableEvents(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start")
	sm.AddTransition("pending", "cancelled", "cancel")

	events := sm.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("AvailableEvents count = %d, want 2", len(events))
	}
}

func TestGenericStateMachine_Transitions(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start")
	sm.AddTransition("pending", "cancelled", "cancel")

	transitions := sm.Transitions()
	if len(transitions) != 2 {
		t.Errorf("Transitions count = %d, want 2", len(transitions))
	}

	// After moving state, transitions should change
	sm.Trigger("start")
	transitions = sm.Transitions()
	if len(transitions) != 0 {
		t.Errorf("After start, should have 0 transitions, got %d", len(transitions))
	}
}

// ----------------------------------------------------------------------------
// Guard Factory Tests
// ----------------------------------------------------------------------------

func TestGuardAnd(t *testing.T) {
	t.Run("all pass", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithGuard(GuardAnd(
				func(sm *StateMachine[string, string]) (bool, error) { return true, nil },
				func(sm *StateMachine[string, string]) (bool, error) { return true, nil },
			))

		allowed, err := sm.TransitionAllowed("start")
		if err != nil {
			t.Fatalf("TransitionAllowed() error = %v", err)
		}
		if !allowed {
			t.Error("GuardAnd should pass when all guards pass")
		}
	})

	t.Run("one fails", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithGuard(GuardAnd(
				func(sm *StateMachine[string, string]) (bool, error) { return true, nil },
				func(sm *StateMachine[string, string]) (bool, error) { return false, nil },
			))

		allowed, err := sm.TransitionAllowed("start")
		if err != nil {
			t.Fatalf("TransitionAllowed() error = %v", err)
		}
		if allowed {
			t.Error("GuardAnd should fail when any guard fails")
		}
	})

	t.Run("one errors", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithGuard(GuardAnd(
				func(sm *StateMachine[string, string]) (bool, error) { return true, nil },
				func(sm *StateMachine[string, string]) (bool, error) { return false, errors.New("guard error") },
			))

		_, err := sm.TransitionAllowed("start")
		if err == nil {
			t.Error("GuardAnd should error when any guard errors")
		}
	})
}

func TestGuardOr(t *testing.T) {
	t.Run("one passes", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithGuard(GuardOr(
				func(sm *StateMachine[string, string]) (bool, error) { return false, nil },
				func(sm *StateMachine[string, string]) (bool, error) { return true, nil },
			))

		allowed, err := sm.TransitionAllowed("start")
		if err != nil {
			t.Fatalf("TransitionAllowed() error = %v", err)
		}
		if !allowed {
			t.Error("GuardOr should pass when any guard passes")
		}
	})

	t.Run("all fail", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithGuard(GuardOr(
				func(sm *StateMachine[string, string]) (bool, error) { return false, nil },
				func(sm *StateMachine[string, string]) (bool, error) { return false, nil },
			))

		allowed, err := sm.TransitionAllowed("start")
		if err != nil {
			t.Fatalf("TransitionAllowed() error = %v", err)
		}
		if allowed {
			t.Error("GuardOr should fail when all guards fail")
		}
	})
}

func TestGuardNot(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start").
		WithGuard(GuardNot(func(sm *StateMachine[string, string]) (bool, error) {
			return false, nil
		}))

	allowed, err := sm.TransitionAllowed("start")
	if err != nil {
		t.Fatalf("TransitionAllowed() error = %v", err)
	}
	if !allowed {
		t.Error("GuardNot should invert false to true")
	}
}

func TestGuardFieldNotZero(t *testing.T) {
	t.Run("string not empty passes", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithGuard(GuardFieldNotZero("entity_id", func(sm *StateMachine[string, string]) any {
				return sm.EntityID
			}))

		allowed, err := sm.TransitionAllowed("start")
		if err != nil {
			t.Fatalf("TransitionAllowed() error = %v", err)
		}
		if !allowed {
			t.Error("GuardFieldNotZero should pass for non-empty string")
		}
	})

	t.Run("string empty fails with error", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithGuard(GuardFieldNotZero("entity_id", func(sm *StateMachine[string, string]) any {
				return sm.EntityID
			}))

		// GuardFieldNotZero returns error when field is empty (business rule violation)
		allowed, err := sm.TransitionAllowed("start")
		if err == nil {
			t.Error("GuardFieldNotZero should return error for empty string")
		}
		if allowed {
			t.Error("GuardFieldNotZero should fail for empty string")
		}
	})
}

func TestGuardVersionMatch(t *testing.T) {
	t.Run("version matches", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.Version = 5
		sm.AddTransition("pending", "processing", "start").
			WithGuard(GuardVersionMatch[string, string](5))

		allowed, err := sm.TransitionAllowed("start")
		if err != nil {
			t.Fatalf("TransitionAllowed() error = %v", err)
		}
		if !allowed {
			t.Error("GuardVersionMatch should pass when version matches")
		}
	})

	t.Run("version mismatch returns error", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.Version = 5
		sm.AddTransition("pending", "processing", "start").
			WithGuard(GuardVersionMatch[string, string](3))

		// GuardVersionMatch returns error on version mismatch (optimistic lock error)
		allowed, err := sm.TransitionAllowed("start")
		if err == nil {
			t.Error("GuardVersionMatch should return error when version doesn't match")
		}
		if allowed {
			t.Error("GuardVersionMatch should fail when version doesn't match")
		}
	})
}

func TestGuardStateIn(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start").
		WithGuard(GuardStateIn[string, string]("pending", "created"))

	allowed, err := sm.TransitionAllowed("start")
	if err != nil {
		t.Fatalf("TransitionAllowed() error = %v", err)
	}
	if !allowed {
		t.Error("GuardStateIn should pass when state is in allowed list")
	}
}

func TestGuardStateNotIn(t *testing.T) {
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
	sm.AddTransition("pending", "processing", "start").
		WithGuard(GuardStateNotIn[string, string]("completed", "cancelled"))

	allowed, err := sm.TransitionAllowed("start")
	if err != nil {
		t.Fatalf("TransitionAllowed() error = %v", err)
	}
	if !allowed {
		t.Error("GuardStateNotIn should pass when state is not in forbidden list")
	}
}

// ----------------------------------------------------------------------------
// Action Factory Tests
// ----------------------------------------------------------------------------

func TestActionChain(t *testing.T) {
	t.Run("all actions succeed", func(t *testing.T) {
		var called []string
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithAction(ActionChain(
				func(sm *StateMachine[string, string]) error { called = append(called, "a"); return nil },
				func(sm *StateMachine[string, string]) error { called = append(called, "b"); return nil },
			))

		_, err := sm.Trigger("start")
		if err != nil {
			t.Fatalf("Trigger() error = %v", err)
		}
		if len(called) != 2 || called[0] != "a" || called[1] != "b" {
			t.Errorf("ActionChain actions called in wrong order: %v", called)
		}
	})

	t.Run("second action fails", func(t *testing.T) {
		sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", nil)
		sm.AddTransition("pending", "processing", "start").
			WithAction(ActionChain(
				func(sm *StateMachine[string, string]) error { return nil },
				func(sm *StateMachine[string, string]) error { return errors.New("action failed") },
			))

		_, err := sm.Trigger("start")
		if err == nil {
			t.Error("Trigger should fail when action chain fails")
		}
		// State should not change on failed action
		if sm.State != "pending" {
			t.Errorf("State should not change on failed action, got %q", sm.State)
		}
	})
}

// ----------------------------------------------------------------------------
// Audit Logger Tests
// ----------------------------------------------------------------------------

func TestAuditLoggerCalledOnTransition(t *testing.T) {
	logger := &mockAuditLogger{}
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", logger)
	sm.AddTransition("pending", "processing", "start")

	sm.Trigger("start")

	if len(logger.Calls) != 1 {
		t.Fatalf("expected 1 audit call, got %d", len(logger.Calls))
	}
	call := logger.Calls[0]
	if call.EntityType != "Order" {
		t.Errorf("EntityType = %q, want %q", call.EntityType, "Order")
	}
	if call.EntityID != "order-123" {
		t.Errorf("EntityID = %q, want %q", call.EntityID, "order-123")
	}
	if call.FromState != "pending" {
		t.Errorf("FromState = %v, want pending", call.FromState)
	}
	if call.ToState != "processing" {
		t.Errorf("ToState = %v, want processing", call.ToState)
	}
	if call.Event != "start" {
		t.Errorf("Event = %q, want %q", call.Event, "start")
	}
	if call.Version != 2 {
		t.Errorf("Version = %d, want %d", call.Version, 2)
	}
	if !call.Success {
		t.Error("Success should be true")
	}
}

func TestAuditLoggerCalledOnFailedTransition(t *testing.T) {
	logger := &mockAuditLogger{}
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", logger)
	sm.AddTransition("pending", "processing", "start").
		WithGuard(func(sm *StateMachine[string, string]) (bool, error) {
			return false, nil
		})

	sm.Trigger("start")

	if len(logger.Calls) != 1 {
		t.Fatalf("expected 1 audit call, got %d", len(logger.Calls))
	}
	call := logger.Calls[0]
	if call.Success {
		t.Error("Success should be false for blocked transition")
	}
}

func TestAuditLoggerCalledOnInvalidTransition(t *testing.T) {
	logger := &mockAuditLogger{}
	sm := NewStateMachineOf[string, string]("Order", "order-123", "pending", logger)
	// No transitions defined

	sm.Trigger("start")

	if len(logger.Calls) != 1 {
		t.Fatalf("expected 1 audit call, got %d", len(logger.Calls))
	}
	call := logger.Calls[0]
	if call.Success {
		t.Error("Success should be false for invalid transition")
	}
}

// ----------------------------------------------------------------------------
// Task State Machine Tests (backward compatibility)
// ----------------------------------------------------------------------------

func TestBuildTaskStateMachine(t *testing.T) {
	sm := BuildTaskStateMachine("task-123")

	if sm.EntityType != "Task" {
		t.Errorf("EntityType = %q, want %q", sm.EntityType, "Task")
	}
	if sm.EntityID != "task-123" {
		t.Errorf("EntityID = %q, want %q", sm.EntityID, "task-123")
	}
	if sm.State != TaskStatusPending {
		t.Errorf("State = %q, want %q", sm.State, TaskStatusPending)
	}

	// Verify key transitions exist
	if !sm.CanTransition("StartDecomposition") {
		t.Error("Should have StartDecomposition transition from pending")
	}
	if !sm.CanTransition("Cancel") {
		t.Error("Should have Cancel transition from pending")
	}
}

func TestBuildSubtaskStateMachine(t *testing.T) {
	sm := BuildSubtaskStateMachine("sub-123")

	if sm.EntityType != "Subtask" {
		t.Errorf("EntityType = %q, want %q", sm.EntityType, "Subtask")
	}
	if sm.State != TaskStatusPending {
		t.Errorf("State = %q, want %q", sm.State, TaskStatusPending)
	}

	// Verify subtask-specific transitions from pending
	if !sm.CanTransition("Assign") {
		t.Error("Should have Assign transition from pending")
	}
}

func TestTaskStateMachine_FullWorkflow(t *testing.T) {
	sm := BuildTaskStateMachine("task-123")

	// pending -> decomposing
	newState, err := sm.Trigger("StartDecomposition")
	if err != nil {
		t.Fatalf("Trigger(StartDecomposition) error = %v", err)
	}
	if newState != TaskStatusDecomposing {
		t.Errorf("State = %q, want %q", newState, TaskStatusDecomposing)
	}

	// decomposing -> dispatched
	newState, err = sm.Trigger("DecompositionComplete")
	if err != nil {
		t.Fatalf("Trigger(DecompositionComplete) error = %v", err)
	}
	if newState != TaskStatusDispatched {
		t.Errorf("State = %q, want %q", newState, TaskStatusDispatched)
	}

	// dispatched -> running
	newState, err = sm.Trigger("StartExecution")
	if err != nil {
		t.Fatalf("Trigger(StartExecution) error = %v", err)
	}
	if newState != TaskStatusRunning {
		t.Errorf("State = %q, want %q", newState, TaskStatusRunning)
	}

	// running -> reviewing
	newState, err = sm.Trigger("CompleteExecution")
	if err != nil {
		t.Fatalf("Trigger(CompleteExecution) error = %v", err)
	}
	if newState != TaskStatusReviewing {
		t.Errorf("State = %q, want %q", newState, TaskStatusReviewing)
	}

	// reviewing -> completed
	newState, err = sm.Trigger("ReviewPassed")
	if err != nil {
		t.Fatalf("Trigger(ReviewPassed) error = %v", err)
	}
	if newState != TaskStatusCompleted {
		t.Errorf("State = %q, want %q", newState, TaskStatusCompleted)
	}

	// Verify no more transitions from completed
	if len(sm.AvailableEvents()) != 0 {
		t.Error("Completed state should have no available events")
	}
}

func TestTaskStateMachine_IllegalTransitions(t *testing.T) {
	sm := BuildTaskStateMachine("task-123")

	// Try to go from pending directly to running - should fail
	_, err := sm.Trigger("StartExecution")
	if err == nil {
		t.Error("pending -> running should be illegal")
	}
	if sm.State != TaskStatusPending {
		t.Errorf("State should still be pending, got %q", sm.State)
	}

	// Try cancel from pending - should succeed
	_, err = sm.Trigger("Cancel")
	if err != nil {
		t.Errorf("pending -> cancelled should succeed, got error: %v", err)
	}
	if sm.State != TaskStatusCancelled {
		t.Errorf("State = %q, want %q", sm.State, TaskStatusCancelled)
	}

	// Try any transition from cancelled - should all fail
	_, err = sm.Trigger("StartDecomposition")
	if err == nil {
		t.Error("Any transition from cancelled should fail")
	}
}

func TestTaskStateMachine_GuardBlocks(t *testing.T) {
	logger := &mockAuditLogger{}
	sm := BuildTaskStateMachineWithLogger("task-123", logger)

	// Add a guard that always blocks
	sm.AddTransition(TaskStatusPending, TaskStatusRunning, "GuardBlockedTransition").
		WithGuard(func(sm *TaskStateMachine) (bool, error) {
			return false, NewL2BusinessRuleViolatedError("always_blocked", nil)
		})

	// pending -> running should be blocked by guard
	_, err := sm.Trigger("GuardBlockedTransition")
	if err == nil {
		t.Error("Transition should be blocked by guard")
	}

	// State should remain pending
	if sm.State != TaskStatusPending {
		t.Errorf("State = %q, want %q", sm.State, TaskStatusPending)
	}

	// Version should not increment
	if sm.Version != 1 {
		t.Errorf("Version = %d, want %d", sm.Version, 1)
	}
}

func TestTaskStateMachine_CancelFromCancelableStates(t *testing.T) {
	// Cancel transitions are defined for: pending, decomposing, dispatched, running, confirming
	// NOT for: reviewing (by design, you can't cancel during review)
	cancelableStates := []TaskStatus{
		TaskStatusPending,
		TaskStatusDecomposing,
		TaskStatusDispatched,
		TaskStatusRunning,
		TaskStatusConfirming,
	}

	for _, fromState := range cancelableStates {
		sm := BuildTaskStateMachine("task-123")
		sm.State = fromState

		_, err := sm.Trigger("Cancel")
		if err != nil {
			t.Errorf("Cancel from %q should succeed, got error: %v", fromState, err)
		}
		if sm.State != TaskStatusCancelled {
			t.Errorf("State = %q, want %q", sm.State, TaskStatusCancelled)
		}
	}
}

func TestTaskStateMachine_CannotCancelFromReviewing(t *testing.T) {
	// reviewing state does NOT have a Cancel transition by design
	sm := BuildTaskStateMachine("task-123")
	sm.State = TaskStatusReviewing

	_, err := sm.Trigger("Cancel")
	if err == nil {
		t.Error("Cancel from reviewing should fail (no transition defined)")
	}
	if sm.State != TaskStatusReviewing {
		t.Errorf("State should remain reviewing, got %q", sm.State)
	}
}

func TestTaskStateMachine_Validate(t *testing.T) {
	t.Run("nil state machine", func(t *testing.T) {
		err := ValidateStateMachine(nil)
		if err == nil {
			t.Error("ValidateStateMachine(nil) should return error")
		}
	})

	t.Run("valid state machine", func(t *testing.T) {
		sm := BuildTaskStateMachine("task-123")
		err := ValidateStateMachine(sm)
		if err != nil {
			t.Errorf("ValidateStateMachine() error = %v", err)
		}
	})

	t.Run("empty entity ID", func(t *testing.T) {
		sm := NewStateMachineOf[TaskStatus, string]("", "Task", TaskStatusPending, nil)
		err := ValidateStateMachine(sm)
		if err == nil {
			t.Error("ValidateStateMachine with empty entity ID should return error")
		}
	})
}

// ----------------------------------------------------------------------------
// Helper function tests
// ----------------------------------------------------------------------------

func TestAllTaskStates(t *testing.T) {
	states := AllTaskStates()
	if len(states) != 9 {
		t.Errorf("AllTaskStates count = %d, want 9", len(states))
	}
}

func TestAllTaskEvents(t *testing.T) {
	events := AllTaskEvents()
	if len(events) == 0 {
		t.Error("AllTaskEvents should not be empty")
	}
	// Events should be sorted
	for i := 1; i < len(events); i++ {
		if events[i-1] > events[i] {
			t.Error("AllTaskEvents should be sorted")
		}
	}
}

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		state    TaskStatus
		terminal bool
	}{
		{TaskStatusPending, false},
		{TaskStatusDecomposing, false},
		{TaskStatusDispatched, false},
		{TaskStatusRunning, false},
		{TaskStatusReviewing, false},
		{TaskStatusConfirming, false},
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

func TestFindTransition(t *testing.T) {
	tf := FindTransition(TaskStatusPending, "StartDecomposition", TaskStateMachineDefinition)
	if tf == nil {
		t.Fatal("FindTransition should find StartDecomposition")
	}
	if tf.To != TaskStatusDecomposing {
		t.Errorf("tf.To = %q, want %q", tf.To, TaskStatusDecomposing)
	}

	// Non-existent transition
	tf = FindTransition(TaskStatusPending, "NonExistent", TaskStateMachineDefinition)
	if tf != nil {
		t.Error("FindTransition should return nil for non-existent transition")
	}
}

// ----------------------------------------------------------------------------
// Integration: Multiple entity types using generic framework
// ----------------------------------------------------------------------------

// SessionStatus for testing generic state machine with different state type
type SessionStatus string

const (
	SessionStatusActive   SessionStatus = "active"
	SessionStatusIdle    SessionStatus = "idle"
	SessionStatusExpired SessionStatus = "expired"
)

// SessionEvent for testing generic state machine with different event type
type SessionEvent string

const (
	SessionEventPing  SessionEvent = "ping"
	SessionEventIdle  SessionEvent = "idle"
	SessionEventClose SessionEvent = "close"
)

func TestGenericStateMachineWithDifferentTypes(t *testing.T) {
	// Create a session state machine with different state/event types
	sm := NewStateMachineOf[SessionStatus, SessionEvent]("Session", "session-123", SessionStatusActive, nil)

	sm.AddTransition(SessionStatusActive, SessionStatusIdle, SessionEventIdle)
	sm.AddTransition(SessionStatusActive, SessionStatusExpired, SessionEventClose)
	sm.AddTransition(SessionStatusIdle, SessionStatusActive, SessionEventPing)
	sm.AddTransition(SessionStatusIdle, SessionStatusExpired, SessionEventClose)

	// active -> idle
	newState, err := sm.Trigger(SessionEventIdle)
	if err != nil {
		t.Fatalf("Trigger(idle) error = %v", err)
	}
	if newState != SessionStatusIdle {
		t.Errorf("State = %q, want %q", newState, SessionStatusIdle)
	}

	// idle -> active
	newState, err = sm.Trigger(SessionEventPing)
	if err != nil {
		t.Fatalf("Trigger(ping) error = %v", err)
	}
	if newState != SessionStatusActive {
		t.Errorf("State = %q, want %q", newState, SessionStatusActive)
	}

	// active -> expired
	newState, err = sm.Trigger(SessionEventClose)
	if err != nil {
		t.Fatalf("Trigger(close) error = %v", err)
	}
	if newState != SessionStatusExpired {
		t.Errorf("State = %q, want %q", newState, SessionStatusExpired)
	}
}
