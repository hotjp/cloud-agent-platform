// Package domain implements L2-Domain layer: domain entities, state machines,
// event collection (Outbox), and business invariants.
// This layer has ZERO external dependencies - pure Go structs + standard library.
package domain

import (
	"fmt"
	"slices"
)

// ----------------------------------------------------------------------------
// AuditLogger Interface
// Domain-layer interface for state transition audit logging.
// Implemented by L4-Service or infrastructure layer and injected via constructor.
// Since domain has zero external dependencies, we define the interface here
// and let infrastructure provide the concrete implementation (e.g., using zap).
// ----------------------------------------------------------------------------

// AuditLogger defines the interface for recording state transition audit logs.
// Implemented by the infrastructure layer (e.g., using zap, slog, etc.).
// The domain layer calls this interface without knowing the concrete implementation.
type AuditLogger interface {
	// LogStateTransition records a state transition event.
	// Parameters use any type since this is an interface boundary between layers.
	LogStateTransition(entityType, entityID string, fromState, toState any, event string, version int64, success bool, err error)
}

// TransitionRecord represents a single state transition for audit purposes.
type TransitionRecord struct {
	EntityType   string
	EntityID     string
	FromState    any
	ToState      any
	Event        string
	Version      int64
	Success      bool
	ErrorMessage string
}

// NoOpAuditLogger is a no-operation audit logger for environments that don't need logging.
type NoOpAuditLogger struct{}

// LogStateTransition implements AuditLogger with no-op (does nothing).
func (NoOpAuditLogger) LogStateTransition(entityType, entityID string, fromState, toState any, event string, version int64, success bool, err error) {
	// no-op
}

// ----------------------------------------------------------------------------
// Generic State Machine Framework
// ----------------------------------------------------------------------------

// Transition represents a declarative state transition rule.
// S is the state type (e.g., TaskStatus, string)
// E is the event type (e.g., string)
type Transition[S comparable, E comparable] struct {
	From    S
	To      S
	Event   E
	Guards  []Guard[S, E]
	Actions []Action[S, E]
}

// Guard is a function that evaluates a condition before allowing a transition.
// Returns (allowed, error):
//   - allowed=true: transition is permitted
//   - allowed=false: transition is blocked by business rule
//   - error: guard evaluation failed (transient error)
type Guard[S comparable, E comparable] func(sm *StateMachine[S, E]) (bool, error)

// Action is a function that executes during a successful transition.
// Actions run after all guards pass and before the state is updated.
// Return error to abort the transition.
type Action[S comparable, E comparable] func(sm *StateMachine[S, E]) error

// StateMachine is a generic, declarative state machine.
// S is the state type (must be comparable for map key usage)
// E is the event type (must be comparable for map key usage)
type StateMachine[S comparable, E comparable] struct {
	// EntityID is the unique identifier of the aggregate this state machine manages.
	EntityID string
	// EntityType is the type name of the aggregate (e.g., "Task", "Subtask", "Session").
	EntityType string
	// State is the current state.
	State S
	// Version is the aggregate version (optimistic locking).
	Version int64
	// auditLogger records state transitions for audit purposes.
	auditLogger AuditLogger

	// transitions are stored in a slice for stability (pointers to slice elements are stable)
	transitions  []Transition[S, E]
	transitionsMap map[transitionKey[S, E]]int // index into transitions slice
	initialState S
}

type transitionKey[S comparable, E comparable] struct {
	from  S
	event E
}

// NewStateMachineOf creates a new generic state machine.
// entityType: the type of entity (e.g., "Task")
// entityID: the unique identifier of the entity
// initialState: the starting state
// auditLogger: optional audit logger (nil uses NoOpAuditLogger)
func NewStateMachineOf[S comparable, E comparable](entityType, entityID string, initialState S, auditLogger AuditLogger) *StateMachine[S, E] {
	if auditLogger == nil {
		auditLogger = NoOpAuditLogger{}
	}
	return &StateMachine[S, E]{
		EntityType:   entityType,
		EntityID:     entityID,
		State:        initialState,
		Version:      1,
		transitions:  nil,
		transitionsMap: make(map[transitionKey[S, E]]int),
		initialState: initialState,
		auditLogger:  auditLogger,
	}
}

// AddTransition registers a declarative transition rule.
// Returns a pointer to the transition for fluent chaining with WithGuard/WithAction.
// The returned pointer remains valid until the next call to AddTransition.
func (sm *StateMachine[S, E]) AddTransition(from, to S, event E) *Transition[S, E] {
	// Check if transition already exists
	key := transitionKey[S, E]{from: from, event: event}
	if idx, exists := sm.transitionsMap[key]; exists {
		// Return pointer to existing transition
		return &sm.transitions[idx]
	}

	// Create new transition
	t := Transition[S, E]{
		From:    from,
		To:      to,
		Event:   event,
		Guards:  nil,
		Actions: nil,
	}
	idx := len(sm.transitions)
	sm.transitions = append(sm.transitions, t)
	sm.transitionsMap[key] = idx
	// Return pointer to the slice element (stable in Go)
	return &sm.transitions[idx]
}

// WithGuard adds a guard condition to a transition.
// Multiple guards are AND-combined: all must pass for transition to proceed.
// Returns the Transition for fluent chaining.
func (t *Transition[S, E]) WithGuard(guard Guard[S, E]) *Transition[S, E] {
	t.Guards = append(t.Guards, guard)
	return t
}

// WithAction adds an action to a transition.
// Multiple actions are executed in order: all must succeed for transition to proceed.
// Returns the Transition for fluent chaining.
func (t *Transition[S, E]) WithAction(action Action[S, E]) *Transition[S, E] {
	t.Actions = append(t.Actions, action)
	return t
}

// CanTransition reports whether the given event can trigger a valid transition
// from the current state (without evaluating guards).
func (sm *StateMachine[S, E]) CanTransition(event E) bool {
	key := transitionKey[S, E]{from: sm.State, event: event}
	_, exists := sm.transitionsMap[key]
	return exists
}

// TransitionAllowed checks if a transition from the current state with the given event is allowed.
// It evaluates all guards but does NOT execute actions or update state.
// Returns (allowed, error):
//   - allowed=true: transition is permitted (guards passed)
//   - allowed=false: transition is not allowed (guards blocked)
//   - error: guard evaluation failed
func (sm *StateMachine[S, E]) TransitionAllowed(event E) (bool, error) {
	key := transitionKey[S, E]{from: sm.State, event: event}
	idx, exists := sm.transitionsMap[key]
	if !exists {
		return false, nil
	}
	transition := sm.transitions[idx]

	// Evaluate all guards (AND composition)
	for _, guard := range transition.Guards {
		allowed, err := guard(sm)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

// Trigger executes the transition for the given event.
// It evaluates guards, executes actions, updates state, and increments version.
// Returns the new state and an error if:
//   - transition doesn't exist
//   - guard evaluation failed
//   - guard blocked the transition
//   - action execution failed
func (sm *StateMachine[S, E]) Trigger(event E) (S, error) {
	key := transitionKey[S, E]{from: sm.State, event: event}
	idx, exists := sm.transitionsMap[key]
	if !exists {
		sm.auditLogger.LogStateTransition(sm.EntityType, sm.EntityID, sm.State, sm.State, fmt.Sprintf("%v", event), sm.Version, false, nil)
		return sm.State, NewL2StateTransitionFailedError(sm.EntityType, sm.EntityID, fmt.Sprintf("%v", sm.State), fmt.Sprintf("%v", sm.State), fmt.Sprintf("%v", event), "transition not found")
	}
	transition := sm.transitions[idx]

	// Evaluate all guards (AND composition)
	for _, guard := range transition.Guards {
		allowed, err := guard(sm)
		if err != nil {
			sm.auditLogger.LogStateTransition(sm.EntityType, sm.EntityID, sm.State, sm.State, fmt.Sprintf("%v", event), sm.Version, false, err)
			return sm.State, err
		}
		if !allowed {
			sm.auditLogger.LogStateTransition(sm.EntityType, sm.EntityID, sm.State, sm.State, fmt.Sprintf("%v", event), sm.Version, false, nil)
			return sm.State, NewL2StateTransitionFailedError(sm.EntityType, sm.EntityID, fmt.Sprintf("%v", sm.State), fmt.Sprintf("%v", transition.To), fmt.Sprintf("%v", event), "guard blocked")
		}
	}

	// Execute all actions (chain composition - all must succeed)
	for _, action := range transition.Actions {
		if err := action(sm); err != nil {
			sm.auditLogger.LogStateTransition(sm.EntityType, sm.EntityID, sm.State, sm.State, fmt.Sprintf("%v", event), sm.Version, false, err)
			return sm.State, err
		}
	}

	// Update state and version
	oldState := sm.State
	sm.State = transition.To
	sm.Version++

	sm.auditLogger.LogStateTransition(sm.EntityType, sm.EntityID, oldState, sm.State, fmt.Sprintf("%v", event), sm.Version, true, nil)

	return sm.State, nil
}

// CurrentState returns the current state.
func (sm *StateMachine[S, E]) CurrentState() S {
	return sm.State
}

// CurrentVersion returns the current version.
func (sm *StateMachine[S, E]) CurrentVersion() int64 {
	return sm.Version
}

// Reset returns the state machine to its initial state.
func (sm *StateMachine[S, E]) Reset() {
	sm.State = sm.initialState
	sm.Version = 1
}

// Transitions returns all defined transitions from the current state.
func (sm *StateMachine[S, E]) Transitions() []Transition[S, E] {
	var result []Transition[S, E]
	for key, transitionIdx := range sm.transitionsMap {
		if key.from == sm.State {
			result = append(result, sm.transitions[transitionIdx])
		}
	}
	return result
}

// AvailableEvents returns the list of events that can be triggered from the current state.
func (sm *StateMachine[S, E]) AvailableEvents() []E {
	var events []E
	seen := make(map[E]bool)
	for key := range sm.transitionsMap {
		if key.from == sm.State && !seen[key.event] {
			events = append(events, key.event)
			seen[key.event] = true
		}
	}
	return events
}

// Validate validates the state machine is well-formed.
func (sm *StateMachine[S, E]) Validate() error {
	if sm == nil {
		return fmt.Errorf("state machine is nil")
	}
	if sm.EntityID == "" {
		return NewL2ParamValidationError("entity_id", "cannot be empty")
	}
	if sm.EntityType == "" {
		return NewL2ParamValidationError("entity_type", "cannot be empty")
	}
	return nil
}

// SetAuditLogger sets the audit logger (for testing or runtime changes).
func (sm *StateMachine[S, E]) SetAuditLogger(logger AuditLogger) {
	if logger != nil {
		sm.auditLogger = logger
	}
}

// ----------------------------------------------------------------------------
// Guard Factories (reusable, generic)
// ----------------------------------------------------------------------------

// GuardFunc is a simple guard that returns a fixed result.
func GuardFunc[S comparable, E comparable](allowed bool, guardErr error) Guard[S, E] {
	return func(sm *StateMachine[S, E]) (bool, error) {
		return allowed, guardErr
	}
}

// GuardAnd returns a guard that AND-combines multiple guards.
// All guards must pass for the composite to pass.
func GuardAnd[S comparable, E comparable](guards ...Guard[S, E]) Guard[S, E] {
	return func(sm *StateMachine[S, E]) (bool, error) {
		for _, g := range guards {
			allowed, err := g(sm)
			if err != nil {
				return false, err
			}
			if !allowed {
				return false, nil
			}
		}
		return true, nil
	}
}

// GuardOr returns a guard that OR-combines multiple guards.
// At least one guard must pass for the composite to pass.
func GuardOr[S comparable, E comparable](guards ...Guard[S, E]) Guard[S, E] {
	return func(sm *StateMachine[S, E]) (bool, error) {
		for _, g := range guards {
			allowed, err := g(sm)
			if err != nil {
				return false, err
			}
			if allowed {
				return true, nil
			}
		}
		return false, nil
	}
}

// GuardNot returns a guard that negates another guard.
func GuardNot[S comparable, E comparable](g Guard[S, E]) Guard[S, E] {
	return func(sm *StateMachine[S, E]) (bool, error) {
		allowed, err := g(sm)
		if err != nil {
			return false, err
		}
		return !allowed, nil
	}
}

// GuardFieldNotZero is a guard that checks a field is not its zero value.
// getValue is a closure that extracts the field value from the state machine.
func GuardFieldNotZero[S comparable, E comparable](fieldName string, getValue func(*StateMachine[S, E]) any) Guard[S, E] {
	return func(sm *StateMachine[S, E]) (bool, error) {
		value := getValue(sm)
		if value == nil {
			return false, NewL2BusinessRuleViolatedError("field_not_zero", map[string]any{
				"field": fieldName,
			})
		}
		switch v := value.(type) {
		case string:
			if v == "" {
				return false, NewL2BusinessRuleViolatedError("field_not_zero", map[string]any{
					"field": fieldName,
				})
			}
		case int:
			if v == 0 {
				return false, NewL2BusinessRuleViolatedError("field_not_zero", map[string]any{
					"field": fieldName,
				})
			}
		case int64:
			if v == 0 {
				return false, NewL2BusinessRuleViolatedError("field_not_zero", map[string]any{
					"field": fieldName,
				})
			}
		case float64:
			if v == 0 {
				return false, NewL2BusinessRuleViolatedError("field_not_zero", map[string]any{
					"field": fieldName,
				})
			}
		}
		return true, nil
	}
}

// GuardVersionMatch is a guard that checks the aggregate version matches expected.
// Useful for optimistic locking scenarios.
func GuardVersionMatch[S comparable, E comparable](expected int64) Guard[S, E] {
	return func(sm *StateMachine[S, E]) (bool, error) {
		if sm.Version != expected {
			return false, NewL2OptimisticLockError(sm.EntityType, sm.EntityID, expected, sm.Version)
		}
		return true, nil
	}
}

// GuardVersionGreaterThan is a guard that checks the aggregate version is greater than expected.
func GuardVersionGreaterThan[S comparable, E comparable](minVersion int64) Guard[S, E] {
	return func(sm *StateMachine[S, E]) (bool, error) {
		if sm.Version <= minVersion {
			return false, NewL2BusinessRuleViolatedError("version_too_low", map[string]any{
				"min_version":    minVersion,
				"actual_version": sm.Version,
			})
		}
		return true, nil
	}
}

// GuardStateIn is a guard that checks the current state is one of the allowed states.
func GuardStateIn[S comparable, E comparable](allowedStates ...S) Guard[S, E] {
	return func(sm *StateMachine[S, E]) (bool, error) {
		for _, s := range allowedStates {
			if sm.State == s {
				return true, nil
			}
		}
		return false, NewL2InvalidStateError(sm.EntityType, fmt.Sprintf("%v", sm.State), "state not in allowed list")
	}
}

// GuardStateNotIn is a guard that checks the current state is NOT one of the forbidden states.
func GuardStateNotIn[S comparable, E comparable](forbiddenStates ...S) Guard[S, E] {
	return func(sm *StateMachine[S, E]) (bool, error) {
		for _, s := range forbiddenStates {
			if sm.State == s {
				return false, NewL2InvalidStateError(sm.EntityType, fmt.Sprintf("%v", sm.State), "state in forbidden list")
			}
		}
		return true, nil
	}
}

// ----------------------------------------------------------------------------
// Action Factories (reusable, generic)
// ----------------------------------------------------------------------------

// ActionFunc creates an action from a simple function.
func ActionFunc[S comparable, E comparable](fn func(*StateMachine[S, E]) error) Action[S, E] {
	return fn
}

// ActionChain returns an action that executes multiple actions in sequence.
// All actions must succeed for the chain to succeed.
func ActionChain[S comparable, E comparable](actions ...Action[S, E]) Action[S, E] {
	return func(sm *StateMachine[S, E]) error {
		for _, a := range actions {
			if err := a(sm); err != nil {
				return err
			}
		}
		return nil
	}
}

// ActionIncrementVersion is an action that increments the version.
// Note: Trigger already increments version after successful transition.
// This action is for explicit version increments within action chains.
func ActionIncrementVersion[S comparable, E comparable]() Action[S, E] {
	return func(sm *StateMachine[S, E]) error {
		sm.Version++
		return nil
	}
}

// ActionRecordAudit is an action that records an audit log entry via the state machine's logger.
func ActionRecordAudit[S comparable, E comparable](action, message string) Action[S, E] {
	return func(sm *StateMachine[S, E]) error {
		sm.auditLogger.LogStateTransition(sm.EntityType, sm.EntityID, sm.State, sm.State, action, sm.Version, true, nil)
		return nil
	}
}

// ----------------------------------------------------------------------------
// Generic Error Constructor (for non-TaskStatus state types)
// ----------------------------------------------------------------------------

// NewL2StateTransitionFailedError creates an error for a failed state transition.
// This is a generic version that works with any state type (passed as strings).
func NewL2StateTransitionFailedError(entityType, entityID, fromState, toState, event, reason string) *AppError {
	return &AppError{
		Code:    CodeL2InvalidStateTransition,
		Message: fmt.Sprintf("state transition failed for %s %s: %s -> %s via %s (%s)", entityType, entityID, fromState, toState, event, reason),
		Layer:   LayerDomain,
		Details: map[string]any{
			"entity_type": entityType,
			"entity_id":   entityID,
			"from_state":  fromState,
			"to_state":    toState,
			"event":       event,
			"reason":      reason,
		},
	}
}

// ----------------------------------------------------------------------------
// Task State Machine Definitions
// ----------------------------------------------------------------------------

// TaskStateMachineDefinition defines the declarative rules for Task state transitions.
// Based on Cloud-Agent-Platform.md §三: 9 states with transitions.
var TaskStateMachineDefinition = []Transition[TaskStatus, string]{
	// pending -> decomposing (start decomposition)
	{From: TaskStatusPending, To: TaskStatusDecomposing, Event: "StartDecomposition"},
	// pending -> cancelled (user cancel)
	{From: TaskStatusPending, To: TaskStatusCancelled, Event: "Cancel"},

	// decomposing -> dispatched (decomposition complete)
	{From: TaskStatusDecomposing, To: TaskStatusDispatched, Event: "DecompositionComplete"},
	// decomposing -> failed (decomposition failed)
	{From: TaskStatusDecomposing, To: TaskStatusFailed, Event: "DecompositionFailed"},
	// decomposing -> cancelled (user cancel)
	{From: TaskStatusDecomposing, To: TaskStatusCancelled, Event: "Cancel"},

	// dispatched -> running (agent starts execution)
	{From: TaskStatusDispatched, To: TaskStatusRunning, Event: "StartExecution"},
	// dispatched -> failed (no available agent)
	{From: TaskStatusDispatched, To: TaskStatusFailed, Event: "NoAgentAvailable"},
	// dispatched -> cancelled (user cancel)
	{From: TaskStatusDispatched, To: TaskStatusCancelled, Event: "Cancel"},

	// running -> reviewing (agent completes)
	{From: TaskStatusRunning, To: TaskStatusReviewing, Event: "CompleteExecution"},
	// running -> failed (agent error)
	{From: TaskStatusRunning, To: TaskStatusFailed, Event: "ExecutionFailed"},
	// running -> confirming (needs user confirmation)
	{From: TaskStatusRunning, To: TaskStatusConfirming, Event: "RequestConfirmation"},
	// running -> cancelled (user cancel)
	{From: TaskStatusRunning, To: TaskStatusCancelled, Event: "Cancel"},

	// reviewing -> completed (review passed)
	{From: TaskStatusReviewing, To: TaskStatusCompleted, Event: "ReviewPassed"},
	// reviewing -> running (review failed, re-execute)
	{From: TaskStatusReviewing, To: TaskStatusRunning, Event: "ReviewFailedRetry"},
	// reviewing -> failed (review failed, cannot fix)
	{From: TaskStatusReviewing, To: TaskStatusFailed, Event: "ReviewFailedFatal"},

	// confirming -> running (user approved)
	{From: TaskStatusConfirming, To: TaskStatusRunning, Event: "UserApproved"},
	// confirming -> failed (user rejected)
	{From: TaskStatusConfirming, To: TaskStatusFailed, Event: "UserRejected"},
	// confirming -> failed (timeout)
	{From: TaskStatusConfirming, To: TaskStatusFailed, Event: "ConfirmationTimeout"},
	// confirming -> cancelled (user cancel)
	{From: TaskStatusConfirming, To: TaskStatusCancelled, Event: "Cancel"},
}

// SubtaskStateMachineDefinition defines the declarative rules for Subtask state transitions.
// Subtasks follow a subset of Task states appropriate for sub-work items.
var SubtaskStateMachineDefinition = []Transition[TaskStatus, string]{
	// pending -> dispatched (assigned to agent)
	{From: TaskStatusPending, To: TaskStatusDispatched, Event: "Assign"},
	// pending -> cancelled (user cancel)
	{From: TaskStatusPending, To: TaskStatusCancelled, Event: "Cancel"},

	// dispatched -> running (agent starts)
	{From: TaskStatusDispatched, To: TaskStatusRunning, Event: "StartExecution"},
	// dispatched -> failed (no agent available)
	{From: TaskStatusDispatched, To: TaskStatusFailed, Event: "NoAgentAvailable"},
	// dispatched -> cancelled (user cancel)
	{From: TaskStatusDispatched, To: TaskStatusCancelled, Event: "Cancel"},

	// running -> reviewing (execution complete)
	{From: TaskStatusRunning, To: TaskStatusReviewing, Event: "CompleteExecution"},
	// running -> failed (execution error)
	{From: TaskStatusRunning, To: TaskStatusFailed, Event: "ExecutionFailed"},
	// running -> cancelled (user cancel)
	{From: TaskStatusRunning, To: TaskStatusCancelled, Event: "Cancel"},

	// reviewing -> completed (review passed)
	{From: TaskStatusReviewing, To: TaskStatusCompleted, Event: "ReviewPassed"},
	// reviewing -> running (review failed, retry)
	{From: TaskStatusReviewing, To: TaskStatusRunning, Event: "ReviewFailedRetry"},
	// reviewing -> failed (review failed, fatal)
	{From: TaskStatusReviewing, To: TaskStatusFailed, Event: "ReviewFailedFatal"},
}

// ----------------------------------------------------------------------------
// Task-Specific State Machine (backward compatibility + enhancements)
// Uses the generic framework with TaskStatus as the state type
// ----------------------------------------------------------------------------

// TaskTransition is an alias for Transition specialized to TaskStatus and string events.
type TaskTransition = Transition[TaskStatus, string]

// TaskGuard is an alias for Guard specialized to TaskStatus and string events.
type TaskGuard = Guard[TaskStatus, string]

// TaskAction is an alias for Action specialized to TaskStatus and string events.
type TaskAction = Action[TaskStatus, string]

// TaskStateMachine is a specialized state machine for Task entities.
// It wraps the generic StateMachine[TaskStatus, string] with Task-specific helpers.
type TaskStateMachine = StateMachine[TaskStatus, string]

// BuildTaskStateMachineWithLogger builds a pre-configured Task state machine with audit logging.
func BuildTaskStateMachineWithLogger(entityID string, logger AuditLogger) *TaskStateMachine {
	sm := NewStateMachineOf[TaskStatus, string]("Task", entityID, TaskStatusPending, logger)
	for _, t := range TaskStateMachineDefinition {
		sm.AddTransition(t.From, t.To, t.Event)
	}
	return sm
}

// BuildTaskStateMachine builds a pre-configured Task state machine (no audit logging).
func BuildTaskStateMachine(entityID string) *TaskStateMachine {
	return BuildTaskStateMachineWithLogger(entityID, NoOpAuditLogger{})
}

// BuildSubtaskStateMachineWithLogger builds a pre-configured Subtask state machine.
func BuildSubtaskStateMachineWithLogger(entityID string, logger AuditLogger) *TaskStateMachine {
	sm := NewStateMachineOf[TaskStatus, string]("Subtask", entityID, TaskStatusPending, logger)
	for _, t := range SubtaskStateMachineDefinition {
		sm.AddTransition(t.From, t.To, t.Event)
	}
	return sm
}

// BuildSubtaskStateMachine builds a pre-configured Subtask state machine.
func BuildSubtaskStateMachine(entityID string) *TaskStateMachine {
	return BuildSubtaskStateMachineWithLogger(entityID, NoOpAuditLogger{})
}

// ----------------------------------------------------------------------------
// State Machine Validation Utilities
// ----------------------------------------------------------------------------

// ValidateStateMachine checks that a Task state machine is well-formed.
func ValidateStateMachine(sm *TaskStateMachine) error {
	if sm == nil {
		return fmt.Errorf("state machine is nil")
	}
	if sm.EntityID == "" {
		return NewL2ParamValidationError("entity_id", "cannot be empty")
	}
	if sm.EntityType == "" {
		return NewL2ParamValidationError("entity_type", "cannot be empty")
	}
	// Check for terminal states that should have no outgoing transitions
	terminalStates := []TaskStatus{TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled}
	for _, terminal := range terminalStates {
		for key := range sm.transitionsMap {
			if key.from == terminal {
				return NewL2StateTransitionFailedError(sm.EntityType, sm.EntityID, string(terminal), fmt.Sprintf("%v_invalid", terminal), "", "terminal state has outgoing transitions")
			}
		}
	}
	return nil
}

// HasOutgoingTransitions reports whether the given TaskStatus has any outgoing transitions.
func HasOutgoingTransitions(state TaskStatus, transitions []TaskTransition) bool {
	for _, t := range transitions {
		if t.From == state {
			return true
		}
	}
	return false
}

// FindTransition looks up a Task transition by from state and event.
func FindTransition(from TaskStatus, event string, transitions []TaskTransition) *TaskTransition {
	for i := range transitions {
		if transitions[i].From == from && transitions[i].Event == event {
			return &transitions[i]
		}
	}
	return nil
}

// IsTerminalState reports whether the given state is a terminal (final) state.
func IsTerminalState(state TaskStatus) bool {
	return state == TaskStatusCompleted || state == TaskStatusFailed || state == TaskStatusCancelled
}

// AllTaskStates returns all defined TaskStatus values.
func AllTaskStates() []TaskStatus {
	return []TaskStatus{
		TaskStatusPending,
		TaskStatusDecomposing,
		TaskStatusDispatched,
		TaskStatusRunning,
		TaskStatusReviewing,
		TaskStatusConfirming,
		TaskStatusCompleted,
		TaskStatusFailed,
		TaskStatusCancelled,
	}
}

// AllTaskEvents returns all defined Task events.
func AllTaskEvents() []string {
	events := make([]string, 0, 20)
	seen := make(map[string]bool)
	for _, t := range TaskStateMachineDefinition {
		if !seen[t.Event] {
			events = append(events, t.Event)
			seen[t.Event] = true
		}
	}
	slices.Sort(events)
	return events
}
