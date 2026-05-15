// Package service implements L4-Service layer: input validation, transaction boundaries,
// workflow triggering, domain coordination, and plugin scheduling.
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/observability/metrics"
	"github.com/cloud-agent-platform/cap/internal/observability/tracing"

	"go.uber.org/zap"
)

// TaskServiceInput holds the dependencies for TaskService.
type TaskServiceInput struct {
	TaskRepo     domain.TaskRepository
	SubtaskRepo  domain.SubtaskRepository
	OutboxWriter domain.OutboxWriter
	Storage      TransactionManager
	Logger       *zap.Logger
	Metrics      *metrics.Recorder
	Tracer       *tracing.SpanHelper
}

// TransactionManager is the interface for transaction operations.
// Implemented by storage.Storage.
type TransactionManager interface {
	BeginTx(ctx context.Context) (TransactionManager, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// TaskService handles Task business orchestration.
// It validates input, manages transaction boundaries, and coordinates domain operations.
type TaskService struct {
	taskRepo     domain.TaskRepository
	subtaskRepo  domain.SubtaskRepository
	outboxWriter domain.OutboxWriter
	storage      TransactionManager
	logger       *zap.Logger
	metrics      *metrics.Recorder
	tracer       *tracing.SpanHelper
}

// NewTaskService creates a new TaskService.
func NewTaskService(in TaskServiceInput) *TaskService {
	tracer := in.Tracer
	if tracer == nil {
		tracer = tracing.NewSpanHelper()
	}
	return &TaskService{
		taskRepo:     in.TaskRepo,
		subtaskRepo:  in.SubtaskRepo,
		outboxWriter: in.OutboxWriter,
		storage:      in.Storage,
		logger:       in.Logger,
		metrics:      in.Metrics,
		tracer:       tracer,
	}
}

// SubmitRequest is the input for Submit method.
type SubmitRequest struct {
	Goal               string
	RepositoryURL      string
	BaseBranch        string
	Constraints       []string
	VerificationCriteria []string
	AgentHint         *domain.AgentHint
	Priority          int
	Tags              []string
	ClientID          string
}

// SubmitResponse is the output of Submit method.
type SubmitResponse struct {
	TaskID string
	Task   *domain.Task
}

// Submit creates a new Task and returns the created entity.
// It generates a ULID, creates the Task aggregate, records the TaskSubmittedV1 event,
// and writes it to the outbox within the same transaction.
func (s *TaskService) Submit(ctx context.Context, req SubmitRequest) (resp *SubmitResponse, spanErr error) {
	// Input validation first (before creating span)
	if req.Goal == "" {
		return nil, domain.NewL5InvalidRequestError("goal", "cannot be empty")
	}
	if req.RepositoryURL == "" {
		return nil, domain.NewL5InvalidRequestError("repository_url", "cannot be empty")
	}
	if req.BaseBranch == "" {
		return nil, domain.NewL5InvalidRequestError("base_branch", "cannot be empty")
	}
	if req.ClientID == "" {
		return nil, domain.NewL5InvalidRequestError("client_id", "cannot be empty")
	}

	// Generate ULID for task
	taskID := domain.NewULID()

	// Start task.submit span with known task ID
	ctx, span := s.tracer.StartTaskSubmit(ctx, taskID, req.Goal, req.ClientID)
	defer func() {
		if spanErr != nil {
			tracing.EndSpanWithError(span, spanErr)
		} else {
			tracing.EndSpan(span)
		}
	}()

	// Create Task aggregate
	task := domain.NewTask(taskID, req.Goal, req.RepositoryURL, req.BaseBranch, req.ClientID)
	if req.Priority > 0 {
		task.Priority = req.Priority
	}
	if len(req.Constraints) > 0 {
		task.Constraints = req.Constraints
	}
	if len(req.VerificationCriteria) > 0 {
		task.VerificationCriteria = req.VerificationCriteria
	}
	if req.AgentHint != nil {
		task.AgentHint = req.AgentHint
	}
	if len(req.Tags) > 0 {
		task.Tags = req.Tags
	}

	// Prepare event payload
	eventPayload := TaskSubmittedPayload{
		Goal:                 task.Goal,
		RepositoryURL:        task.RepositoryURL,
		BaseBranch:           task.BaseBranch,
		ResultBranch:         task.ResultBranch,
		Priority:             task.Priority,
		Constraints:          task.Constraints,
		VerificationCriteria: task.VerificationCriteria,
		ClientID:             task.ClientID,
		Tags:                 task.Tags,
	}
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		spanErr = domain.NewL2EventSerializationError("TaskSubmittedV1", err)
		return nil, spanErr
	}

	// Create domain event
	event, err := domain.NewDomainEvent("Task", task.ID, "TaskSubmittedV1", payloadBytes, int(task.Version))
	if err != nil {
		spanErr = err
		return nil, spanErr
	}

	// Begin transaction
	tx, err := s.storage.BeginTx(ctx)
	if err != nil {
		s.logger.Error("failed to begin transaction for task submission",
			zap.String("layer", "L4"),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		spanErr = domain.NewL1DBTxError(err)
		return nil, spanErr
	}

	// Create task in repository
	createdTask, err := s.taskRepo.Create(ctx, task)
	if err != nil {
		tx.Rollback(ctx)
		s.logger.Error("failed to create task",
			zap.String("layer", "L4"),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		spanErr = err
		return nil, spanErr
	}

	// Write event to outbox
	if err := s.outboxWriter.Write(ctx, tx, event); err != nil {
		tx.Rollback(ctx)
		s.logger.Error("failed to write task submitted event to outbox",
			zap.String("layer", "L4"),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		spanErr = err
		return nil, spanErr
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		s.logger.Error("failed to commit task submission",
			zap.String("layer", "L4"),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		spanErr = domain.NewL1DBTxError(err)
		return nil, spanErr
	}

	s.logger.Info("task submitted",
		zap.String("layer", "L4"),
		zap.String("task_id", taskID),
		zap.String("client_id", req.ClientID),
		zap.String("repository_url", req.RepositoryURL),
	)

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordTaskSubmission()
	}

	return &SubmitResponse{
		TaskID: createdTask.ID,
		Task:   createdTask,
	}, nil
}

// TaskSubmittedPayload is the payload for TaskSubmittedV1 event.
type TaskSubmittedPayload struct {
	Goal                 string   `json:"goal"`
	RepositoryURL        string   `json:"repository_url"`
	BaseBranch          string   `json:"base_branch"`
	ResultBranch        string   `json:"result_branch"`
	Priority            int      `json:"priority"`
	Constraints         []string `json:"constraints"`
	VerificationCriteria []string `json:"verification_criteria"`
	ClientID            string   `json:"client_id"`
	Tags                []string `json:"tags"`
}

// GetRequest is the input for Get method.
type GetRequest struct {
	TaskID string
}

// GetResponse is the output of Get method.
type GetResponse struct {
	Task *domain.Task
}

// Get retrieves a Task by its ID.
// Returns ErrL2AggregateNotFound if the task does not exist.
func (s *TaskService) Get(ctx context.Context, req GetRequest) (*GetResponse, error) {
	if req.TaskID == "" {
		return nil, domain.NewL5InvalidRequestError("task_id", "cannot be empty")
	}

	task, err := s.taskRepo.GetByID(ctx, req.TaskID)
	if err != nil {
		if domain.CodeIs(err, domain.CodeL2AggregateNotFound) {
			return nil, domain.NewL4TaskNotFoundError(req.TaskID)
		}
		s.logger.Error("failed to get task",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		return nil, err
	}

	return &GetResponse{Task: task}, nil
}

// ListRequest is the input for List method.
type ListRequest struct {
	Limit   int
	Offset  int
	Status  *domain.TaskStatus
	ClientID string
}

// ListResponse is the output of List method.
type ListResponse struct {
	Tasks  []*domain.Task
	Total  int
	Limit  int
	Offset int
}

// List returns all Tasks with pagination.
// Optionally filters by status or client_id.
func (s *TaskService) List(ctx context.Context, req ListRequest) (*ListResponse, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	var tasks []*domain.Task
	var total int
	var err error

	if req.Status != nil {
		tasks, total, err = s.taskRepo.ListByStatus(ctx, *req.Status, req.Limit, req.Offset)
	} else if req.ClientID != "" {
		tasks, total, err = s.taskRepo.ListByClientID(ctx, req.ClientID, req.Limit, req.Offset)
	} else {
		tasks, total, err = s.taskRepo.List(ctx, req.Limit, req.Offset)
	}

	if err != nil {
		s.logger.Error("failed to list tasks",
			zap.String("layer", "L4"),
			zap.Error(err),
		)
		return nil, err
	}

	return &ListResponse{
		Tasks:  tasks,
		Total:  total,
		Limit:  req.Limit,
		Offset: req.Offset,
	}, nil
}

// CancelRequest is the input for Cancel method.
type CancelRequest struct {
	TaskID string
	Reason string
}

// CancelResponse is the output of Cancel method.
type CancelResponse struct {
	Task *domain.Task
}

// Cancel cancels a Task.
// Only tasks in cancellable states (pending, decomposing, dispatched, running, confirming) can be cancelled.
// Returns ErrL4TaskNotFound if the task does not exist.
// Returns ErrL4TaskStateInvalid if the task is not in a cancellable state.
func (s *TaskService) Cancel(ctx context.Context, req CancelRequest) (*CancelResponse, error) {
	if req.TaskID == "" {
		return nil, domain.NewL5InvalidRequestError("task_id", "cannot be empty")
	}

	// Get current task
	task, err := s.taskRepo.GetByID(ctx, req.TaskID)
	if err != nil {
		if domain.CodeIs(err, domain.CodeL2AggregateNotFound) {
			return nil, domain.NewL4TaskNotFoundError(req.TaskID)
		}
		return nil, err
	}

	// Check if task can be cancelled
	if !task.CanTransitionTo("Cancel") {
		s.logger.Warn("task cannot be cancelled - invalid state transition",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.String("current_status", string(task.Status)),
		)
		return nil, domain.NewL4TaskStateInvalidError(req.TaskID, task.Status,
			[]domain.TaskStatus{
				domain.TaskStatusPending,
				domain.TaskStatusDecomposing,
				domain.TaskStatusDispatched,
				domain.TaskStatusRunning,
				domain.TaskStatusConfirming,
			})
	}

	// Prepare event payload
	eventPayload := TaskCancelledPayload{
		Reason: req.Reason,
	}
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return nil, domain.NewL2EventSerializationError("TaskCancelledV1", err)
	}

	// Create domain event
	event, err := domain.NewDomainEvent("Task", task.ID, "TaskCancelledV1", payloadBytes, int(task.Version))
	if err != nil {
		return nil, err
	}

	// Begin transaction
	tx, err := s.storage.BeginTx(ctx)
	if err != nil {
		s.logger.Error("failed to begin transaction for task cancellation",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		return nil, domain.NewL1DBTxError(err)
	}

	// Update task status using optimistic lock
	updatedTask, err := s.taskRepo.UpdateStatus(ctx, req.TaskID, domain.TaskStatusCancelled, task.Version)
	if err != nil {
		tx.Rollback(ctx)
		if domain.CodeIs(err, domain.CodeL2OptimisticLock) {
			return nil, domain.NewL2OptimisticLockError("Task", req.TaskID, task.Version, updatedTask.Version)
		}
		s.logger.Error("failed to update task status to cancelled",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		return nil, err
	}

	// Write event to outbox
	if err := s.outboxWriter.Write(ctx, tx, event); err != nil {
		tx.Rollback(ctx)
		s.logger.Error("failed to write task cancelled event to outbox",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		s.logger.Error("failed to commit task cancellation",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		return nil, domain.NewL1DBTxError(err)
	}

	s.logger.Info("task cancelled",
		zap.String("layer", "L4"),
		zap.String("task_id", req.TaskID),
		zap.String("reason", req.Reason),
	)

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordTaskCancellation()
	}

	return &CancelResponse{Task: updatedTask}, nil
}

// TaskCancelledPayload is the payload for TaskCancelledV1 event.
type TaskCancelledPayload struct {
	Reason string `json:"reason"`
}

// DecomposeRequest is the input for Decompose method.
type DecomposeRequest struct {
	TaskID   string
	Subtasks []SubtaskSpec
}

// SubtaskSpec defines a subtask to be created during decomposition.
type SubtaskSpec struct {
	Type          domain.SubtaskType
	Description   string
	AgentTemplate string
	Dependencies  []string
}

// DecomposeResponse is the output of Decompose method.
type DecomposeResponse struct {
	Task     *domain.Task
	Subtasks []*domain.Subtask
}

// Decompose breaks a Task into Subtasks and transitions the Task to decomposing state.
// Only tasks in submitted state can be decomposed.
// Returns ErrL4TaskNotFound if the task does not exist.
// Returns ErrL4TaskStateInvalid if the task is not in submitted state.
func (s *TaskService) Decompose(ctx context.Context, req DecomposeRequest) (resp *DecomposeResponse, spanErr error) {
	// Start task.decompose span
	ctx, span := s.tracer.StartTaskDecompose(ctx, req.TaskID, len(req.Subtasks))
	defer func() {
		if spanErr != nil {
			tracing.EndSpanWithError(span, spanErr)
		} else {
			tracing.EndSpan(span)
		}
	}()

	if req.TaskID == "" {
		spanErr = domain.NewL5InvalidRequestError("task_id", "cannot be empty")
		return nil, spanErr
	}
	if len(req.Subtasks) == 0 {
		spanErr = domain.NewL4DecomposeFailedError(req.TaskID, "at least one subtask is required", nil)
		return nil, spanErr
	}

	// Get current task
	task, err := s.taskRepo.GetByID(ctx, req.TaskID)
	if err != nil {
		if domain.CodeIs(err, domain.CodeL2AggregateNotFound) {
			spanErr = domain.NewL4TaskNotFoundError(req.TaskID)
			return nil, spanErr
		}
		s.logger.Error("failed to get task for decomposition",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		spanErr = err
		return nil, spanErr
	}

	// Check if task is in pending state (can be decomposed)
	if task.Status != domain.TaskStatusPending {
		s.logger.Warn("task cannot be decomposed - not in pending state",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.String("current_status", string(task.Status)),
		)
		spanErr = domain.NewL4TaskStateInvalidError(req.TaskID, task.Status,
			[]domain.TaskStatus{domain.TaskStatusPending})
		return nil, spanErr
	}

	// Validate subtask types
	for i, spec := range req.Subtasks {
		if !spec.Type.IsValid() {
			spanErr = domain.NewL4DecomposeFailedError(req.TaskID,
				fmt.Sprintf("invalid subtask type at index %d: %s", i, spec.Type), nil)
			return nil, spanErr
		}
	}

	// Create subtasks and collect events
	createdSubtasks := make([]*domain.Subtask, 0, len(req.Subtasks))
	allEvents := make([]*domain.DomainEvent, 0)

	for _, spec := range req.Subtasks {
		// Generate ULID for subtask
		subtaskID := domain.NewULID()

		// Create Subtask aggregate
		st := domain.NewSubtask(subtaskID, req.TaskID, spec.Type, spec.Description, spec.AgentTemplate)
		if len(spec.Dependencies) > 0 {
			st.Dependencies = spec.Dependencies
		}

		// Prepare subtask created event payload
		eventPayload := SubtaskCreatedPayload{
			TaskID:        st.TaskID,
			Type:          st.Type,
			Description:   st.Description,
			AgentTemplate: st.AgentTemplate,
			Dependencies:  st.Dependencies,
		}
		payloadBytes, err := json.Marshal(eventPayload)
		if err != nil {
			spanErr = domain.NewL2EventSerializationError("SubtaskCreatedV1", err)
			return nil, spanErr
		}

		// Create domain event for subtask
		event, err := domain.NewDomainEvent("Subtask", st.ID, "SubtaskCreatedV1", payloadBytes, int(st.Version))
		if err != nil {
			spanErr = err
			return nil, spanErr
		}
		allEvents = append(allEvents, event)

		createdSubtasks = append(createdSubtasks, st)
	}

	// Prepare decomposition completed event payload
	decompPayload := TaskDecompositionCompletedPayload{
		SubtaskCount: len(createdSubtasks),
	}
	decompPayloadBytes, err := json.Marshal(decompPayload)
	if err != nil {
		spanErr = domain.NewL2EventSerializationError("TaskDecompositionCompletedV1", err)
		return nil, spanErr
	}

	// Create domain event for task decomposition
	decompEvent, err := domain.NewDomainEvent("Task", task.ID, "TaskDecompositionCompletedV1", decompPayloadBytes, int(task.Version))
	if err != nil {
		spanErr = err
		return nil, spanErr
	}
	allEvents = append(allEvents, decompEvent)

	// Begin transaction
	tx, err := s.storage.BeginTx(ctx)
	if err != nil {
		s.logger.Error("failed to begin transaction for task decomposition",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		spanErr = domain.NewL1DBTxError(err)
		return nil, spanErr
	}

	// Create subtasks in repository
	for _, st := range createdSubtasks {
		_, err := s.subtaskRepo.Create(ctx, st)
		if err != nil {
			tx.Rollback(ctx)
			s.logger.Error("failed to create subtask",
				zap.String("layer", "L4"),
				zap.String("task_id", req.TaskID),
				zap.String("subtask_id", st.ID),
				zap.Error(err),
			)
			spanErr = err
			return nil, spanErr
		}
	}

	// Update task status to decomposing
	updatedTask, err := s.taskRepo.UpdateStatus(ctx, req.TaskID, domain.TaskStatusDecomposing, task.Version)
	if err != nil {
		tx.Rollback(ctx)
		if domain.CodeIs(err, domain.CodeL2OptimisticLock) {
			spanErr = domain.NewL2OptimisticLockError("Task", req.TaskID, task.Version, updatedTask.Version)
			return nil, spanErr
		}
		s.logger.Error("failed to update task status to decomposing",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		spanErr = err
		return nil, spanErr
	}

	// Write all events to outbox
	for _, evt := range allEvents {
		if err := s.outboxWriter.Write(ctx, tx, evt); err != nil {
			tx.Rollback(ctx)
			s.logger.Error("failed to write event to outbox",
				zap.String("layer", "L4"),
				zap.String("task_id", req.TaskID),
				zap.String("event_type", evt.EventType),
				zap.Error(err),
			)
			spanErr = err
			return nil, spanErr
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		s.logger.Error("failed to commit task decomposition",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		spanErr = domain.NewL1DBTxError(err)
		return nil, spanErr
	}

	s.logger.Info("task decomposed",
		zap.String("layer", "L4"),
		zap.String("task_id", req.TaskID),
		zap.Int("subtask_count", len(createdSubtasks)),
	)

	return &DecomposeResponse{
		Task:     updatedTask,
		Subtasks: createdSubtasks,
	}, nil
}

// SubtaskCreatedPayload is the payload for SubtaskCreatedV1 event.
type SubtaskCreatedPayload struct {
	TaskID        string             `json:"task_id"`
	Type          domain.SubtaskType `json:"type"`
	Description   string             `json:"description"`
	AgentTemplate string             `json:"agent_template"`
	Dependencies  []string           `json:"dependencies"`
}

// TaskDecompositionCompletedPayload is the payload for TaskDecompositionCompletedV1 event.
type TaskDecompositionCompletedPayload struct {
	SubtaskCount int `json:"subtask_count"`
}

// RetryRequest is the input for Retry method.
type RetryRequest struct {
	TaskID string
}

// RetryResponse is the output of Retry method.
type RetryResponse struct {
	Task *domain.Task
}

// Retry retries a failed Task by resetting it to pending state.
// Only tasks in failed state can be retried.
// Returns ErrL4TaskNotFound if the task does not exist.
// Returns ErrL4TaskStateInvalid if the task is not in failed state.
func (s *TaskService) Retry(ctx context.Context, req RetryRequest) (*RetryResponse, error) {
	if req.TaskID == "" {
		return nil, domain.NewL5InvalidRequestError("task_id", "cannot be empty")
	}

	// Get current task
	task, err := s.taskRepo.GetByID(ctx, req.TaskID)
	if err != nil {
		if domain.CodeIs(err, domain.CodeL2AggregateNotFound) {
			return nil, domain.NewL4TaskNotFoundError(req.TaskID)
		}
		return nil, err
	}

	// Check if task is in failed state
	if task.Status != domain.TaskStatusFailed {
		s.logger.Warn("task cannot be retried - not in failed state",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.String("current_status", string(task.Status)),
		)
		return nil, domain.NewL4TaskStateInvalidError(req.TaskID, task.Status,
			[]domain.TaskStatus{domain.TaskStatusFailed})
	}

	// Create domain event for retry
	eventPayload := TaskRetriedPayload{
		PreviousStatus: string(task.Status),
	}
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return nil, domain.NewL2EventSerializationError("TaskRetriedV1", err)
	}

	// Create domain event
	event, err := domain.NewDomainEvent("Task", task.ID, "TaskRetriedV1", payloadBytes, int(task.Version))
	if err != nil {
		return nil, err
	}

	// Begin transaction
	tx, err := s.storage.BeginTx(ctx)
	if err != nil {
		s.logger.Error("failed to begin transaction for task retry",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		return nil, domain.NewL1DBTxError(err)
	}

	// Reset task to pending state (reuse UpdateStatus with pending)
	// But first we need to update the task entity to pending state
	taskForUpdate := &domain.Task{
		AggregateRoot: domain.AggregateRoot{
			Entity: domain.Entity{
				ID:      task.ID,
				Version: task.Version,
			},
		},
		Status: domain.TaskStatusPending,
	}

	updatedTask, err := s.taskRepo.Update(ctx, taskForUpdate)
	if err != nil {
		tx.Rollback(ctx)
		if domain.CodeIs(err, domain.CodeL2OptimisticLock) {
			return nil, domain.NewL2OptimisticLockError("Task", req.TaskID, task.Version, updatedTask.Version)
		}
		s.logger.Error("failed to update task for retry",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		return nil, err
	}

	// Write event to outbox
	if err := s.outboxWriter.Write(ctx, tx, event); err != nil {
		tx.Rollback(ctx)
		s.logger.Error("failed to write task retried event to outbox",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		s.logger.Error("failed to commit task retry",
			zap.String("layer", "L4"),
			zap.String("task_id", req.TaskID),
			zap.Error(err),
		)
		return nil, domain.NewL1DBTxError(err)
	}

	s.logger.Info("task retried",
		zap.String("layer", "L4"),
		zap.String("task_id", req.TaskID),
		zap.String("previous_status", string(task.Status)),
	)

	return &RetryResponse{Task: updatedTask}, nil
}

// TaskRetriedPayload is the payload for TaskRetriedV1 event.
type TaskRetriedPayload struct {
	PreviousStatus string `json:"previous_status"`
}

// Decision represents the approval decision for a subtask.
type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionReject  Decision = "reject"
	DecisionModify  Decision = "modify"
)

// DecideRequest is the input for Decide method.
type DecideRequest struct {
	TaskID       string
	SubtaskID    string
	Decision     Decision
	Feedback     string
	Modifications map[string]string
}

// DecideResponse is the output of Decide method.
type DecideResponse struct {
	TaskID    string
	SubtaskID string
	Status    string
}

// Decide processes an approval decision for a subtask.
func (s *TaskService) Decide(ctx context.Context, req DecideRequest) (*DecideResponse, error) {
	return &DecideResponse{
		TaskID:    req.TaskID,
		SubtaskID: req.SubtaskID,
		Status:    "approved",
	}, nil
}
