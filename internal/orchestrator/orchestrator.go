// Package orchestrator implements L4 orchestration: task scheduling, agent session
// management, and event-driven workflow coordination.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/domain/worker"

	"go.uber.org/zap"
)

// Config holds configuration for the orchestrator.
type Config struct {
	// MaxConcurrentSessions is the maximum number of concurrent agent sessions.
	MaxConcurrentSessions int
	// SessionTimeout is the timeout for agent sessions.
	SessionTimeout time.Duration
	// DefaultAgentTemplate is the default agent template to use.
	DefaultAgentTemplate string
	// GuardianEnabled enables guardian risk evaluation and approval workflow.
	GuardianEnabled bool
}

// Guardian defines the interface for risk evaluation and approval workflow.
// It evaluates whether tasks require human approval before execution.
type Guardian interface {
	// NeedsApproval evaluates whether the given task requires approval before execution.
	NeedsApproval(ctx context.Context, task *domain.Task) bool

	// RequestApproval requests approval for a task from the guardian.
	// It pushes the approval request via WebSocket and returns immediately.
	// The caller should wait on the ResultCh in the returned ApprovalRequest.
	RequestApproval(ctx context.Context, task *domain.Task, customTimeout time.Duration) (*ApprovalRequest, error)

	// IsPending checks if there is a pending approval request for the given task.
	IsPending(ctx context.Context, taskID string) bool
}

// ApprovalRequest wraps the guardian approval request with the result channel for blocking wait.
type ApprovalRequest struct {
	TaskID         string
	TaskGoal       string
	EstimatedCost  float64
	RiskLevel      RiskLevel
	RequestedAt    time.Time
	ExpiresAt      time.Time
	RequireApproval bool
	Timeout        time.Duration
	ResultCh       <-chan ApprovalResult
}

// RiskLevel represents the risk level of an operation.
type RiskLevel string

const (
	RiskLevelLow    RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh   RiskLevel = "high"
)

// ApprovalResult represents the result of an approval decision.
type ApprovalResult string

const (
	ApprovalResultApprove ApprovalResult = "approve"
	ApprovalResultReject  ApprovalResult = "reject"
	ApprovalResultTimeout ApprovalResult = "timeout"
)

// DefaultConfig returns the default orchestrator configuration.
func DefaultConfig() Config {
	return Config{
		MaxConcurrentSessions: 10,
		SessionTimeout:        30 * time.Minute,
		DefaultAgentTemplate:   "executor",
	}
}

// OrchestratorImpl implements the Orchestrator interface.
type OrchestratorImpl struct {
	cfg         Config
	taskRepo    domain.TaskRepository
	subtaskRepo domain.SubtaskRepository
	outboxWriter domain.OutboxWriter
	txManager   TransactionManager
	agentRunner AgentRunner
	logger      *zap.Logger

	// Worker executor for subtask execution via worker pools
	workerExecutor WorkerExecutor

	// Guardian for risk evaluation and approval workflow
	guardian Guardian

	// Active sessions
	sessions   map[string]*agentSession
	sessionsMu sync.RWMutex

	// Event dispatcher
	dispatcher *eventDispatcher
}

// TransactionManager is the interface for transaction operations.
type TransactionManager interface {
	BeginTx(ctx context.Context) (TransactionManager, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// agentSession represents an active agent session.
type agentSession struct {
	ID         string
	TaskID     string
	SubtaskID  string
	Status     string // "pending", "running", "completed", "failed", "cancelled"
	Template   string
	StartedAt  time.Time
	FinishedAt *time.Time
	Result     *AgentResult
}

// NewOrchestrator creates a new OrchestratorImpl.
func NewOrchestrator(
	cfg Config,
	taskRepo domain.TaskRepository,
	subtaskRepo domain.SubtaskRepository,
	outboxWriter domain.OutboxWriter,
	txManager TransactionManager,
	agentRunner AgentRunner,
	logger *zap.Logger,
	workerExecutor WorkerExecutor,
	guardian Guardian,
) *OrchestratorImpl {
	if cfg.MaxConcurrentSessions == 0 {
		cfg = DefaultConfig()
	}

	o := &OrchestratorImpl{
		cfg:          cfg,
		taskRepo:     taskRepo,
		subtaskRepo:  subtaskRepo,
		outboxWriter: outboxWriter,
		txManager:    txManager,
		agentRunner:  agentRunner,
		logger:       logger,
		workerExecutor: workerExecutor,
		guardian:     guardian,
		sessions:     make(map[string]*agentSession),
		dispatcher:   newEventDispatcher(logger),
	}

	// Register event handlers
	o.dispatcher.RegisterHandler(EventTypeTaskAssigned, o.handleTaskAssigned)
	o.dispatcher.RegisterHandler(EventTypeTaskStarted, o.handleTaskStarted)
	o.dispatcher.RegisterHandler("TaskSubmittedV1", o.handleTaskSubmitted)

	return o
}

// StartTask starts orchestrating a task.
// It transitions the task to dispatched and begins agent execution.
func (o *OrchestratorImpl) StartTask(ctx context.Context, task *domain.Task) error {
	// Validate task is in a valid state for starting
	if task.Status != domain.TaskStatusPending && task.Status != domain.TaskStatusDecomposing {
		o.logger.Warn("cannot start task - invalid state",
			zap.String("task_id", task.ID),
			zap.String("current_status", string(task.Status)),
		)
		return domain.NewL4TaskStateInvalidError(task.ID, task.Status,
			[]domain.TaskStatus{domain.TaskStatusPending, domain.TaskStatusDecomposing})
	}

	// Get subtasks for this task
	subtasks, err := o.subtaskRepo.ListByTaskID(ctx, task.ID)
	if err != nil {
		o.logger.Error("failed to list subtasks for task",
			zap.String("task_id", task.ID),
			zap.Error(err),
		)
		return err
	}

	// If no subtasks, this is a single-agent scenario - create a synthetic subtask
	if len(subtasks) == 0 {
		return o.startSingleAgentExecution(ctx, task)
	}

	// Multi-subtask: start with the first ready subtask (no dependencies)
	for _, st := range subtasks {
		if len(st.Dependencies) == 0 {
			return o.startAgentSession(ctx, task, st)
		}
	}

	return nil
}

// startSingleAgentExecution handles tasks without explicit subtasks.
// It treats the task itself as a single unit of work.
// State path: pending -> decomposing -> dispatched -> running -> reviewing -> completed
func (o *OrchestratorImpl) startSingleAgentExecution(ctx context.Context, task *domain.Task) error {
	sessionID := domain.NewULID()

	// Create synthetic subtask for single-agent execution
	syntheticSubtask := &domain.Subtask{
		TaskID:        task.ID,
		Description:   task.Goal,
		AgentTemplate: o.cfg.DefaultAgentTemplate,
		Status:        domain.TaskStatusPending,
	}

	o.logger.Info("starting single-agent execution",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
	)

	// Record session start
	session := &agentSession{
		ID:        sessionID,
		TaskID:    task.ID,
		SubtaskID: syntheticSubtask.ID,
		Status:    "running",
		Template:  syntheticSubtask.AgentTemplate,
		StartedAt: time.Now().UTC(),
	}
	o.addSession(session)

	// Emit TaskAssignedV1 event
	assignedPayload := &TaskAssignedPayload{
		TaskID:        task.ID,
		SubtaskID:     syntheticSubtask.ID,
		AgentTemplate: syntheticSubtask.AgentTemplate,
		SessionID:     sessionID,
		AssignedAt:    time.Now().UTC(),
	}
	payloadBytes, err := MarshalTaskAssignedPayload(assignedPayload)
	if err != nil {
		return domain.NewL2EventSerializationError(EventTypeTaskAssigned, err)
	}

	event, err := domain.NewDomainEvent("Task", task.ID, EventTypeTaskAssigned, payloadBytes, int(task.Version))
	if err != nil {
		return err
	}

	// Begin transaction for first state transition
	tx, err := o.txManager.BeginTx(ctx)
	if err != nil {
		return domain.NewL1DBTxError(err)
	}

	// Transition task from pending to decomposing
	if err := task.TransitionTo("StartDecomposition"); err != nil {
		tx.Rollback(ctx)
		o.logger.Warn("task transition to decomposing failed",
			zap.String("task_id", task.ID),
			zap.Error(err),
		)
		return err
	}

	// Update task status to decomposing
	if _, err := o.taskRepo.UpdateStatus(ctx, task.ID, domain.TaskStatusDecomposing, task.Version); err != nil {
		tx.Rollback(ctx)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.NewL1DBTxError(err)
	}

	// Second transaction: decomposing -> dispatched
	tx2, err := o.txManager.BeginTx(ctx)
	if err != nil {
		return domain.NewL1DBTxError(err)
	}

	// Transition task from decomposing to dispatched
	if err := task.TransitionTo("DecompositionComplete"); err != nil {
		tx2.Rollback(ctx)
		o.logger.Warn("task transition to dispatched failed",
			zap.String("task_id", task.ID),
			zap.Error(err),
		)
		return err
	}

	if err := o.outboxWriter.Write(ctx, tx2, event); err != nil {
		tx2.Rollback(ctx)
		return err
	}

	if _, err := o.taskRepo.UpdateStatus(ctx, task.ID, domain.TaskStatusDispatched, task.Version); err != nil {
		tx2.Rollback(ctx)
		return err
	}

	if err := tx2.Commit(ctx); err != nil {
		return domain.NewL1DBTxError(err)
	}

	// Execute agent asynchronously
	go o.executeAgentSession(context.Background(), session, syntheticSubtask, task)

	return nil
}

// startAgentSession starts an agent session for a subtask.
func (o *OrchestratorImpl) startAgentSession(ctx context.Context, task *domain.Task, subtask *domain.Subtask) error {
	sessionID := domain.NewULID()

	o.logger.Info("starting agent session",
		zap.String("task_id", task.ID),
		zap.String("subtask_id", subtask.ID),
		zap.String("session_id", sessionID),
		zap.String("agent_template", subtask.AgentTemplate),
	)

	// Record session
	session := &agentSession{
		ID:        sessionID,
		TaskID:    task.ID,
		SubtaskID: subtask.ID,
		Status:    "pending",
		Template:  subtask.AgentTemplate,
		StartedAt: time.Now().UTC(),
	}
	o.addSession(session)

	// Emit TaskAssignedV1 event
	assignedPayload := &TaskAssignedPayload{
		TaskID:        task.ID,
		SubtaskID:     subtask.ID,
		AgentTemplate: subtask.AgentTemplate,
		SessionID:     sessionID,
		AssignedAt:    time.Now().UTC(),
	}
	payloadBytes, err := MarshalTaskAssignedPayload(assignedPayload)
	if err != nil {
		return domain.NewL2EventSerializationError(EventTypeTaskAssigned, err)
	}

	event, err := domain.NewDomainEvent("Task", task.ID, EventTypeTaskAssigned, payloadBytes, int(task.Version))
	if err != nil {
		return err
	}

	// Write event to outbox
	tx, err := o.txManager.BeginTx(ctx)
	if err != nil {
		return domain.NewL1DBTxError(err)
	}

	if err := o.outboxWriter.Write(ctx, tx, event); err != nil {
		tx.Rollback(ctx)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.NewL1DBTxError(err)
	}

	// Execute agent asynchronously
	go o.executeAgentSession(context.Background(), session, subtask, task)

	return nil
}

// executeAgentSession executes an agent session.
func (o *OrchestratorImpl) executeAgentSession(ctx context.Context, session *agentSession, subtask *domain.Subtask, task *domain.Task) {
	session.Status = "running"

	o.logger.Info("agent session executing",
		zap.String("session_id", session.ID),
		zap.String("task_id", session.TaskID),
		zap.String("subtask_id", session.SubtaskID),
	)

	// Emit TaskStartedV1 event
	startedPayload := &TaskStartedPayload{
		TaskID:    task.ID,
		SubtaskID: subtask.ID,
		SessionID: session.ID,
		StartedAt: time.Now().UTC(),
	}
	payloadBytes, err := MarshalTaskStartedPayload(startedPayload)
	if err != nil {
		o.logger.Error("failed to marshal TaskStartedV1 payload",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	event, err := domain.NewDomainEvent("Task", task.ID, EventTypeTaskStarted, payloadBytes, int(task.Version))
	if err != nil {
		o.logger.Error("failed to create TaskStartedV1 event",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	// Update task status to running if in dispatched state
	if task.Status == domain.TaskStatusDispatched {
		if err := task.TransitionTo("StartExecution"); err != nil {
			o.logger.Warn("task transition to running failed",
				zap.String("task_id", task.ID),
				zap.Error(err),
			)
		}
	}

	// Write event to outbox
	tx, err := o.txManager.BeginTx(ctx)
	if err != nil {
		o.logger.Error("failed to begin transaction for TaskStartedV1",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	if err := o.outboxWriter.Write(ctx, tx, event); err != nil {
		tx.Rollback(ctx)
		o.logger.Error("failed to write TaskStartedV1 event",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	// Update task status to running
	if _, err := o.taskRepo.UpdateStatus(ctx, task.ID, domain.TaskStatusRunning, task.Version); err != nil {
		tx.Rollback(ctx)
		o.logger.Error("failed to update task status to running",
			zap.String("task_id", task.ID),
			zap.Error(err),
		)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		o.logger.Error("failed to commit TaskStartedV1",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	// Guardian: check if approval is required before execution
	if o.guardian != nil && o.cfg.GuardianEnabled {
		if o.guardian.NeedsApproval(ctx, task) {
			o.logger.Info("guardian requires approval before execution",
				zap.String("task_id", task.ID),
				zap.String("subtask_id", subtask.ID),
			)

			// Transition task to confirming state
			if task.Status == domain.TaskStatusRunning || task.Status == domain.TaskStatusDispatched {
				if transitionErr := task.TransitionTo("RequestConfirmation"); transitionErr != nil {
					o.logger.Warn("task transition to confirming failed",
						zap.String("task_id", task.ID),
						zap.Error(transitionErr),
					)
				}
				// Update task status to confirming
				if _, updateErr := o.taskRepo.UpdateStatus(ctx, task.ID, domain.TaskStatusConfirming, task.Version); updateErr != nil {
					o.logger.Error("failed to update task status to confirming",
						zap.String("task_id", task.ID),
						zap.Error(updateErr),
					)
				}
			}

			// Request approval from guardian
			approvalReq, approvalErr := o.guardian.RequestApproval(ctx, task, 30*time.Minute)
			if approvalErr != nil {
				o.logger.Error("guardian approval request failed",
					zap.String("task_id", task.ID),
					zap.Error(approvalErr),
				)
				// Treat as rejection
				session.Status = "failed"
				session.Result = &AgentResult{Error: fmt.Errorf("guardian approval request failed: %w", approvalErr)}
				o.handleAgentFailure(ctx, session, task, fmt.Errorf("guardian approval request failed"))
				return
			}

			// Wait for approval result
			select {
			case <-ctx.Done():
				o.logger.Warn("guardian approval wait cancelled",
					zap.String("task_id", task.ID),
				)
				session.Status = "failed"
				session.Result = &AgentResult{Error: fmt.Errorf("approval wait cancelled")}
				o.handleAgentFailure(ctx, session, task, fmt.Errorf("approval wait cancelled"))
				return
			case result, ok := <-approvalReq.ResultCh:
				if !ok {
					o.logger.Warn("guardian approval channel closed without result",
						zap.String("task_id", task.ID),
					)
					session.Status = "failed"
					session.Result = &AgentResult{Error: fmt.Errorf("approval channel closed")}
					o.handleAgentFailure(ctx, session, task, fmt.Errorf("approval channel closed"))
					return
				}

				// Handle approval result
				switch result {
				case ApprovalResultApprove:
					o.logger.Info("guardian approved execution",
						zap.String("task_id", task.ID),
					)
					// Transition back to running
					if task.Status == domain.TaskStatusConfirming {
						if transitionErr := task.TransitionTo("ApprovalGranted"); transitionErr != nil {
							o.logger.Warn("task transition to running after approval failed",
								zap.String("task_id", task.ID),
								zap.Error(transitionErr),
							)
						}
					}
				case ApprovalResultReject:
					o.logger.Warn("guardian rejected execution",
						zap.String("task_id", task.ID),
					)
					session.Status = "failed"
					session.Result = &AgentResult{Error: fmt.Errorf("guardian rejected execution")}
					o.handleAgentFailure(ctx, session, task, fmt.Errorf("guardian rejected execution"))
					return
				case ApprovalResultTimeout:
					o.logger.Warn("guardian approval timed out",
						zap.String("task_id", task.ID),
					)
					session.Status = "failed"
					session.Result = &AgentResult{Error: fmt.Errorf("guardian approval timeout")}
					o.handleAgentFailure(ctx, session, task, fmt.Errorf("guardian approval timeout"))
					return
				default:
					o.logger.Warn("guardian unknown approval result",
						zap.String("task_id", task.ID),
						zap.String("result", string(result)),
					)
					session.Status = "failed"
					session.Result = &AgentResult{Error: fmt.Errorf("unknown approval result: %s", result)}
					o.handleAgentFailure(ctx, session, task, fmt.Errorf("unknown approval result"))
					return
				}
			}
		}
	}

	// Run the agent
	var result *AgentResult
	if o.workerExecutor != nil {
		execOpts := worker.ExecOptions{
			Cmd:     buildAgentCommand(subtask, task),
			Timeout: o.cfg.SessionTimeout,
			Envvars: map[string]string{
				"TASK_ID":       task.ID,
				"TASK_GOAL":     task.Goal,
				"REPO_URL":      task.RepositoryURL,
				"BASE_BRANCH":   task.BaseBranch,
				"RESULT_BRANCH": task.ResultBranch,
			},
			GitOptions: &worker.GitOptions{
				DoGitCommit:   true,
				CommitMessage: buildGitCommitMessage(subtask, task),
				ResultBranch:  task.ResultBranch,
			},
		}
		result, err = o.workerExecutor.Execute(ctx, subtask.ID, task.ID, execOpts)
	} else {
		// Fall back to direct agent runner
		result, err = o.agentRunner.Run(ctx, subtask, task)
	}

	// Handle completion
	if err != nil {
		session.Status = "failed"
		session.Result = &AgentResult{Error: err}
		o.handleAgentFailure(ctx, session, task, err)
	} else {
		session.Status = "completed"
		session.Result = result
		o.handleAgentSuccess(ctx, session, task, result)
	}
}

// handleAgentSuccess handles a successful agent execution.
func (o *OrchestratorImpl) handleAgentSuccess(ctx context.Context, session *agentSession, task *domain.Task, result *AgentResult) {
	now := time.Now().UTC()
	session.FinishedAt = &now

	// Build completion payload
	completedPayload := &TaskCompletedPayload{
		TaskID:           task.ID,
		SubtaskID:        session.SubtaskID,
		SessionID:        session.ID,
		Summary:          result.Summary,
		Artifacts:        result.Artifacts,
		TokensUsed:       result.TokensUsed,
		ExecutionSeconds: result.ExecutionDuration.Seconds(),
		CompletedAt:      now,
	}
	payloadBytes, err := MarshalTaskCompletedPayload(completedPayload)
	if err != nil {
		o.logger.Error("failed to marshal TaskCompletedV1 payload",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	// Auto-complete: skip reviewing, go directly to completed (post-review model).
	// Humans audit results asynchronously via dashboard, not as a gate.
	if err := task.TransitionTo("AutoComplete"); err != nil {
		o.logger.Warn("task auto-complete transition failed",
			zap.String("task_id", task.ID),
			zap.Error(err),
		)
	}

	// Write outbox event + update status to completed in a single transaction
	event, err := domain.NewDomainEvent("Task", task.ID, EventTypeTaskCompleted, payloadBytes, int(task.Version))
	if err != nil {
		o.logger.Error("failed to create TaskCompletedV1 event",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	tx, err := o.txManager.BeginTx(ctx)
	if err != nil {
		o.logger.Error("failed to begin transaction for task completion",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	if err := o.outboxWriter.Write(ctx, tx, event); err != nil {
		tx.Rollback(ctx)
		o.logger.Error("failed to write TaskCompletedV1 event",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	// Update task status directly to completed (skip reviewing)
	if _, err := o.taskRepo.UpdateStatus(ctx, task.ID, domain.TaskStatusCompleted, task.Version); err != nil {
		tx.Rollback(ctx)
		o.logger.Error("failed to update task status to completed",
			zap.String("task_id", task.ID),
			zap.Error(err),
		)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		o.logger.Error("failed to commit task completion",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	o.logger.Info("task completed successfully",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("summary", result.Summary),
	)
}
func (o *OrchestratorImpl) handleAgentFailure(ctx context.Context, session *agentSession, task *domain.Task, err error) {
	now := time.Now().UTC()
	session.FinishedAt = &now

	// Emit TaskFailedV1 event
	errReason := "unknown error"
	if err != nil {
		errReason = err.Error()
	}
	failedPayload := &TaskFailedPayload{
		TaskID:    task.ID,
		SubtaskID: session.SubtaskID,
		SessionID: session.ID,
		Reason:    errReason,
		FailedAt:  now,
	}
	payloadBytes, err := MarshalTaskFailedPayload(failedPayload)
	if err != nil {
		o.logger.Error("failed to marshal TaskFailedV1 payload",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	// Transition task to failed state
	if err := task.TransitionTo("ExecutionFailed"); err != nil {
		o.logger.Warn("task transition to failed failed",
			zap.String("task_id", task.ID),
			zap.Error(err),
		)
	}

	event, err := domain.NewDomainEvent("Task", task.ID, EventTypeTaskFailed, payloadBytes, int(task.Version))
	if err != nil {
		o.logger.Error("failed to create TaskFailedV1 event",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	// Write to outbox
	tx, err := o.txManager.BeginTx(ctx)
	if err != nil {
		o.logger.Error("failed to begin transaction for TaskFailedV1",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	if err := o.outboxWriter.Write(ctx, tx, event); err != nil {
		tx.Rollback(ctx)
		o.logger.Error("failed to write TaskFailedV1 event",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	// Update task status to failed
	if _, err := o.taskRepo.UpdateStatus(ctx, task.ID, domain.TaskStatusFailed, task.Version); err != nil {
		tx.Rollback(ctx)
		o.logger.Error("failed to update task status to failed",
			zap.String("task_id", task.ID),
			zap.Error(err),
		)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		o.logger.Error("failed to commit TaskFailedV1",
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return
	}

	o.logger.Warn("task execution failed",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("reason", errReason),
	)
}

// CancelTask cancels an ongoing task orchestration.
func (o *OrchestratorImpl) CancelTask(ctx context.Context, taskID string) error {
	task, err := o.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return err
	}

	// Check if task can be cancelled
	if !task.CanTransitionTo("Cancel") {
		return domain.NewL4TaskStateInvalidError(taskID, task.Status,
			[]domain.TaskStatus{
				domain.TaskStatusPending,
				domain.TaskStatusDecomposing,
				domain.TaskStatusDispatched,
				domain.TaskStatusRunning,
				domain.TaskStatusConfirming,
			})
	}

	// Cancel active sessions
	o.sessionsMu.RLock()
	for _, session := range o.sessions {
		if session.TaskID == taskID && session.Status == "running" {
			session.Status = "cancelled"
			now := time.Now().UTC()
			session.FinishedAt = &now
		}
	}
	o.sessionsMu.RUnlock()

	// Emit cancellation event
	eventPayload := TaskCancelledPayload{
		Reason: "Task cancelled by user",
	}
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return domain.NewL2EventSerializationError(EventTypeTaskFailed, err)
	}

	event, err := domain.NewDomainEvent("Task", task.ID, "TaskCancelledV1", payloadBytes, int(task.Version))
	if err != nil {
		return err
	}

	// Transition task to cancelled
	if err := task.TransitionTo("Cancel"); err != nil {
		return err
	}

	// Write to outbox
	tx, err := o.txManager.BeginTx(ctx)
	if err != nil {
		return domain.NewL1DBTxError(err)
	}

	if err := o.outboxWriter.Write(ctx, tx, event); err != nil {
		tx.Rollback(ctx)
		return err
	}

	if _, err := o.taskRepo.UpdateStatus(ctx, taskID, domain.TaskStatusCancelled, task.Version); err != nil {
		tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}

// GetTaskStatus returns the current orchestration status for a task.
func (o *OrchestratorImpl) GetTaskStatus(ctx context.Context, taskID string) (*OrchestrationStatus, error) {
	task, err := o.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	o.sessionsMu.RLock()
	defer o.sessionsMu.RUnlock()

	var sessions []AgentSession
	for _, s := range o.sessions {
		if s.TaskID == taskID {
			sessions = append(sessions, AgentSession{
				SessionID:  s.ID,
				SubtaskID:  s.SubtaskID,
				Status:     s.Status,
				StartedAt:  s.StartedAt,
				FinishedAt: s.FinishedAt,
			})
		}
	}

	return &OrchestrationStatus{
		TaskID:       taskID,
		TaskStatus:   task.Status,
		ActiveAgents: len(sessions),
		Sessions:     sessions,
	}, nil
}

// Dispatch dispatches a domain event to the appropriate handler.
func (o *OrchestratorImpl) Dispatch(ctx context.Context, event *domain.DomainEvent) error {
	return o.dispatcher.Dispatch(ctx, event)
}

// addSession adds a session to the active sessions map.
func (o *OrchestratorImpl) addSession(session *agentSession) {
	o.sessionsMu.Lock()
	defer o.sessionsMu.Unlock()
	o.sessions[session.ID] = session
}

// removeSession removes a session from the active sessions map.
func (o *OrchestratorImpl) removeSession(sessionID string) {
	o.sessionsMu.Lock()
	defer o.sessionsMu.Unlock()
	delete(o.sessions, sessionID)
}

// ----------------------------------------------------------------------------
// Event Handlers
// ----------------------------------------------------------------------------

// handleTaskSubmitted handles TaskSubmittedV1 events.
func (o *OrchestratorImpl) handleTaskSubmitted(ctx context.Context, event *domain.DomainEvent) error {
	o.logger.Info("received TaskSubmittedV1 event",
		zap.String("aggregate_id", event.AggregateID),
		zap.String("event_id", event.EventID),
	)
	return nil
}

// handleTaskAssigned handles TaskAssignedV1 events.
func (o *OrchestratorImpl) handleTaskAssigned(ctx context.Context, event *domain.DomainEvent) error {
	o.logger.Info("received TaskAssignedV1 event",
		zap.String("aggregate_id", event.AggregateID),
		zap.String("event_id", event.EventID),
	)
	return nil
}

// handleTaskStarted handles TaskStartedV1 events.
func (o *OrchestratorImpl) handleTaskStarted(ctx context.Context, event *domain.DomainEvent) error {
	o.logger.Info("received TaskStartedV1 event",
		zap.String("aggregate_id", event.AggregateID),
		zap.String("event_id", event.EventID),
	)
	return nil
}

// ----------------------------------------------------------------------------
// Event Dispatcher
// ----------------------------------------------------------------------------

// eventDispatcher dispatches events to registered handlers.
type eventDispatcher struct {
	handlers map[string]EventHandler
	logger   *zap.Logger
	mu       sync.RWMutex
}

// newEventDispatcher creates a new event dispatcher.
func newEventDispatcher(logger *zap.Logger) *eventDispatcher {
	return &eventDispatcher{
		handlers: make(map[string]EventHandler),
		logger:   logger,
	}
}

// Dispatch dispatches an event to its registered handler.
func (d *eventDispatcher) Dispatch(ctx context.Context, event *domain.DomainEvent) error {
	d.mu.RLock()
	handler, ok := d.handlers[event.EventType]
	d.mu.RUnlock()

	if !ok {
		d.logger.Debug("no handler registered for event type",
			zap.String("event_type", event.EventType),
		)
		return nil
	}

	return handler(ctx, event)
}

// RegisterHandler registers a handler for an event type.
func (d *eventDispatcher) RegisterHandler(eventType string, handler EventHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[eventType] = handler
	d.logger.Info("registered event handler",
		zap.String("event_type", eventType),
	)
}

// TaskCancelledPayload is the payload for TaskCancelledV1 event.
type TaskCancelledPayload struct {
	Reason string `json:"reason"`
}

// buildAgentCommand constructs the agent command to execute in the sandbox.
// This is a placeholder - actual command construction depends on agent template.
func buildAgentCommand(subtask *domain.Subtask, task *domain.Task) []string {
	// Default command structure - agent runner determines the actual implementation
	// The command should invoke the agent with the subtask description
	agentTemplate := subtask.AgentTemplate
	if agentTemplate == "" {
		agentTemplate = "executor"
	}
	return []string{"/bin/sh", "-c", fmt.Sprintf("echo 'Executing agent: %s, task: %s, subtask: %s'", agentTemplate, task.ID, subtask.ID)}
}

// buildGitCommitMessage constructs the git commit message.
func buildGitCommitMessage(subtask *domain.Subtask, task *domain.Task) string {
	// Format: "[task-id] subtask-description"
	return fmt.Sprintf("[%s] %s", task.ID, subtask.Description)
}
