// Package persistence implements L1-Storage repository interfaces using ent ORM.
package persistence

import (
	"context"
	"time"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/ent/auditlog"
	"github.com/cloud-agent-platform/cap/internal/domain"

	"go.uber.org/zap"
)

// AuditLogRepositoryImpl implements domain.AuditLogRepository using ent ORM.
type AuditLogRepositoryImpl struct {
	client *ent.Client
	logger *zap.Logger
}

// NewAuditLogRepository creates a new AuditLogRepositoryImpl.
func NewAuditLogRepository(client *ent.Client, logger *zap.Logger) *AuditLogRepositoryImpl {
	if logger == nil {
		panic("logger is required")
	}
	return &AuditLogRepositoryImpl{
		client: client,
		logger: logger,
	}
}

// Create creates a new AuditLog entry and returns the created entity.
func (r *AuditLogRepositoryImpl) Create(ctx context.Context, log *domain.AuditLog) (*domain.AuditLog, error) {
	if log == nil {
		return nil, domain.NewL2ParamValidationError("log", "log is nil")
	}

	create := r.client.AuditLog.Create().
		SetID(log.ID).
		SetTaskID(log.TaskID).
		SetAction(log.Action).
		SetLevel(log.Level).
		SetMessage(log.Message).
		SetTimestamp(log.Timestamp.UnixNano())

	if log.SubtaskID != nil {
		create = create.SetSubtaskID(*log.SubtaskID)
	}
	if log.AgentTemplate != nil {
		create = create.SetAgentTemplate(*log.AgentTemplate)
	}
	if len(log.Details) > 0 {
		create = create.SetDetails(log.Details)
	}

	entLog, err := create.Save(ctx)
	if err != nil {
		r.logger.Error("failed to create audit log",
			zap.String("layer", "L1"),
			zap.String("log_id", log.ID),
			zap.String("task_id", log.TaskID),
			zap.Error(err),
		)
		return nil, domain.NewL1DBQueryError(err)
	}

	r.logger.Debug("audit log created",
		zap.String("layer", "L1"),
		zap.String("log_id", entLog.ID),
		zap.String("task_id", entLog.TaskID),
	)

	return r.entToDomainAuditLog(entLog)
}

// ListByTaskID returns all AuditLogs for a given Task.
func (r *AuditLogRepositoryImpl) ListByTaskID(ctx context.Context, taskID string, limit, offset int) ([]*domain.AuditLog, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	entLogs, err := r.client.AuditLog.Query().
		Where(auditlog.TaskID(taskID)).
		Order(ent.Desc(auditlog.FieldTimestamp)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		r.logger.Error("failed to list audit logs by task_id",
			zap.String("layer", "L1"),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	// Count total
	count, err := r.client.AuditLog.Query().
		Where(auditlog.TaskID(taskID)).
		Count(ctx)
	if err != nil {
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	domainLogs := make([]*domain.AuditLog, len(entLogs))
	for i, entLog := range entLogs {
		domainLogs[i], err = r.entToDomainAuditLog(entLog)
		if err != nil {
			return nil, 0, err
		}
	}

	return domainLogs, count, nil
}

// ListBySubtaskID returns all AuditLogs for a given Subtask.
func (r *AuditLogRepositoryImpl) ListBySubtaskID(ctx context.Context, subtaskID string, limit, offset int) ([]*domain.AuditLog, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	entLogs, err := r.client.AuditLog.Query().
		Where(auditlog.SubtaskID(subtaskID)).
		Order(ent.Desc(auditlog.FieldTimestamp)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		r.logger.Error("failed to list audit logs by subtask_id",
			zap.String("layer", "L1"),
			zap.String("subtask_id", subtaskID),
			zap.Error(err),
		)
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	// Count total
	count, err := r.client.AuditLog.Query().
		Where(auditlog.SubtaskID(subtaskID)).
		Count(ctx)
	if err != nil {
		return nil, 0, domain.NewL1DBQueryError(err)
	}

	domainLogs := make([]*domain.AuditLog, len(entLogs))
	for i, entLog := range entLogs {
		domainLogs[i], err = r.entToDomainAuditLog(entLog)
		if err != nil {
			return nil, 0, err
		}
	}

	return domainLogs, count, nil
}

// entToDomainAuditLog maps an ent AuditLog to a domain AuditLog.
func (r *AuditLogRepositoryImpl) entToDomainAuditLog(entLog *ent.AuditLog) (*domain.AuditLog, error) {
	if entLog == nil {
		return nil, nil
	}

	domLog := &domain.AuditLog{
		ID:        entLog.ID,
		TaskID:    entLog.TaskID,
		Action:    entLog.Action,
		Level:     entLog.Level,
		Message:   entLog.Message,
		Details:   entLog.Details,
		Timestamp: time.Unix(0, entLog.Timestamp).UTC(),
	}

	if entLog.SubtaskID != "" {
		domLog.SubtaskID = &entLog.SubtaskID
	}
	if entLog.AgentTemplate != "" {
		domLog.AgentTemplate = &entLog.AgentTemplate
	}

	return domLog, nil
}

// Verify interface implementation at compile time.
var _ domain.AuditLogRepository = (*AuditLogRepositoryImpl)(nil)