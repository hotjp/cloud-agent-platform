// Package persistence implements L1-Storage repository interfaces using ent ORM.
package persistence

import (
	"context"
	"time"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/ent/subtask"
	"github.com/cloud-agent-platform/cap/internal/domain"

	"go.uber.org/zap"
)

// SubtaskRepositoryImpl implements domain.SubtaskRepository using ent ORM.
type SubtaskRepositoryImpl struct {
	client *ent.Client
	logger *zap.Logger
}

// NewSubtaskRepository creates a new SubtaskRepositoryImpl.
func NewSubtaskRepository(client *ent.Client, logger *zap.Logger) *SubtaskRepositoryImpl {
	if logger == nil {
		panic("logger is required")
	}
	return &SubtaskRepositoryImpl{
		client: client,
		logger: logger,
	}
}

// domainToEntSubtask maps domain subtask fields to ent SubtaskCreate.
func (r *SubtaskRepositoryImpl) domainToEntSubtaskCreate(s *domain.Subtask, create *ent.SubtaskCreate) *ent.SubtaskCreate {
	create = create.
		SetID(s.ID).
		SetTaskID(s.TaskID).
		SetType(string(s.Type)).
		SetDescription(s.Description).
		SetAgentTemplate(s.AgentTemplate).
		SetStatus(string(s.Status)).
		SetTokensUsed(s.TokensUsed).
		SetDependencies(s.Dependencies).
		SetCreatedAt(time.Now().UnixNano()).
		SetVersion(int64(s.Version))

	if s.AgentInstance != nil {
		create = create.SetAgentInstance(*s.AgentInstance)
	}
	if s.Summary != nil {
		create = create.SetSummary(*s.Summary)
	}
	if len(s.Artifacts) > 0 {
		// Convert []domain.ArtifactRef to []map[string]interface{}
		artifacts := make([]map[string]interface{}, len(s.Artifacts))
		for i, a := range s.Artifacts {
			artifacts[i] = map[string]interface{}{
				"id":        a.ID,
				"type":      a.Type,
				"summary":   a.Summary,
				"url":       a.URL,
				"size":      a.Size,
				"create_at": a.CreateAt.UnixNano(),
			}
		}
		create = create.SetArtifacts(artifacts)
	}
	if s.StartedAt != nil {
		create = create.SetStartedAt(*s.StartedAt)
	}
	if s.CompletedAt != nil {
		create = create.SetCompletedAt(*s.CompletedAt)
	}

	return create
}

// entToDomainSubtask maps an ent Subtask to a domain Subtask.
func (r *SubtaskRepositoryImpl) entToDomainSubtask(entSubtask *ent.Subtask) (*domain.Subtask, error) {
	if entSubtask == nil {
		return nil, nil
	}

	domSubtask := &domain.Subtask{
		TaskID:        entSubtask.TaskID,
		Type:          domain.SubtaskType(entSubtask.Type),
		Description:   entSubtask.Description,
		AgentTemplate: entSubtask.AgentTemplate,
		Status:        domain.TaskStatus(entSubtask.Status),
		TokensUsed:    entSubtask.TokensUsed,
		Dependencies:  entSubtask.Dependencies,
	}

	// Set embedded Entity fields
	domSubtask.ID = entSubtask.ID
	domSubtask.Version = entSubtask.Version

	if entSubtask.AgentInstance != "" {
		domSubtask.AgentInstance = &entSubtask.AgentInstance
	}
	if entSubtask.Summary != "" {
		domSubtask.Summary = &entSubtask.Summary
	}

	// Map Artifacts
	if len(entSubtask.Artifacts) > 0 {
		artifacts := make([]domain.ArtifactRef, len(entSubtask.Artifacts))
		for i := range entSubtask.Artifacts {
			m := entSubtask.Artifacts[i]
			artifacts[i] = domain.ArtifactRef{
				ID:      getStringFromMap(m, "id"),
				Type:    getStringFromMap(m, "type"),
				Summary: getStringFromMap(m, "summary"),
				URL:     getStringFromMap(m, "url"),
			}
			if size, ok := m["size"].(float64); ok {
				artifacts[i].Size = int64(size)
			}
			if createAt, ok := m["create_at"].(float64); ok {
				artifacts[i].CreateAt = time.Unix(0, int64(createAt)).UTC()
			}
		}
		domSubtask.Artifacts = artifacts
	}

	// Map time pointers
	if !entSubtask.StartedAt.IsZero() {
		domSubtask.StartedAt = &entSubtask.StartedAt
	}
	if !entSubtask.CompletedAt.IsZero() {
		domSubtask.CompletedAt = &entSubtask.CompletedAt
	}

	return domSubtask, nil
}

// getStringFromMap safely extracts a string from a map.
func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// Create creates a new Subtask and returns the created entity.
func (r *SubtaskRepositoryImpl) Create(ctx context.Context, s *domain.Subtask) (*domain.Subtask, error) {
	if s == nil {
		return nil, domain.NewL2ParamValidationError("subtask", "subtask is nil")
	}

	create := r.client.Subtask.Create()
	create = r.domainToEntSubtaskCreate(s, create)

	entSubtask, err := create.Save(ctx)
	if err != nil {
		r.logger.Error("failed to create subtask",
			zap.String("layer", "L1"),
			zap.String("subtask_id", s.ID),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	r.logger.Debug("subtask created",
		zap.String("layer", "L1"),
		zap.String("subtask_id", entSubtask.ID),
	)

	return r.entToDomainSubtask(entSubtask)
}

// GetByID retrieves a Subtask by its ID.
// Returns domain.ErrL2AggregateNotFound if the subtask does not exist.
func (r *SubtaskRepositoryImpl) GetByID(ctx context.Context, id string) (*domain.Subtask, error) {
	if id == "" {
		return nil, domain.NewL2ParamValidationError("id", "id is empty")
	}

	entSubtask, err := r.client.Subtask.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.NewL2AggregateNotFoundError("Subtask", id)
		}
		r.logger.Error("failed to get subtask",
			zap.String("layer", "L1"),
			zap.String("subtask_id", id),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	return r.entToDomainSubtask(entSubtask)
}

// Update updates an existing Subtask and returns the updated entity.
// Returns domain.ErrL2AggregateNotFound if the subtask does not exist.
// Returns domain.ErrL2OptimisticLock if the version doesn't match.
func (r *SubtaskRepositoryImpl) Update(ctx context.Context, s *domain.Subtask) (*domain.Subtask, error) {
	if s == nil {
		return nil, domain.NewL2ParamValidationError("subtask", "subtask is nil")
	}

	// First check if the subtask exists and get current version
	existing, err := r.client.Subtask.Get(ctx, s.ID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.NewL2AggregateNotFoundError("Subtask", s.ID)
		}
		return nil, domain.NewL1DBQueryError(err)
	}

	// Check optimistic lock
	if existing.Version != s.Version {
		return nil, domain.NewL2OptimisticLockError("Subtask", s.ID, s.Version, existing.Version)
	}

	// Build update mutation
	update := r.client.Subtask.UpdateOneID(s.ID).
		SetTaskID(s.TaskID).
		SetType(string(s.Type)).
		SetDescription(s.Description).
		SetAgentTemplate(s.AgentTemplate).
		SetStatus(string(s.Status)).
		SetTokensUsed(s.TokensUsed).
		SetDependencies(s.Dependencies).
		SetVersion(s.Version + 1)

	if s.AgentInstance != nil {
		update = update.SetAgentInstance(*s.AgentInstance)
	}
	if s.Summary != nil {
		update = update.SetSummary(*s.Summary)
	}
	if len(s.Artifacts) > 0 {
		artifacts := make([]map[string]interface{}, len(s.Artifacts))
		for i, a := range s.Artifacts {
			artifacts[i] = map[string]interface{}{
				"id":        a.ID,
				"type":      a.Type,
				"summary":   a.Summary,
				"url":       a.URL,
				"size":      a.Size,
				"create_at": a.CreateAt.UnixNano(),
			}
		}
		update = update.SetArtifacts(artifacts)
	}
	if s.StartedAt != nil {
		update = update.SetStartedAt(*s.StartedAt)
	}
	if s.CompletedAt != nil {
		update = update.SetCompletedAt(*s.CompletedAt)
	}

	entSubtask, err := update.Save(ctx)
	if err != nil {
		r.logger.Error("failed to update subtask",
			zap.String("layer", "L1"),
			zap.String("subtask_id", s.ID),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	r.logger.Debug("subtask updated",
		zap.String("layer", "L1"),
		zap.String("subtask_id", entSubtask.ID),
		zap.Int64("new_version", entSubtask.Version),
	)

	return r.entToDomainSubtask(entSubtask)
}

// Delete deletes a Subtask by its ID.
// Returns domain.ErrL2AggregateNotFound if the subtask does not exist.
func (r *SubtaskRepositoryImpl) Delete(ctx context.Context, id string) error {
	if id == "" {
		return domain.NewL2ParamValidationError("id", "id is empty")
	}

	err := r.client.Subtask.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.NewL2AggregateNotFoundError("Subtask", id)
		}
		r.logger.Error("failed to delete subtask",
			zap.String("layer", "L1"),
			zap.String("subtask_id", id),
			zap.Error(err),
		)
		return domain.NewL1DBQueryError(err)
	}

	r.logger.Debug("subtask deleted",
		zap.String("layer", "L1"),
		zap.String("subtask_id", id),
	)

	return nil
}

// ListByTaskID returns all Subtasks for a given Task.
func (r *SubtaskRepositoryImpl) ListByTaskID(ctx context.Context, taskID string) ([]*domain.Subtask, error) {
	entSubtasks, err := r.client.Subtask.Query().
		Where(subtask.TaskID(taskID)).
		Order(ent.Asc(subtask.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		r.logger.Error("failed to list subtasks by task_id",
			zap.String("layer", "L1"),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	domainSubtasks := make([]*domain.Subtask, len(entSubtasks))
	for i, entSubtask := range entSubtasks {
		domainSubtasks[i], err = r.entToDomainSubtask(entSubtask)
		if err != nil {
			return nil, err
		}
	}

	return domainSubtasks, nil
}

// UpdateStatus updates only the status and version of a Subtask.
// This is optimized for state machine transitions.
func (r *SubtaskRepositoryImpl) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Subtask, error) {
	if id == "" {
		return nil, domain.NewL2ParamValidationError("id", "id is empty")
	}

	// First check current version for optimistic lock
	existing, err := r.client.Subtask.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.NewL2AggregateNotFoundError("Subtask", id)
		}
		return nil, domain.NewL1DBQueryError(err)
	}

	if existing.Version != expectedVersion {
		return nil, domain.NewL2OptimisticLockError("Subtask", id, expectedVersion, existing.Version)
	}

	// Update only status and version
	entSubtask, err := r.client.Subtask.UpdateOneID(id).
		SetStatus(string(status)).
		SetVersion(expectedVersion + 1).
		Save(ctx)
	if err != nil {
		r.logger.Error("failed to update subtask status",
			zap.String("layer", "L1"),
			zap.String("subtask_id", id),
			zap.String("status", string(status)),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	return r.entToDomainSubtask(entSubtask)
}

// ListByStatus returns all Subtasks with the given status.
func (r *SubtaskRepositoryImpl) ListByStatus(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Subtask, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	query := r.client.Subtask.Query().
		Where(subtask.Status(string(status))).
		Order(ent.Desc(subtask.FieldCreatedAt)).
		Limit(limit).
		Offset(offset)

	entSubtasks, err := query.All(ctx)
	if err != nil {
		r.logger.Error("failed to list subtasks by status",
			zap.String("layer", "L1"),
			zap.String("status", string(status)),
			zap.Error(err),
		)
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	// Count total
	count, err := r.client.Subtask.Query().
		Where(subtask.Status(string(status))).
		Count(ctx)
	if err != nil {
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	domainSubtasks := make([]*domain.Subtask, len(entSubtasks))
	for i, entSubtask := range entSubtasks {
		domainSubtasks[i], err = r.entToDomainSubtask(entSubtask)
		if err != nil {
			return nil, 0, err
		}
	}

	return domainSubtasks, count, nil
}

// Verify interface implementation at compile time.
var _ domain.SubtaskRepository = (*SubtaskRepositoryImpl)(nil)
