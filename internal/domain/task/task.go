// Package task implements the Task and Subtask domain entities with
// 9-state declarative state machine, domain event collection, and business invariants.
// This package is part of L2-Domain layer and has ZERO external dependencies.
package task

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

// ----------------------------------------------------------------------------
// ID generation
// ----------------------------------------------------------------------------

// randEntropy wraps crypto/rand.Reader so it satisfies ulid.Entropy.
type randEntropy struct{}

func (randEntropy) Read(p []byte) (n int, err error) {
	return rand.Read(p)
}

// NewULID generates a new ULID string using crypto/rand as the entropy source.
func NewULID() string {
	entropy := ulid.Monotonic(randEntropy{}, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// ----------------------------------------------------------------------------
// TaskStatus — 9 states for Task/Subtask
// ----------------------------------------------------------------------------

// TaskStatus represents the state of a Task or Subtask.
type TaskStatus string

const (
	TaskStatusPending     TaskStatus = "pending"
	TaskStatusSubmitted  TaskStatus = "submitted"  // Task submitted, awaiting decomposition
	TaskStatusDecomposing TaskStatus = "decomposing" // Decomposing task into subtasks
	TaskStatusAssigned    TaskStatus = "assigned"   // Subtasks assigned to agents
	TaskStatusRunning     TaskStatus = "running"    // Agent executing
	TaskStatusReviewing   TaskStatus = "reviewing"  // Reviewing results
	TaskStatusCompleted   TaskStatus = "completed"  // Terminal: success
	TaskStatusFailed      TaskStatus = "failed"     // Terminal: failed
	TaskStatusCancelled   TaskStatus = "cancelled"  // Terminal: cancelled by user
)

// String returns the string representation of TaskStatus.
func (s TaskStatus) String() string { return string(s) }

// IsValid reports whether s is a known TaskStatus.
func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusPending, TaskStatusSubmitted, TaskStatusDecomposing,
		TaskStatusAssigned, TaskStatusRunning, TaskStatusReviewing,
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
type AgentRole string

const (
	AgentRoleObserver   AgentRole = "observer"
	AgentRoleStrategist AgentRole = "strategist"
	AgentRoleExecutor   AgentRole = "executor"
	AgentRoleGuardian   AgentRole = "guardian"
	AgentRoleTester     AgentRole = "tester"
	AgentRoleResearcher AgentRole = "researcher"
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
// Domain Event
// ----------------------------------------------------------------------------

// DomainEvent represents a domain event for the Outbox pattern.
type DomainEvent struct {
	EventID        string    `json:"event_id"`
	AggregateType  string    `json:"aggregate_type"`
	AggregateID    string    `json:"aggregate_id"`
	EventType      string    `json:"event_type"`
	Payload        []byte    `json:"payload"`
	OccurredAt     time.Time `json:"occurred_at"`
	IdempotencyKey string    `json:"idempotency_key"`
	Version        int       `json:"version"`
}

// NewDomainEvent creates a new DomainEvent.
func NewDomainEvent(aggregateType, aggregateID, eventType string, payload []byte, version int) *DomainEvent {
	return &DomainEvent{
		EventID:        NewULID(),
		AggregateType:  aggregateType,
		AggregateID:    aggregateID,
		EventType:      eventType,
		Payload:        payload,
		OccurredAt:     time.Now().UTC(),
		IdempotencyKey: fmt.Sprintf("%s:%s:%d", aggregateID, eventType, version),
		Version:        version,
	}
}

// ----------------------------------------------------------------------------
// Aggregate Root Base
// ----------------------------------------------------------------------------

// AggregateRoot is the base for all aggregate roots.
// It collects domain events that will be published via the Outbox pattern.
type AggregateRoot struct {
	ID      string
	Version int
	events  []*DomainEvent
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

// ----------------------------------------------------------------------------
// State Machine Types
// ----------------------------------------------------------------------------

// Transition represents a state transition rule.
type Transition struct {
	From    TaskStatus
	To      TaskStatus
	Event   string
	Guards  []Guard
	Actions []Action
}

// Guard is a function that evaluates a condition before allowing a transition.
type Guard func(sm *StateMachine) (bool, error)

// Action is a function that executes during a transition.
type Action func(sm *StateMachine) error

// StateMachine holds the declarative state machine definition and current state.
type StateMachine struct {
	EntityID      string
	EntityType    string
	State         TaskStatus
	Version       int64
	transitions   map[transitionKey]Transition
	initialState  TaskStatus
}

type transitionKey struct {
	from  TaskStatus
	event string
}

// NewStateMachine creates a new state machine for an entity.
func NewStateMachine(entityType, entityID string, initialState TaskStatus) *StateMachine {
	return &StateMachine{
		EntityType:   entityType,
		EntityID:     entityID,
		State:        initialState,
		Version:      1,
		transitions:  make(map[transitionKey]Transition),
		initialState: initialState,
	}
}

// AddTransition registers a declarative transition rule.
func (sm *StateMachine) AddTransition(from, to TaskStatus, event string) *StateMachine {
	sm.transitions[transitionKey{from: from, event: event}] = Transition{
		From:   from,
		To:     to,
		Event:  event,
		Guards: nil,
		Actions: nil,
	}
	return sm
}

// WithGuard adds a guard condition to the most recently added transition.
func (sm *StateMachine) WithGuard(guard Guard) *StateMachine {
	key := sm.findLastKey()
	if key != nil {
		t := sm.transitions[*key]
		t.Guards = append(t.Guards, guard)
		sm.transitions[*key] = t
	}
	return sm
}

// WithAction adds an action to the most recently added transition.
func (sm *StateMachine) WithAction(action Action) *StateMachine {
	key := sm.findLastKey()
	if key != nil {
		t := sm.transitions[*key]
		t.Actions = append(t.Actions, action)
		sm.transitions[*key] = t
	}
	return sm
}

func (sm *StateMachine) findLastKey() *transitionKey {
	for k := range sm.transitions {
		return &k
	}
	return nil
}

// CanTransition reports whether the given event can trigger a valid transition.
func (sm *StateMachine) CanTransition(event string) bool {
	key := transitionKey{from: sm.State, event: event}
	_, exists := sm.transitions[key]
	return exists
}

// Trigger executes the transition for the given event.
func (sm *StateMachine) Trigger(event string) (TaskStatus, error) {
	key := transitionKey{from: sm.State, event: event}
	transition, exists := sm.transitions[key]
	if !exists {
		return sm.State, fmt.Errorf("invalid state transition for %s: %s -> (via %s)", sm.EntityType, sm.State, event)
	}

	// Evaluate all guards
	for _, guard := range transition.Guards {
		allowed, err := guard(sm)
		if err != nil {
			return sm.State, err
		}
		if !allowed {
			return sm.State, fmt.Errorf("guard rejected transition for %s: %s -> %s", sm.EntityType, sm.State, transition.To)
		}
	}

	// Execute all actions before state change
	for _, action := range transition.Actions {
		if err := action(sm); err != nil {
			return sm.State, err
		}
	}

	// Update state and version
	sm.State = transition.To
	sm.Version++

	return sm.State, nil
}

// Reset returns the state machine to its initial state.
func (sm *StateMachine) Reset() {
	sm.State = sm.initialState
	sm.Version = 1
}

// AvailableEvents returns the list of events that can be triggered from the current state.
func (sm *StateMachine) AvailableEvents() []string {
	var events []string
	for key := range sm.transitions {
		if key.from == sm.State {
			events = append(events, key.event)
		}
	}
	return events
}

// ----------------------------------------------------------------------------
// Task State Machine Definition (9 states)
// pending → submitted → decomposing → assigned → running → reviewing → completed/failed/cancelled
// ----------------------------------------------------------------------------

// TaskStateMachineDefinition defines the declarative rules for Task state transitions.
var TaskStateMachineDefinition = []Transition{
	// pending → submitted (submit task)
	{From: TaskStatusPending, To: TaskStatusSubmitted, Event: "Submit"},
	// pending → cancelled (user cancel)
	{From: TaskStatusPending, To: TaskStatusCancelled, Event: "Cancel"},

	// submitted → decomposing (start decomposition)
	{From: TaskStatusSubmitted, To: TaskStatusDecomposing, Event: "StartDecomposition"},
	// submitted → cancelled (user cancel)
	{From: TaskStatusSubmitted, To: TaskStatusCancelled, Event: "Cancel"},

	// decomposing → assigned (decomposition complete, subtasks assigned)
	{From: TaskStatusDecomposing, To: TaskStatusAssigned, Event: "DecompositionComplete"},
	// decomposing → failed (decomposition failed)
	{From: TaskStatusDecomposing, To: TaskStatusFailed, Event: "DecompositionFailed"},
	// decomposing → cancelled (user cancel)
	{From: TaskStatusDecomposing, To: TaskStatusCancelled, Event: "Cancel"},

	// assigned → running (agent starts execution)
	{From: TaskStatusAssigned, To: TaskStatusRunning, Event: "StartExecution"},
	// assigned → failed (no available agent)
	{From: TaskStatusAssigned, To: TaskStatusFailed, Event: "NoAgentAvailable"},
	// assigned → cancelled (user cancel)
	{From: TaskStatusAssigned, To: TaskStatusCancelled, Event: "Cancel"},

	// running → reviewing (execution complete, start review)
	{From: TaskStatusRunning, To: TaskStatusReviewing, Event: "CompleteExecution"},
	// running → failed (execution error)
	{From: TaskStatusRunning, To: TaskStatusFailed, Event: "ExecutionFailed"},
	// running → cancelled (user cancel)
	{From: TaskStatusRunning, To: TaskStatusCancelled, Event: "Cancel"},

	// reviewing → completed (review passed)
	{From: TaskStatusReviewing, To: TaskStatusCompleted, Event: "ReviewPassed"},
	// reviewing → running (review failed, retry)
	{From: TaskStatusReviewing, To: TaskStatusRunning, Event: "ReviewFailedRetry"},
	// reviewing → failed (review failed, fatal)
	{From: TaskStatusReviewing, To: TaskStatusFailed, Event: "ReviewFailedFatal"},
	// reviewing → cancelled (user cancel)
	{From: TaskStatusReviewing, To: TaskStatusCancelled, Event: "Cancel"},
}

// BuildTaskStateMachine builds a pre-configured Task state machine.
func BuildTaskStateMachine(entityID string) *StateMachine {
	sm := NewStateMachine("Task", entityID, TaskStatusPending)
	for _, t := range TaskStateMachineDefinition {
		sm.AddTransition(t.From, t.To, t.Event)
	}
	return sm
}

// ----------------------------------------------------------------------------
// Subtask State Machine Definition (subset of Task states)
// ----------------------------------------------------------------------------

// SubtaskStateMachineDefinition defines the declarative rules for Subtask state transitions.
var SubtaskStateMachineDefinition = []Transition{
	// pending → assigned (assigned to agent)
	{From: TaskStatusPending, To: TaskStatusAssigned, Event: "Assign"},
	// pending → cancelled (user cancel)
	{From: TaskStatusPending, To: TaskStatusCancelled, Event: "Cancel"},

	// assigned → running (agent starts)
	{From: TaskStatusAssigned, To: TaskStatusRunning, Event: "StartExecution"},
	// assigned → failed (no agent available)
	{From: TaskStatusAssigned, To: TaskStatusFailed, Event: "NoAgentAvailable"},
	// assigned → cancelled (user cancel)
	{From: TaskStatusAssigned, To: TaskStatusCancelled, Event: "Cancel"},

	// running → reviewing (execution complete)
	{From: TaskStatusRunning, To: TaskStatusReviewing, Event: "CompleteExecution"},
	// running → failed (execution error)
	{From: TaskStatusRunning, To: TaskStatusFailed, Event: "ExecutionFailed"},
	// running → cancelled (user cancel)
	{From: TaskStatusRunning, To: TaskStatusCancelled, Event: "Cancel"},

	// reviewing → completed (review passed)
	{From: TaskStatusReviewing, To: TaskStatusCompleted, Event: "ReviewPassed"},
	// reviewing → running (review failed, retry)
	{From: TaskStatusReviewing, To: TaskStatusRunning, Event: "ReviewFailedRetry"},
	// reviewing → failed (review failed, fatal)
	{From: TaskStatusReviewing, To: TaskStatusFailed, Event: "ReviewFailedFatal"},
	// reviewing → cancelled (user cancel)
	{From: TaskStatusReviewing, To: TaskStatusCancelled, Event: "Cancel"},
}

// BuildSubtaskStateMachine builds a pre-configured Subtask state machine.
func BuildSubtaskStateMachine(entityID string) *StateMachine {
	sm := NewStateMachine("Subtask", entityID, TaskStatusPending)
	for _, t := range SubtaskStateMachineDefinition {
		sm.AddTransition(t.From, t.To, t.Event)
	}
	return sm
}

// ----------------------------------------------------------------------------
// IsTerminalState reports whether the given state is a terminal state.
// ----------------------------------------------------------------------------

func IsTerminalState(state TaskStatus) bool {
	return state == TaskStatusCompleted || state == TaskStatusFailed || state == TaskStatusCancelled
}

// ----------------------------------------------------------------------------
// Task Aggregate Root
// ----------------------------------------------------------------------------

// Task represents a task aggregate root.
type Task struct {
	AggregateRoot

	// Goal is the task objective
	Goal string
	// Status is the current task status
	Status TaskStatus
	// Priority is 0-9, default 5
	Priority int
	// RepositoryURL is the Git repository URL
	RepositoryURL string
	// BaseBranch is the branch to base work on
	BaseBranch string
	// ResultBranch is where results will be pushed: {base}/agent/{task-id}
	ResultBranch string
	// Constraints are the task constraints
	Constraints []string
	// VerificationCriteria are the acceptance criteria
	VerificationCriteria []string
	// AgentHint specifies agent preferences
	AgentHint *AgentHint
	// Progress is 0-100
	Progress float64
	// TokensUsed is cumulative token consumption
	TokensUsed int
	// EstimatedCost is the estimated cost in yuan
	EstimatedCost float64
	// AgentsUsed is the number of agents used
	AgentsUsed int
	// ClientID is the submitter's client ID
	ClientID string
	// Tags are task tags
	Tags []string
	// CreatedAt is the creation timestamp
	CreatedAt time.Time
	// StartedAt is when execution started (nil if not started)
	StartedAt *time.Time
	// CompletedAt is when execution completed (nil if not completed)
	CompletedAt *time.Time
}

// AgentHint specifies preferences for agent execution.
type AgentHint struct {
	// Templates are suggested agent roles to use
	Templates []string
	// Model overrides the default LLM model
	Model string
	// MaxAgents is the maximum concurrent agents
	MaxAgents int
}

// NewTask creates a new Task aggregate with the given parameters.
func NewTask(id, goal, repositoryURL, baseBranch, clientID string) *Task {
	return &Task{
		AggregateRoot: AggregateRoot{
			ID:      id,
			Version: 1,
		},
		Goal:           goal,
		Status:         TaskStatusPending,
		Priority:       5,
		RepositoryURL:  repositoryURL,
		BaseBranch:     baseBranch,
		ResultBranch:   baseBranch + "/agent/" + id,
		Constraints:    []string{},
		VerificationCriteria: []string{},
		ClientID:       clientID,
		Tags:           []string{},
		CreatedAt:     time.Now().UTC(),
	}
}

// Submit submits the task for processing.
// Business rule: only pending tasks can be submitted.
func (t *Task) Submit() error {
	if t.Status != TaskStatusPending {
		return fmt.Errorf("task submit failed: task is in %s state, expected pending", t.Status)
	}

	// Record the event before transition
	event := NewDomainEvent("Task", t.ID, "TaskSubmittedV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("Submit")
}

// CanSubmit checks if the task can be submitted.
func (t *Task) CanSubmit() bool {
	return t.Status == TaskStatusPending
}

// StartDecomposition starts the task decomposition phase.
// Business rule: only submitted tasks can start decomposition.
func (t *Task) StartDecomposition() error {
	if t.Status != TaskStatusSubmitted {
		return fmt.Errorf("start decomposition failed: task is in %s state, expected submitted", t.Status)
	}

	event := NewDomainEvent("Task", t.ID, "TaskDecompositionStartedV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("StartDecomposition")
}

// CanStartDecomposition checks if decomposition can be started.
func (t *Task) CanStartDecomposition() bool {
	return t.Status == TaskStatusSubmitted
}

// CompleteDecomposition marks decomposition as complete and assigns subtasks.
// Business rule: only decomposing tasks can complete decomposition.
func (t *Task) CompleteDecomposition() error {
	if t.Status != TaskStatusDecomposing {
		return fmt.Errorf("complete decomposition failed: task is in %s state, expected decomposing", t.Status)
	}

	event := NewDomainEvent("Task", t.ID, "TaskDecompositionCompletedV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("DecompositionComplete")
}

// CanCompleteDecomposition checks if decomposition can be completed.
func (t *Task) CanCompleteDecomposition() bool {
	return t.Status == TaskStatusDecomposing
}

// FailDecomposition marks decomposition as failed.
// Business rule: only decomposing tasks can fail decomposition.
func (t *Task) FailDecomposition() error {
	if t.Status != TaskStatusDecomposing {
		return fmt.Errorf("fail decomposition failed: task is in %s state, expected decomposing", t.Status)
	}

	event := NewDomainEvent("Task", t.ID, "TaskDecompositionFailedV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("DecompositionFailed")
}

// Assign assigns the task to an agent for execution.
// Business rule: only assigned tasks can start execution.
func (t *Task) Assign() error {
	if t.Status != TaskStatusAssigned {
		return fmt.Errorf("assign failed: task is in %s state, expected assigned", t.Status)
	}

	event := NewDomainEvent("Task", t.ID, "TaskAssignedV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("StartExecution")
}

// CanAssign checks if the task can be assigned.
func (t *Task) CanAssign() bool {
	return t.Status == TaskStatusAssigned
}

// StartExecution marks the task as running.
// Business rule: only assigned tasks can start execution.
func (t *Task) StartExecution() error {
	if t.Status != TaskStatusAssigned {
		return fmt.Errorf("start execution failed: task is in %s state, expected assigned", t.Status)
	}

	now := time.Now().UTC()
	t.StartedAt = &now

	event := NewDomainEvent("Task", t.ID, "TaskExecutionStartedV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("StartExecution")
}

// CanStartExecution checks if execution can be started.
func (t *Task) CanStartExecution() bool {
	return t.Status == TaskStatusAssigned
}

// CompleteExecution marks execution as complete and starts review.
// Business rule: only running tasks can complete execution.
func (t *Task) CompleteExecution() error {
	if t.Status != TaskStatusRunning {
		return fmt.Errorf("complete execution failed: task is in %s state, expected running", t.Status)
	}

	event := NewDomainEvent("Task", t.ID, "TaskExecutionCompletedV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("CompleteExecution")
}

// CanCompleteExecution checks if execution can be completed.
func (t *Task) CanCompleteExecution() bool {
	return t.Status == TaskStatusRunning
}

// FailExecution marks the task execution as failed.
// Business rule: only running tasks can fail execution.
func (t *Task) FailExecution() error {
	if t.Status != TaskStatusRunning {
		return fmt.Errorf("fail execution failed: task is in %s state, expected running", t.Status)
	}

	now := time.Now().UTC()
	t.CompletedAt = &now

	event := NewDomainEvent("Task", t.ID, "TaskExecutionFailedV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("ExecutionFailed")
}

// FailNoAgentAvailable marks the task as failed due to no agent available.
// Business rule: only assigned tasks can fail with no agent.
func (t *Task) FailNoAgentAvailable() error {
	if t.Status != TaskStatusAssigned {
		return fmt.Errorf("fail no agent failed: task is in %s state, expected assigned", t.Status)
	}

	now := time.Now().UTC()
	t.CompletedAt = &now

	event := NewDomainEvent("Task", t.ID, "TaskNoAgentAvailableV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("NoAgentAvailable")
}

// StartReview starts the review phase.
// Business rule: only reviewing tasks can pass review.
func (t *Task) ReviewPassed() error {
	if t.Status != TaskStatusReviewing {
		return fmt.Errorf("review passed failed: task is in %s state, expected reviewing", t.Status)
	}

	now := time.Now().UTC()
	t.CompletedAt = &now

	event := NewDomainEvent("Task", t.ID, "TaskReviewPassedV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("ReviewPassed")
}

// CanReviewPassed checks if review can pass.
func (t *Task) CanReviewPassed() bool {
	return t.Status == TaskStatusReviewing
}

// ReviewFailedRetry indicates review failed but retry is possible.
// Business rule: only reviewing tasks can fail review with retry.
func (t *Task) ReviewFailedRetry() error {
	if t.Status != TaskStatusReviewing {
		return fmt.Errorf("review failed retry failed: task is in %s state, expected reviewing", t.Status)
	}

	event := NewDomainEvent("Task", t.ID, "TaskReviewFailedRetryV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("ReviewFailedRetry")
}

// ReviewFailedFatal indicates review failed fatally.
// Business rule: only reviewing tasks can fail review fatally.
func (t *Task) ReviewFailedFatal() error {
	if t.Status != TaskStatusReviewing {
		return fmt.Errorf("review failed fatal failed: task is in %s state, expected reviewing", t.Status)
	}

	now := time.Now().UTC()
	t.CompletedAt = &now

	event := NewDomainEvent("Task", t.ID, "TaskReviewFailedFatalV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("ReviewFailedFatal")
}

// Cancel cancels the task.
// Business rule: tasks can be cancelled from non-terminal states.
func (t *Task) Cancel() error {
	if t.Status.IsTerminal() {
		return fmt.Errorf("cancel failed: task is in terminal state %s", t.Status)
	}

	now := time.Now().UTC()
	t.CompletedAt = &now

	event := NewDomainEvent("Task", t.ID, "TaskCancelledV1", nil, int(t.Version))
	t.RecordEvent(event)

	return t.transitionTo("Cancel")
}

// CanCancel checks if the task can be cancelled.
func (t *Task) CanCancel() bool {
	return !t.Status.IsTerminal()
}

// transitionTo changes the task status using the state machine.
// It validates the transition is allowed and updates version.
func (t *Task) transitionTo(event string) error {
	sm := BuildTaskStateMachine(t.ID)
	sm.State = t.Status
	sm.Version = int64(t.Version)

	newState, err := sm.Trigger(event)
	if err != nil {
		return err
	}

	t.Status = newState
	t.Version = int(sm.Version)
	return nil
}

// AvailableEvents returns the list of events that can be triggered from the current state.
func (t *Task) AvailableEvents() []string {
	sm := BuildTaskStateMachine(t.ID)
	sm.State = t.Status
	return sm.AvailableEvents()
}

// ToTaskCreatedEventPayload creates the payload for TaskCreatedV1 event.
type TaskCreatedEventPayload struct {
	Goal            string   `json:"goal"`
	RepositoryURL   string   `json:"repository_url"`
	BaseBranch     string   `json:"base_branch"`
	ResultBranch   string   `json:"result_branch"`
	Constraints    []string `json:"constraints"`
	ClientID       string   `json:"client_id"`
	Tags           []string `json:"tags"`
}

// RecordTaskCreated records the TaskCreatedV1 domain event.
func (t *Task) RecordTaskCreated() error {
	payload := TaskCreatedEventPayload{
		Goal:            t.Goal,
		RepositoryURL:   t.RepositoryURL,
		BaseBranch:     t.BaseBranch,
		ResultBranch:   t.ResultBranch,
		Constraints:    t.Constraints,
		ClientID:       t.ClientID,
		Tags:           t.Tags,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal TaskCreated payload: %w", err)
	}
	event := NewDomainEvent("Task", t.ID, "TaskCreatedV1", data, 1)
	t.RecordEvent(event)
	return nil
}

// ----------------------------------------------------------------------------
// Subtask Aggregate Root
// ----------------------------------------------------------------------------

// Subtask represents a subtask aggregate root.
type Subtask struct {
	AggregateRoot

	// TaskID is the parent task ID
	TaskID string
	// Type is the subtask type
	Type SubtaskType
	// Description is the subtask description
	Description string
	// AgentTemplate is the agent role ID to use
	AgentTemplate string
	// AgentInstance is the actual agent instance ID (set when assigned)
	AgentInstance *string
	// Status is the current subtask status
	Status TaskStatus
	// Summary is the execution summary (set when completed)
	Summary *string
	// Artifacts are the produced artifacts
	Artifacts []ArtifactRef
	// TokensUsed is the token consumption for this subtask
	TokensUsed int
	// Dependencies are IDs of subtasks this depends on (DAG)
	Dependencies []string
	// StartedAt is when execution started
	StartedAt *time.Time
	// CompletedAt is when execution completed
	CompletedAt *time.Time
}

// ArtifactRef references an artifact produced by a subtask.
type ArtifactRef struct {
	ID       string
	Type     string // "analysis", "diff", "test_result", "report", "log"
	Summary  string
	URL      string
	Size     int64
	CreateAt time.Time
}

// NewSubtask creates a new Subtask aggregate.
func NewSubtask(id, taskID string, subType SubtaskType, description, agentTemplate string) *Subtask {
	return &Subtask{
		AggregateRoot: AggregateRoot{
			ID:      id,
			Version: 1,
		},
		TaskID:        taskID,
		Type:          subType,
		Description:   description,
		AgentTemplate: agentTemplate,
		Status:        TaskStatusPending,
		Artifacts:     []ArtifactRef{},
		Dependencies:  []string{},
	}
}

// Assign assigns the subtask to an agent instance.
// Business rule: only pending subtasks can be assigned.
func (s *Subtask) Assign(agentInstanceID string) error {
	if s.Status != TaskStatusPending {
		return fmt.Errorf("subtask assign failed: subtask is in %s state, expected pending", s.Status)
	}

	s.AgentInstance = &agentInstanceID

	event := NewDomainEvent("Subtask", s.ID, "SubtaskAssignedV1", nil, int(s.Version))
	s.RecordEvent(event)

	return s.transitionTo("Assign")
}

// CanAssign checks if the subtask can be assigned.
func (s *Subtask) CanAssign() bool {
	return s.Status == TaskStatusPending
}

// StartExecution marks the subtask as running.
// Business rule: only assigned subtasks can start execution.
func (s *Subtask) StartExecution() error {
	if s.Status != TaskStatusAssigned {
		return fmt.Errorf("subtask start execution failed: subtask is in %s state, expected assigned", s.Status)
	}

	now := time.Now().UTC()
	s.StartedAt = &now

	event := NewDomainEvent("Subtask", s.ID, "SubtaskExecutionStartedV1", nil, int(s.Version))
	s.RecordEvent(event)

	return s.transitionTo("StartExecution")
}

// CanStartExecution checks if execution can be started.
func (s *Subtask) CanStartExecution() bool {
	return s.Status == TaskStatusAssigned
}

// CompleteExecution marks execution as complete and starts review.
// Business rule: only running subtasks can complete execution.
func (s *Subtask) CompleteExecution() error {
	if s.Status != TaskStatusRunning {
		return fmt.Errorf("subtask complete execution failed: subtask is in %s state, expected running", s.Status)
	}

	event := NewDomainEvent("Subtask", s.ID, "SubtaskExecutionCompletedV1", nil, int(s.Version))
	s.RecordEvent(event)

	return s.transitionTo("CompleteExecution")
}

// CanCompleteExecution checks if execution can be completed.
func (s *Subtask) CanCompleteExecution() bool {
	return s.Status == TaskStatusRunning
}

// FailExecution marks the subtask execution as failed.
// Business rule: only running subtasks can fail execution.
func (s *Subtask) FailExecution() error {
	if s.Status != TaskStatusRunning {
		return fmt.Errorf("subtask fail execution failed: subtask is in %s state, expected running", s.Status)
	}

	now := time.Now().UTC()
	s.CompletedAt = &now

	event := NewDomainEvent("Subtask", s.ID, "SubtaskExecutionFailedV1", nil, int(s.Version))
	s.RecordEvent(event)

	return s.transitionTo("ExecutionFailed")
}

// FailNoAgentAvailable marks the subtask as failed due to no agent available.
// Business rule: only assigned subtasks can fail with no agent.
func (s *Subtask) FailNoAgentAvailable() error {
	if s.Status != TaskStatusAssigned {
		return fmt.Errorf("subtask fail no agent failed: subtask is in %s state, expected assigned", s.Status)
	}

	now := time.Now().UTC()
	s.CompletedAt = &now

	event := NewDomainEvent("Subtask", s.ID, "SubtaskNoAgentAvailableV1", nil, int(s.Version))
	s.RecordEvent(event)

	return s.transitionTo("NoAgentAvailable")
}

// ReviewPassed marks the subtask review as passed.
// Business rule: only reviewing subtasks can pass review.
func (s *Subtask) ReviewPassed() error {
	if s.Status != TaskStatusReviewing {
		return fmt.Errorf("subtask review passed failed: subtask is in %s state, expected reviewing", s.Status)
	}

	now := time.Now().UTC()
	s.CompletedAt = &now

	event := NewDomainEvent("Subtask", s.ID, "SubtaskReviewPassedV1", nil, int(s.Version))
	s.RecordEvent(event)

	return s.transitionTo("ReviewPassed")
}

// CanReviewPassed checks if review can pass.
func (s *Subtask) CanReviewPassed() bool {
	return s.Status == TaskStatusReviewing
}

// ReviewFailedRetry indicates review failed but retry is possible.
// Business rule: only reviewing subtasks can fail review with retry.
func (s *Subtask) ReviewFailedRetry() error {
	if s.Status != TaskStatusReviewing {
		return fmt.Errorf("subtask review failed retry failed: subtask is in %s state, expected reviewing", s.Status)
	}

	event := NewDomainEvent("Subtask", s.ID, "SubtaskReviewFailedRetryV1", nil, int(s.Version))
	s.RecordEvent(event)

	return s.transitionTo("ReviewFailedRetry")
}

// ReviewFailedFatal indicates review failed fatally.
// Business rule: only reviewing subtasks can fail review fatally.
func (s *Subtask) ReviewFailedFatal() error {
	if s.Status != TaskStatusReviewing {
		return fmt.Errorf("subtask review failed fatal failed: subtask is in %s state, expected reviewing", s.Status)
	}

	now := time.Now().UTC()
	s.CompletedAt = &now

	event := NewDomainEvent("Subtask", s.ID, "SubtaskReviewFailedFatalV1", nil, int(s.Version))
	s.RecordEvent(event)

	return s.transitionTo("ReviewFailedFatal")
}

// Cancel cancels the subtask.
// Business rule: subtasks can be cancelled from non-terminal states.
func (s *Subtask) Cancel() error {
	if s.Status.IsTerminal() {
		return fmt.Errorf("subtask cancel failed: subtask is in terminal state %s", s.Status)
	}

	now := time.Now().UTC()
	s.CompletedAt = &now

	event := NewDomainEvent("Subtask", s.ID, "SubtaskCancelledV1", nil, int(s.Version))
	s.RecordEvent(event)

	return s.transitionTo("Cancel")
}

// CanCancel checks if the subtask can be cancelled.
func (s *Subtask) CanCancel() bool {
	return !s.Status.IsTerminal()
}

// transitionTo changes the subtask status using the state machine.
func (s *Subtask) transitionTo(event string) error {
	sm := BuildSubtaskStateMachine(s.ID)
	sm.State = s.Status
	sm.Version = int64(s.Version)

	newState, err := sm.Trigger(event)
	if err != nil {
		return err
	}

	s.Status = newState
	s.Version = int(sm.Version)
	return nil
}

// AvailableEvents returns the list of events that can be triggered from the current state.
func (s *Subtask) AvailableEvents() []string {
	sm := BuildSubtaskStateMachine(s.ID)
	sm.State = s.Status
	return sm.AvailableEvents()
}

// RecordSubtaskCreated records the SubtaskCreatedV1 domain event.
func (s *Subtask) RecordSubtaskCreated() error {
	type SubtaskCreatedEventPayload struct {
		TaskID        string      `json:"task_id"`
		Type          SubtaskType `json:"type"`
		Description   string      `json:"description"`
		AgentTemplate string      `json:"agent_template"`
		Dependencies  []string    `json:"dependencies"`
	}
	payload := SubtaskCreatedEventPayload{
		TaskID:        s.TaskID,
		Type:          s.Type,
		Description:   s.Description,
		AgentTemplate: s.AgentTemplate,
		Dependencies:  s.Dependencies,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal SubtaskCreated payload: %w", err)
	}
	event := NewDomainEvent("Subtask", s.ID, "SubtaskCreatedV1", data, 1)
	s.RecordEvent(event)
	return nil
}