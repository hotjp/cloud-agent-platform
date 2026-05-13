package persistence

import (
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/infra/outbox"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// ----------------------------------------------------------------------------
// Interface compliance tests
// ----------------------------------------------------------------------------

// TestOutboxRepositoryImplInterface verifies that OutboxRepositoryImpl implements domain.OutboxRepository.
func TestOutboxRepositoryImplInterface(t *testing.T) {
	var _ domain.OutboxRepository = (*OutboxRepositoryImpl)(nil)
}

// TestAuditLogRepositoryImplInterface verifies that AuditLogRepositoryImpl implements domain.AuditLogRepository.
func TestAuditLogRepositoryImplInterface(t *testing.T) {
	var _ domain.AuditLogRepository = (*AuditLogRepositoryImpl)(nil)
}

// ----------------------------------------------------------------------------
// Constructor validation tests
// ----------------------------------------------------------------------------

// TestNewOutboxRepositoryWithNilLogger tests that NewOutboxRepository panics with nil logger.
func TestNewOutboxRepositoryWithNilLogger(t *testing.T) {
	assert.Panics(t, func() {
		NewOutboxRepository(nil, nil)
	}, "NewOutboxRepository should panic with nil logger")
}

// TestNewAuditLogRepositoryWithNilLogger tests that NewAuditLogRepository panics with nil logger.
func TestNewAuditLogRepositoryWithNilLogger(t *testing.T) {
	assert.Panics(t, func() {
		NewAuditLogRepository(nil, nil)
	}, "NewAuditLogRepository should panic with nil logger")
}

// ----------------------------------------------------------------------------
// Repository instantiation tests
// ----------------------------------------------------------------------------

// TestOutboxRepositoryImplCanBeInstantiated tests that the repository can be instantiated.
func TestOutboxRepositoryImplCanBeInstantiated(t *testing.T) {
	logger := zaptest.NewLogger(t)
	repo := NewOutboxRepository(nil, logger)

	assert.NotNil(t, repo, "NewOutboxRepository should return non-nil repo")
	assert.Equal(t, logger, repo.logger, "logger should be set correctly")
}

// TestAuditLogRepositoryImplCanBeInstantiated tests that the repository can be instantiated.
func TestAuditLogRepositoryImplCanBeInstantiated(t *testing.T) {
	logger := zaptest.NewLogger(t)
	repo := NewAuditLogRepository(nil, logger)

	assert.NotNil(t, repo, "NewAuditLogRepository should return non-nil repo")
	assert.Equal(t, logger, repo.logger, "logger should be set correctly")
}

// ----------------------------------------------------------------------------
// Mapping helper tests
// ----------------------------------------------------------------------------

// TestEntToDomainOutboxEventMapping tests the ent to domain outbox event mapping.
func TestEntToDomainOutboxEventMapping(t *testing.T) {
	logger := zaptest.NewLogger(t)
	repo := NewOutboxRepository(nil, logger)

	// Test nil input
	result := repo.entToDomainOutboxEvent(nil)
	assert.Nil(t, result, "entToDomainOutboxEvent should return nil for nil input")
}

// TestEntToDomainAuditLogMapping tests the ent to domain audit log mapping.
func TestEntToDomainAuditLogMapping(t *testing.T) {
	logger := zaptest.NewLogger(t)
	repo := NewAuditLogRepository(nil, logger)

	// Test nil input
	result, err := repo.entToDomainAuditLog(nil)
	assert.NoError(t, err)
	assert.Nil(t, result, "entToDomainAuditLog should return nil for nil input")
}

// ----------------------------------------------------------------------------
// Domain model creation tests
// ----------------------------------------------------------------------------

// TestDomainOutboxEventMapping tests domain OutboxEvent creation.
func TestDomainOutboxEventMapping(t *testing.T) {
	payload := []byte(`{"key": "value"}`)

	domEvent := &domain.OutboxEvent{
		ID:            "event-123",
		AggregateType: "Task",
		AggregateID:   "task-456",
		EventType:     "TaskCreatedV1",
		Payload:       payload,
		OccurredAt:    time.Now().UnixNano(),
		Status:        "pending",
		RetryCount:    0,
		LastError:     "",
		CreatedAt:     time.Now().UnixNano(),
	}

	assert.Equal(t, "event-123", domEvent.ID)
	assert.Equal(t, "Task", domEvent.AggregateType)
	assert.Equal(t, "task-456", domEvent.AggregateID)
	assert.Equal(t, "TaskCreatedV1", domEvent.EventType)
	assert.Equal(t, "pending", domEvent.Status)
}

// TestDomainAuditLogMapping tests domain AuditLog creation.
func TestDomainAuditLogMapping(t *testing.T) {
	now := time.Now().UTC()
	subtaskID := "subtask-789"
	agentTemplate := "executor"

	domLog := &domain.AuditLog{
		ID:            "log-123",
		TaskID:        "task-456",
		SubtaskID:     &subtaskID,
		AgentTemplate: &agentTemplate,
		Action:        "SubtaskStarted",
		Level:         "info",
		Message:       "Subtask started execution",
		Details: map[string]any{
			"agent_id": "agent-001",
		},
		Timestamp: now,
	}

	assert.Equal(t, "log-123", domLog.ID)
	assert.Equal(t, "task-456", domLog.TaskID)
	assert.NotNil(t, domLog.SubtaskID)
	assert.Equal(t, "subtask-789", *domLog.SubtaskID)
	assert.NotNil(t, domLog.AgentTemplate)
	assert.Equal(t, "executor", *domLog.AgentTemplate)
	assert.Equal(t, "SubtaskStarted", domLog.Action)
	assert.Equal(t, "info", domLog.Level)
	assert.Equal(t, "Subtask started execution", domLog.Message)
	assert.Equal(t, "agent-001", domLog.Details["agent_id"])
}

// TestDomainNewAuditLog tests the NewAuditLog constructor.
func TestDomainNewAuditLog(t *testing.T) {
	log := domain.NewAuditLog("log-123", "task-456", "TaskCreated", "Task was created", "info")

	assert.Equal(t, "log-123", log.ID)
	assert.Equal(t, "task-456", log.TaskID)
	assert.Equal(t, "TaskCreated", log.Action)
	assert.Equal(t, "info", log.Level)
	assert.Equal(t, "Task was created", log.Message)
	assert.NotNil(t, log.Details)
	assert.False(t, log.Timestamp.IsZero())
}

// TestDomainAuditLogWithSubtask tests the WithSubtask builder method.
func TestDomainAuditLogWithSubtask(t *testing.T) {
	log := domain.NewAuditLog("log-123", "task-456", "SubtaskStarted", "Subtask started", "info")
	log.WithSubtask("subtask-789")

	assert.NotNil(t, log.SubtaskID)
	assert.Equal(t, "subtask-789", *log.SubtaskID)
}

// TestDomainAuditLogWithAgentTemplate tests the WithAgentTemplate builder method.
func TestDomainAuditLogWithAgentTemplate(t *testing.T) {
	log := domain.NewAuditLog("log-123", "task-456", "AgentAssigned", "Agent assigned", "info")
	log.WithAgentTemplate("executor")

	assert.NotNil(t, log.AgentTemplate)
	assert.Equal(t, "executor", *log.AgentTemplate)
}

// TestDomainAuditLogWithDetail tests the WithDetail builder method.
func TestDomainAuditLogWithDetail(t *testing.T) {
	log := domain.NewAuditLog("log-123", "task-456", "TaskProgress", "Progress updated", "info")
	log.WithDetail("progress", 50.0)
	log.WithDetail("message", "halfway done")

	assert.Equal(t, 50.0, log.Details["progress"])
	assert.Equal(t, "halfway done", log.Details["message"])
}

// ----------------------------------------------------------------------------
// Status constant tests
// ----------------------------------------------------------------------------

// TestOutboxStatusConstants tests that status constants are defined correctly.
func TestOutboxStatusConstants(t *testing.T) {
	assert.Equal(t, "pending", outbox.StatusPending)
	assert.Equal(t, "published", outbox.StatusPublished)
	assert.Equal(t, "failed", outbox.StatusFailed)
}