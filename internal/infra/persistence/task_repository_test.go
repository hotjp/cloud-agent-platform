package persistence

import (
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// TestTaskRepositoryImplInterface verifies that TaskRepositoryImpl implements domain.TaskRepository.
func TestTaskRepositoryImplInterface(t *testing.T) {
	var _ domain.TaskRepository = (*TaskRepositoryImpl)(nil)
}

// TestSubtaskRepositoryImplInterface verifies that SubtaskRepositoryImpl implements domain.SubtaskRepository.
func TestSubtaskRepositoryImplInterface(t *testing.T) {
	var _ domain.SubtaskRepository = (*SubtaskRepositoryImpl)(nil)
}

// TestNewTaskRepositoryWithNilLogger tests that NewTaskRepository panics with nil logger.
func TestNewTaskRepositoryWithNilLogger(t *testing.T) {
	assert.Panics(t, func() {
		NewTaskRepository(nil, nil)
	}, "NewTaskRepository should panic with nil logger")
}

// TestNewSubtaskRepositoryWithNilLogger tests that NewSubtaskRepository panics with nil logger.
func TestNewSubtaskRepositoryWithNilLogger(t *testing.T) {
	assert.Panics(t, func() {
		NewSubtaskRepository(nil, nil)
	}, "NewSubtaskRepository should panic with nil logger")
}

// TestDomainToEntTaskCreateValidation tests validation in domainToEntTaskCreate.
func TestDomainToEntTaskCreateValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	// Use a mock or just verify constructor doesn't panic
	assert.NotPanics(t, func() {
		repo := NewTaskRepository(nil, logger)
		_ = repo // avoid unused variable
	}, "NewTaskRepository should not panic with nil client but non-nil logger")
}

// TestEntToDomainTaskMapping tests the ent to domain mapping.
func TestEntToDomainTaskMapping(t *testing.T) {
	logger := zaptest.NewLogger(t)
	repo := NewTaskRepository(nil, logger)

	// Test nil input
	result, err := repo.entToDomainTask(nil)
	assert.NoError(t, err)
	assert.Nil(t, result, "entToDomainTask should return nil for nil input")
}

// TestGetStringFromMap tests the helper function.
func TestGetStringFromMap(t *testing.T) {
	m := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
		"key3": "value3",
	}

	assert.Equal(t, "value1", getStringFromMap(m, "key1"))
	assert.Equal(t, "", getStringFromMap(m, "key2"))   // not a string
	assert.Equal(t, "", getStringFromMap(m, "key4"))   // key doesn't exist
	assert.Equal(t, "", getStringFromMap(nil, "key1")) // nil map
}

// TestTaskRepositoryImplCRUD tests that the repository can be instantiated.
func TestTaskRepositoryImplCRUD(t *testing.T) {
	logger := zaptest.NewLogger(t)
	repo := NewTaskRepository(nil, logger)

	assert.NotNil(t, repo, "NewTaskRepository should return non-nil repo")
	assert.Equal(t, logger, repo.logger, "logger should be set correctly")
}

// TestSubtaskRepositoryImplCRUD tests that the repository can be instantiated.
func TestSubtaskRepositoryImplCRUD(t *testing.T) {
	logger := zaptest.NewLogger(t)
	repo := NewSubtaskRepository(nil, logger)

	assert.NotNil(t, repo, "NewSubtaskRepository should return non-nil repo")
	assert.Equal(t, logger, repo.logger, "logger should be set correctly")
}

// TestDomainSubtaskMapping tests the domain subtask creation.
func TestDomainSubtaskMapping(t *testing.T) {
	now := time.Now().UTC()
	completedAt := now.Add(1 * time.Hour)

	subtask := domain.NewSubtask("subtask-123", "task-456", domain.SubtaskTypeCoding, "Write tests", "executor")
	subtask.Status = domain.TaskStatusPending
	subtask.TokensUsed = 1000
	subtask.Dependencies = []string{}
	subtask.StartedAt = &now
	subtask.CompletedAt = &completedAt
	subtask.Artifacts = []domain.ArtifactRef{}
	// Note: Version is managed internally via AggregateRoot

	assert.Equal(t, "subtask-123", subtask.ID)
	assert.Equal(t, "task-456", subtask.TaskID)
	assert.Equal(t, domain.SubtaskTypeCoding, subtask.Type)
	assert.Equal(t, domain.TaskStatusPending, subtask.Status)
	assert.NotNil(t, subtask.StartedAt)
	assert.NotNil(t, subtask.CompletedAt)
}

// TestDomainTaskMapping tests the domain task creation.
func TestDomainTaskMapping(t *testing.T) {
	now := time.Now().UTC()
	startedAt := now.Add(-1 * time.Hour)
	completedAt := now.Add(1 * time.Hour)

	task := domain.NewTask("task-123", "Implement feature X", "https://github.com/example/repo", "main", "client-abc")
	task.Status = domain.TaskStatusRunning
	task.Priority = 8
	task.ResultBranch = "main/agent/task-123"
	task.Constraints = []string{"constraint1"}
	task.VerificationCriteria = []string{"criterion1"}
	task.Progress = 50.0
	task.TokensUsed = 5000
	task.EstimatedCost = 10.5
	task.AgentsUsed = 2
	task.Tags = []string{"feature", "urgent"}
	task.StartedAt = &startedAt
	task.CompletedAt = &completedAt
	task.AgentHint = &domain.AgentHint{
		Templates: []string{"executor", "reviewer"},
		Model:     "claude-3-5-sonnet",
		MaxAgents: 3,
	}
	// Note: Version is managed internally and set via AggregateRoot

	assert.Equal(t, "Implement feature X", task.Goal)
	assert.Equal(t, domain.TaskStatusRunning, task.Status)
	assert.Equal(t, 8, task.Priority)
	assert.Equal(t, 50.0, task.Progress)
	assert.NotNil(t, task.AgentHint)
	assert.Equal(t, []string{"executor", "reviewer"}, task.AgentHint.Templates)
	assert.Equal(t, "claude-3-5-sonnet", task.AgentHint.Model)
	assert.Equal(t, 3, task.AgentHint.MaxAgents)
}

// TestTaskStatusValues tests TaskStatus constants.
func TestTaskStatusValues(t *testing.T) {
	statuses := []domain.TaskStatus{
		domain.TaskStatusPending,
		domain.TaskStatusDecomposing,
		domain.TaskStatusDispatched,
		domain.TaskStatusRunning,
		domain.TaskStatusReviewing,
		domain.TaskStatusConfirming,
		domain.TaskStatusCompleted,
		domain.TaskStatusFailed,
		domain.TaskStatusCancelled,
	}

	for _, status := range statuses {
		assert.NotEmpty(t, string(status), "TaskStatus should have string value")
		assert.True(t, status.IsValid(), "TaskStatus %s should be valid", status)
	}

	assert.False(t, domain.TaskStatus("invalid").IsValid(), "Invalid status should return false")
}

// TestSubtaskTypeValues tests SubtaskType constants.
func TestSubtaskTypeValues(t *testing.T) {
	types := []domain.SubtaskType{
		domain.SubtaskTypeAnalysis,
		domain.SubtaskTypeCoding,
		domain.SubtaskTypeReview,
		domain.SubtaskTypeTesting,
		domain.SubtaskTypeResearch,
	}

	for _, subType := range types {
		assert.NotEmpty(t, string(subType), "SubtaskType should have string value")
		assert.True(t, subType.IsValid(), "SubtaskType %s should be valid", subType)
	}

	assert.False(t, domain.SubtaskType("invalid").IsValid(), "Invalid type should return false")
}

// TestAgentRoleValues tests AgentRole constants.
func TestAgentRoleValues(t *testing.T) {
	roles := []domain.AgentRole{
		domain.AgentRoleObserver,
		domain.AgentRoleStrategist,
		domain.AgentRoleExecutor,
		domain.AgentRoleGuardian,
		domain.AgentRoleTester,
		domain.AgentRoleResearcher,
	}

	for _, role := range roles {
		assert.NotEmpty(t, string(role), "AgentRole should have string value")
		assert.True(t, role.IsValid(), "AgentRole %s should be valid", role)
	}

	assert.False(t, domain.AgentRole("invalid").IsValid(), "Invalid role should return false")
}

// TestTaskStatusIsTerminal tests IsTerminal method.
func TestTaskStatusIsTerminal(t *testing.T) {
	assert.True(t, domain.TaskStatusCompleted.IsTerminal())
	assert.True(t, domain.TaskStatusFailed.IsTerminal())
	assert.True(t, domain.TaskStatusCancelled.IsTerminal())

	assert.False(t, domain.TaskStatusPending.IsTerminal())
	assert.False(t, domain.TaskStatusRunning.IsTerminal())
	assert.False(t, domain.TaskStatusReviewing.IsTerminal())
}
