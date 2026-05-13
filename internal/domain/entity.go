// Package domain implements L2-Domain layer: domain entities, state machines,
// event collection (Outbox), and business invariants.
// This layer has ZERO external dependencies - pure Go structs + standard library.
package domain

import (
	"time"
)

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

// ----------------------------------------------------------------------------
// Task Aggregate
// ----------------------------------------------------------------------------

// Task represents a task aggregate root.
// Corresponds to Cloud-Agent-Platform.md §三.
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
			Entity: Entity{
				ID:      id,
				Version: 1,
			},
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

// TransitionTo changes the task status using the state machine.
// It validates the transition is allowed and updates version.
func (t *Task) TransitionTo(event string) error {
	sm := BuildTaskStateMachine(t.ID)
	sm.State = t.Status
	sm.Version = t.Version

	newState, err := sm.Trigger(event)
	if err != nil {
		return err
	}

	t.Status = newState
	t.Version = sm.Version
	return nil
}

// CanTransitionTo checks if a transition is allowed without executing it.
func (t *Task) CanTransitionTo(event string) bool {
	sm := BuildTaskStateMachine(t.ID)
	sm.State = t.Status
	return sm.CanTransition(event)
}

// MarkStarted marks the task as started.
func (t *Task) MarkStarted() error {
	return t.TransitionTo("StartExecution")
}

// MarkCompleted marks the task as completed.
func (t *Task) MarkCompleted() error {
	now := time.Now().UTC()
	t.CompletedAt = &now
	return t.TransitionTo("ReviewPassed")
}

// MarkFailed marks the task as failed.
func (t *Task) MarkFailed() error {
	now := time.Now().UTC()
	t.CompletedAt = &now
	return t.TransitionTo("ExecutionFailed")
}

// MarkCancelled marks the task as cancelled.
func (t *Task) MarkCancelled() error {
	return t.TransitionTo("Cancel")
}

// ----------------------------------------------------------------------------
// Subtask Aggregate
// ----------------------------------------------------------------------------

// Subtask represents a subtask aggregate root.
// Corresponds to Cloud-Agent-Platform.md §三.
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
			Entity: Entity{
				ID:      id,
				Version: 1,
			},
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

// TransitionTo changes the subtask status using the state machine.
func (s *Subtask) TransitionTo(event string) error {
	sm := BuildSubtaskStateMachine(s.ID)
	sm.State = s.Status
	sm.Version = s.Version

	newState, err := sm.Trigger(event)
	if err != nil {
		return err
	}

	s.Status = newState
	s.Version = sm.Version
	return nil
}

// CanTransitionTo checks if a transition is allowed without executing it.
func (s *Subtask) CanTransitionTo(event string) bool {
	sm := BuildSubtaskStateMachine(s.ID)
	sm.State = s.Status
	return sm.CanTransition(event)
}

// Assign assigns the subtask to an agent instance.
func (s *Subtask) Assign(agentInstanceID string) error {
	s.AgentInstance = &agentInstanceID
	return s.TransitionTo("Assign")
}

// MarkStarted marks the subtask as started.
func (s *Subtask) MarkStarted() error {
	now := time.Now().UTC()
	s.StartedAt = &now
	return s.TransitionTo("StartExecution")
}

// MarkCompleted marks the subtask as completed.
func (s *Subtask) MarkCompleted(summary string) error {
	s.Summary = &summary
	now := time.Now().UTC()
	s.CompletedAt = &now
	return s.TransitionTo("ReviewPassed")
}

// MarkFailed marks the subtask as failed.
func (s *Subtask) MarkFailed() error {
	now := time.Now().UTC()
	s.CompletedAt = &now
	return s.TransitionTo("ExecutionFailed")
}

// MarkCancelled marks the subtask as cancelled.
func (s *Subtask) MarkCancelled() error {
	return s.TransitionTo("Cancel")
}

// ----------------------------------------------------------------------------
// AuditLog Entity
// ----------------------------------------------------------------------------

// AuditLog represents an audit log entry.
// Corresponds to Cloud-Agent-Platform.md §三.
type AuditLog struct {
	ID             string
	TaskID         string
	SubtaskID      *string
	AgentTemplate  *string
	Action         string
	Level          string // "info", "warning", "error", "critical"
	Message        string
	Details        map[string]any
	Timestamp      time.Time
}

// NewAuditLog creates a new AuditLog entry.
func NewAuditLog(id, taskID, action, message, level string) *AuditLog {
	return &AuditLog{
		ID:        id,
		TaskID:    taskID,
		Action:    action,
		Level:     level,
		Message:   message,
		Details:   make(map[string]any),
		Timestamp: time.Now().UTC(),
	}
}

// WithSubtask sets the subtask ID.
func (a *AuditLog) WithSubtask(subtaskID string) *AuditLog {
	a.SubtaskID = &subtaskID
	return a
}

// WithAgentTemplate sets the agent template.
func (a *AuditLog) WithAgentTemplate(template string) *AuditLog {
	a.AgentTemplate = &template
	return a
}

// WithDetail adds a key-value detail.
func (a *AuditLog) WithDetail(key string, value any) *AuditLog {
	if a.Details == nil {
		a.Details = make(map[string]any)
	}
	a.Details[key] = value
	return a
}
