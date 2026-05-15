// Package orchestrator implements L4 orchestration: task scheduling, agent session
// management, and event-driven workflow coordination.
package orchestrator

import (
	"context"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/domain/worker"
)

// ----------------------------------------------------------------------------
// Agent Runner Interface
// ----------------------------------------------------------------------------

// AgentRunner is the interface for executing agent tasks.
// Implemented by the plugin layer (e.g., Eino-based agent runner).
type AgentRunner interface {
	// Run executes the agent work for the given subtask.
	// It blocks until the work is complete or the context is cancelled.
	// Returns the execution result or an error if execution failed.
	Run(ctx context.Context, subtask *domain.Subtask, task *domain.Task) (*AgentResult, error)

	// Type returns the agent runner type identifier.
	Type() string
}

// ----------------------------------------------------------------------------
// Worker Executor Interface
// ----------------------------------------------------------------------------

// WorkerExecutor is the interface for executing subtasks via worker pools.
// It handles worker acquisition, execution, and release internally.
// Implemented by the plugin layer (e.g., WorkerManager-based executor).
type WorkerExecutor interface {
	// Execute executes a subtask in a worker and returns the result.
	// It handles worker acquisition and release internally.
	// The opts parameter contains the command to execute and optional git options.
	// If opts.GitOptions.DoGitCommit is true, git add/commit/push will be
	// performed inside the sandbox after the agent execution.
	// Returns the execution result or an error if execution failed.
	Execute(ctx context.Context, subtaskID, taskID string, opts worker.ExecOptions) (*AgentResult, error)
}

// AgentResult represents the result of an agent execution.
type AgentResult struct {
	// Summary is a human-readable summary of the execution.
	Summary string
	// Artifacts are the produced artifacts.
	Artifacts []domain.ArtifactRef
	// TokensUsed is the token consumption.
	TokensUsed int
	// ExecutionDuration is how long the execution took.
	ExecutionDuration time.Duration
	// Error is the error if execution failed.
	Error error
}

// ----------------------------------------------------------------------------
// Orchestrator Interface
// ----------------------------------------------------------------------------

// Orchestrator handles task orchestration and agent session management.
// It receives tasks, creates agent sessions, and coordinates execution.
type Orchestrator interface {
	// StartTask starts orchestrating a task.
	// It transitions the task to dispatched and begins agent execution.
	StartTask(ctx context.Context, task *domain.Task) error

	// CancelTask cancels an ongoing task orchestration.
	CancelTask(ctx context.Context, taskID string) error

	// GetTaskStatus returns the current orchestration status for a task.
	GetTaskStatus(ctx context.Context, taskID string) (*OrchestrationStatus, error)
}

// OrchestrationStatus represents the current orchestration status.
type OrchestrationStatus struct {
	TaskID       string
	TaskStatus   domain.TaskStatus
	ActiveAgents int
	Sessions     []AgentSession
}

// AgentSession represents an active agent session.
type AgentSession struct {
	SessionID  string
	SubtaskID  string
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
}

// ----------------------------------------------------------------------------
// Outbox Event Listener Interface
// ----------------------------------------------------------------------------

// EventDispatcher dispatches domain events to appropriate handlers.
type EventDispatcher interface {
	// Dispatch handles a domain event.
	Dispatch(ctx context.Context, event *domain.DomainEvent) error

	// RegisterHandler registers a handler for a specific event type.
	RegisterHandler(eventType string, handler EventHandler)
}

// EventHandler is a function that handles domain events.
type EventHandler func(ctx context.Context, event *domain.DomainEvent) error

// Verify interface implementations at compile time.
var (
	_ Orchestrator = (*OrchestratorImpl)(nil)
)
