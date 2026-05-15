// Package persistence implements L1-Storage repository interfaces using ent ORM.
// It is located in the infra layer and implements domain layer interfaces.
package persistence

import (
	"context"
	"time"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/ent/task"
	"github.com/cloud-agent-platform/cap/internal/domain"

	"go.uber.org/zap"
)

// TaskRepositoryImpl implements domain.TaskRepository using ent ORM.
type TaskRepositoryImpl struct {
	client *ent.Client
	logger *zap.Logger
}

// NewTaskRepository creates a new TaskRepositoryImpl.
func NewTaskRepository(client *ent.Client, logger *zap.Logger) *TaskRepositoryImpl {
	if logger == nil {
		panic("logger is required")
	}
	return &TaskRepositoryImpl{
		client: client,
		logger: logger,
	}
}

// domainToEntTask maps domain task fields to ent TaskCreate.
// It handles field type conversions between domain and ent models.
func (r *TaskRepositoryImpl) domainToEntTaskCreate(t *domain.Task, create *ent.TaskCreate) *ent.TaskCreate {
	create = create.
		SetID(t.ID).
		SetGoal(t.Goal).
		SetStatus(string(t.Status)).
		SetPriority(t.Priority).
		SetRepositoryURL(t.RepositoryURL).
		SetBaseBranch(t.BaseBranch).
		SetResultBranch(t.ResultBranch).
		SetConstraints(t.Constraints).
		SetVerificationCriteria(t.VerificationCriteria).
		SetProgress(t.Progress).
		SetTokensUsed(t.TokensUsed).
		SetEstimatedCost(t.EstimatedCost).
		SetAgentsUsed(t.AgentsUsed).
		SetClientID(t.ClientID).
		SetTags(t.Tags).
		SetCreatedAt(t.CreatedAt.UnixNano()).
		SetVersion(int64(t.Version))

	if t.AgentHint != nil {
		hintMap := map[string]interface{}{
			"templates":  t.AgentHint.Templates,
			"model":      t.AgentHint.Model,
			"max_agents": t.AgentHint.MaxAgents,
		}
		create = create.SetAgentHint(hintMap)
	}

	if t.StartedAt != nil && !t.StartedAt.IsZero() {
		create = create.SetStartedAt(*t.StartedAt)
	}
	if t.CompletedAt != nil && !t.CompletedAt.IsZero() {
		create = create.SetCompletedAt(*t.CompletedAt)
	}

	return create
}

// entToDomainTask maps an ent Task to a domain Task.
func (r *TaskRepositoryImpl) entToDomainTask(entTask *ent.Task) (*domain.Task, error) {
	if entTask == nil {
		return nil, nil
	}

	domTask := &domain.Task{
		Goal:                 entTask.Goal,
		Status:               domain.TaskStatus(entTask.Status),
		Priority:             entTask.Priority,
		RepositoryURL:        entTask.RepositoryURL,
		BaseBranch:           entTask.BaseBranch,
		ResultBranch:         entTask.ResultBranch,
		Constraints:          entTask.Constraints,
		VerificationCriteria: entTask.VerificationCriteria,
		Progress:             entTask.Progress,
		TokensUsed:           entTask.TokensUsed,
		EstimatedCost:        entTask.EstimatedCost,
		AgentsUsed:           entTask.AgentsUsed,
		ClientID:             entTask.ClientID,
		Tags:                 entTask.Tags,
		CreatedAt:            time.Unix(0, entTask.CreatedAt).UTC(),
	}

	// Set embedded Entity fields
	domTask.ID = entTask.ID
	domTask.Version = entTask.Version

	// Map AgentHint
	if len(entTask.AgentHint) > 0 {
		hint := &domain.AgentHint{}
		if v, ok := entTask.AgentHint["templates"].([]interface{}); ok {
			templates := make([]string, len(v))
			for i, tmpl := range v {
				if s, ok := tmpl.(string); ok {
					templates[i] = s
				}
			}
			hint.Templates = templates
		}
		if v, ok := entTask.AgentHint["model"].(string); ok {
			hint.Model = v
		}
		if v, ok := entTask.AgentHint["max_agents"].(float64); ok {
			hint.MaxAgents = int(v)
		}
		domTask.AgentHint = hint
	}

	// Map time pointers
	if !entTask.StartedAt.IsZero() {
		domTask.StartedAt = &entTask.StartedAt
	}
	if !entTask.CompletedAt.IsZero() {
		domTask.CompletedAt = &entTask.CompletedAt
	}

	return domTask, nil
}

// Create creates a new Task and returns the created entity.
func (r *TaskRepositoryImpl) Create(ctx context.Context, t *domain.Task) (*domain.Task, error) {
	if t == nil {
		return nil, domain.NewL2ParamValidationError("task", "task is nil")
	}

	create := r.client.Task.Create()
	create = r.domainToEntTaskCreate(t, create)

	entTask, err := create.Save(ctx)
	if err != nil {
		r.logger.Error("failed to create task",
			zap.String("layer", "L1"),
			zap.String("task_id", t.ID),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	r.logger.Debug("task created",
		zap.String("layer", "L1"),
		zap.String("task_id", entTask.ID),
	)

	return r.entToDomainTask(entTask)
}

// GetByID retrieves a Task by its ID.
// Returns domain.ErrL2AggregateNotFound if the task does not exist.
func (r *TaskRepositoryImpl) GetByID(ctx context.Context, id string) (*domain.Task, error) {
	if id == "" {
		return nil, domain.NewL2ParamValidationError("id", "id is empty")
	}

	entTask, err := r.client.Task.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.NewL2AggregateNotFoundError("Task", id)
		}
		r.logger.Error("failed to get task",
			zap.String("layer", "L1"),
			zap.String("task_id", id),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	return r.entToDomainTask(entTask)
}

// Update updates an existing Task and returns the updated entity.
// Returns domain.ErrL2AggregateNotFound if the task does not exist.
// Returns domain.ErrL2OptimisticLock if the version doesn't match.
func (r *TaskRepositoryImpl) Update(ctx context.Context, t *domain.Task) (*domain.Task, error) {
	if t == nil {
		return nil, domain.NewL2ParamValidationError("task", "task is nil")
	}

	// First check if the task exists and get current version
	existing, err := r.client.Task.Get(ctx, t.ID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.NewL2AggregateNotFoundError("Task", t.ID)
		}
		return nil, domain.NewL1DBQueryError(err)
	}

	// Check optimistic lock
	if existing.Version != t.Version {
		return nil, domain.NewL2OptimisticLockError("Task", t.ID, int64(t.Version), existing.Version)
	}

	// Build update mutation
	update := r.client.Task.UpdateOneID(t.ID).
		SetGoal(t.Goal).
		SetStatus(string(t.Status)).
		SetPriority(t.Priority).
		SetRepositoryURL(t.RepositoryURL).
		SetBaseBranch(t.BaseBranch).
		SetResultBranch(t.ResultBranch).
		SetConstraints(t.Constraints).
		SetVerificationCriteria(t.VerificationCriteria).
		SetProgress(t.Progress).
		SetTokensUsed(t.TokensUsed).
		SetEstimatedCost(t.EstimatedCost).
		SetAgentsUsed(t.AgentsUsed).
		SetClientID(t.ClientID).
		SetTags(t.Tags).
		SetVersion(int64(t.Version + 1))

	if t.AgentHint != nil {
		hintMap := map[string]interface{}{
			"templates":  t.AgentHint.Templates,
			"model":      t.AgentHint.Model,
			"max_agents": t.AgentHint.MaxAgents,
		}
		update = update.SetAgentHint(hintMap)
	}

	if !t.StartedAt.IsZero() {
		update = update.SetStartedAt(*t.StartedAt)
	}
	if t.CompletedAt != nil && !t.CompletedAt.IsZero() {
		update = update.SetCompletedAt(*t.CompletedAt)
	}

	entTask, err := update.Save(ctx)
	if err != nil {
		r.logger.Error("failed to update task",
			zap.String("layer", "L1"),
			zap.String("task_id", t.ID),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	r.logger.Debug("task updated",
		zap.String("layer", "L1"),
		zap.String("task_id", entTask.ID),
		zap.Int64("new_version", entTask.Version),
	)

	return r.entToDomainTask(entTask)
}

// Delete deletes a Task by its ID.
// Returns domain.ErrL2AggregateNotFound if the task does not exist.
func (r *TaskRepositoryImpl) Delete(ctx context.Context, id string) error {
	if id == "" {
		return domain.NewL2ParamValidationError("id", "id is empty")
	}

	err := r.client.Task.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.NewL2AggregateNotFoundError("Task", id)
		}
		r.logger.Error("failed to delete task",
			zap.String("layer", "L1"),
			zap.String("task_id", id),
			zap.Error(err),
		)
		return domain.NewL1DBQueryError(err)
	}

	r.logger.Debug("task deleted",
		zap.String("layer", "L1"),
		zap.String("task_id", id),
	)

	return nil
}

// ListByStatus returns all Tasks with the given status.
func (r *TaskRepositoryImpl) ListByStatus(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Task, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	query := r.client.Task.Query().
		Where(task.Status(string(status))).
		Order(ent.Desc(task.FieldCreatedAt)).
		Limit(limit).
		Offset(offset)

	entTasks, err := query.All(ctx)
	if err != nil {
		r.logger.Error("failed to list tasks by status",
			zap.String("layer", "L1"),
			zap.String("status", string(status)),
			zap.Error(err),
		)
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	// Count total
	count, err := r.client.Task.Query().
		Where(task.Status(string(status))).
		Count(ctx)
	if err != nil {
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	domainTasks := make([]*domain.Task, len(entTasks))
	for i, entTask := range entTasks {
		domainTasks[i], err = r.entToDomainTask(entTask)
		if err != nil {
			return nil, 0, err
		}
	}

	return domainTasks, count, nil
}

// ListByClientID returns all Tasks for a given client.
func (r *TaskRepositoryImpl) ListByClientID(ctx context.Context, clientID string, limit, offset int) ([]*domain.Task, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	query := r.client.Task.Query().
		Where(task.ClientID(clientID)).
		Order(ent.Desc(task.FieldCreatedAt)).
		Limit(limit).
		Offset(offset)

	entTasks, err := query.All(ctx)
	if err != nil {
		r.logger.Error("failed to list tasks by client_id",
			zap.String("layer", "L1"),
			zap.String("client_id", clientID),
			zap.Error(err),
		)
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	// Count total
	count, err := r.client.Task.Query().
		Where(task.ClientID(clientID)).
		Count(ctx)
	if err != nil {
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	domainTasks := make([]*domain.Task, len(entTasks))
	for i, entTask := range entTasks {
		domainTasks[i], err = r.entToDomainTask(entTask)
		if err != nil {
			return nil, 0, err
		}
	}

	return domainTasks, count, nil
}

// List returns all Tasks with pagination.
func (r *TaskRepositoryImpl) List(ctx context.Context, limit, offset int) ([]*domain.Task, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	query := r.client.Task.Query().
		Order(ent.Desc(task.FieldCreatedAt)).
		Limit(limit).
		Offset(offset)

	entTasks, err := query.All(ctx)
	if err != nil {
		r.logger.Error("failed to list tasks",
			zap.String("layer", "L1"),
			zap.Error(err),
		)
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	// Count total
	count, err := r.client.Task.Query().Count(ctx)
	if err != nil {
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	domainTasks := make([]*domain.Task, len(entTasks))
	for i, entTask := range entTasks {
		domainTasks[i], err = r.entToDomainTask(entTask)
		if err != nil {
			return nil, 0, err
		}
	}

	return domainTasks, count, nil
}

// UpdateStatus updates only the status and version of a Task.
// This is optimized for state machine transitions.
func (r *TaskRepositoryImpl) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
	if id == "" {
		return nil, domain.NewL2ParamValidationError("id", "id is empty")
	}

	// Optimistic lock: atomic check+update using ent Where clause
	// expectedVersion is the NEW version (after TransitionTo increments it)
	// so the WHERE condition should match the OLD version (expectedVersion - 1)
	oldVersion := expectedVersion - 1
	entTask, err := r.client.Task.UpdateOneID(id).
		Where(task.VersionEQ(oldVersion)).
		SetStatus(string(status)).
		SetVersion(expectedVersion).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.NewL2OptimisticLockError("Task", id, expectedVersion, oldVersion)
		}
		r.logger.Error("failed to update task status",
			zap.String("layer", "L1"),
			zap.String("task_id", id),
			zap.String("status", string(status)),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	return r.entToDomainTask(entTask)
}

// CountByStatus returns the count of Tasks with the given status.
func (r *TaskRepositoryImpl) CountByStatus(ctx context.Context, status domain.TaskStatus) (int, error) {
	count, err := r.client.Task.Query().
		Where(task.Status(string(status))).
		Count(ctx)
	if err != nil {
		r.logger.Error("failed to count tasks by status",
			zap.String("layer", "L1"),
			zap.String("status", string(status)),
			zap.Error(err),
		)
		return 0, domain.NewL1DBQueryError(err)
	}
	return count, nil
}

// Verify interface implementation at compile time.
var _ domain.TaskRepository = (*TaskRepositoryImpl)(nil)
