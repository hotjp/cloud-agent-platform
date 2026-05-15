// Package guardian implements the Guardian approval mechanism for high-risk operations.
package guardian

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/config"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockWSPusher is a mock implementation of WSPusher for testing.
type mockWSPusher struct {
	pushCalled        bool
	pushTaskID        string
	pushTaskGoal      string
	pushEstimatedCost float64
	pushRiskLevel    string
	pushError         error
}

func (m *mockWSPusher) PushApprovalRequired(ctx context.Context, taskID string, taskGoal string, estimatedCost float64, riskLevel string, requestedAt, expiresAt time.Time, timeout time.Duration) error {
	m.pushCalled = true
	m.pushTaskID = taskID
	m.pushTaskGoal = taskGoal
	m.pushEstimatedCost = estimatedCost
	m.pushRiskLevel = riskLevel
	return m.pushError
}

func newTestTask(id, goal string, estimatedCost float64) *domain.Task {
	task := domain.NewTask(id, goal, "https://github.com/test/repo", "main", "test-client")
	task.EstimatedCost = estimatedCost
	return task
}

func TestNewGuardian(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}

	g := NewGuardian(cfg, logger, mockWS)

	assert.NotNil(t, g)
	assert.Equal(t, 5*time.Minute, g.cfg.DefaultTimeout)
	assert.Equal(t, 30*time.Minute, g.cfg.MaxTimeout)
	assert.Equal(t, 1.0, g.cfg.HighRiskCostThreshold)
}

func TestGuardian_NeedsApproval(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	tests := []struct {
		name     string
		task     *domain.Task
		expected bool
	}{
		{
			name:     "nil task",
			task:     nil,
			expected: false,
		},
		{
			name: "cost below threshold",
			task: newTestTask("task1", "test task", 0.5),
			expected: false,
		},
		{
			name: "cost at threshold",
			task: newTestTask("task2", "test task", 1.0),
			expected: false,
		},
		{
			name: "cost above threshold",
			task: newTestTask("task3", "test task", 1.5),
			expected: true,
		},
		{
			name: "high cost task",
			task: newTestTask("task4", "high risk task", 100.0),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := g.NeedsApproval(context.Background(), tt.task)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGuardian_EvaluateRiskLevel(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	tests := []struct {
		name     string
		task     *domain.Task
		expected RiskLevel
	}{
		{
			name:     "nil task",
			task:     nil,
			expected: RiskLevelLow,
		},
		{
			name: "low cost",
			task: newTestTask("task1", "test task", 5.0),
			expected: RiskLevelLow,
		},
		{
			name: "medium cost",
			task: newTestTask("task2", "test task", 50.0),
			expected: RiskLevelMedium,
		},
		{
			name: "high cost",
			task: newTestTask("task3", "test task", 150.0),
			expected: RiskLevelHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := g.EvaluateRiskLevel(tt.task)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGuardian_RequestApproval(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	task := newTestTask("task1", "test task requiring approval", 50.0)

	ctx := context.Background()
	req, err := g.RequestApproval(ctx, task, 0)

	require.NoError(t, err)
	assert.NotNil(t, req)
	assert.Equal(t, task.ID, req.TaskID)
	assert.Equal(t, task.Goal, req.TaskGoal)
	assert.Equal(t, task.EstimatedCost, req.EstimatedCost)
	assert.Equal(t, RiskLevelMedium, req.RiskLevel)
	assert.True(t, req.RequireApproval)
	assert.Equal(t, 5*time.Minute, req.Timeout)
	assert.True(t, mockWS.pushCalled)
	assert.Equal(t, task.ID, mockWS.pushTaskID)
	assert.Equal(t, task.Goal, mockWS.pushTaskGoal)
	assert.Equal(t, task.EstimatedCost, mockWS.pushEstimatedCost)
}

func TestGuardian_RequestApproval_WithCustomTimeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	task := newTestTask("task1", "test task", 50.0)

	ctx := context.Background()
	req, err := g.RequestApproval(ctx, task, 10*time.Minute)

	require.NoError(t, err)
	assert.NotNil(t, req)
	assert.Equal(t, 10*time.Minute, req.Timeout)

	// Verify timeout is capped at max
	req2, err := g.RequestApproval(ctx, task, 60*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Minute, req2.Timeout) // capped at max
}

func TestGuardian_RequestApproval_NilTask(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	ctx := context.Background()
	req, err := g.RequestApproval(ctx, nil, 0)

	assert.Error(t, err)
	assert.Nil(t, req)
	assert.Contains(t, err.Error(), "task is nil")
}

func TestGuardian_GetApprovalRequest(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	task := newTestTask("task1", "test task", 50.0)

	ctx := context.Background()

	// Get non-existent request
	_, err := g.GetApprovalRequest(ctx, "nonexistent")
	assert.Error(t, err)

	// Create request
	req, err := g.RequestApproval(ctx, task, 0)
	require.NoError(t, err)

	// Get the request
	retrieved, err := g.GetApprovalRequest(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, req.TaskID, retrieved.TaskID)
	assert.Equal(t, req.TaskGoal, retrieved.TaskGoal)
}

func TestGuardian_ProcessApproval_Approved(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	task := newTestTask("task1", "test task", 50.0)

	ctx := context.Background()
	_, err := g.RequestApproval(ctx, task, 0)
	require.NoError(t, err)

	decision := &ApprovalDecision{
		TaskID:    task.ID,
		Result:    ApprovalResultApproved,
		Reason:    "looks good",
		DecidedBy: "user123",
		DecidedAt: time.Now().UTC(),
	}

	event, err := g.ProcessApproval(ctx, decision)
	require.NoError(t, err)
	assert.NotNil(t, event)
	assert.Equal(t, "Task", event.AggregateType)
	assert.Equal(t, task.ID, event.AggregateID)
	assert.Equal(t, "TaskUserApprovedV1", event.EventType)
	assert.False(t, g.IsPending(ctx, task.ID)) // Should be removed from pending
}

func TestGuardian_ProcessApproval_Rejected(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	task := newTestTask("task1", "test task", 50.0)

	ctx := context.Background()
	_, err := g.RequestApproval(ctx, task, 0)
	require.NoError(t, err)

	decision := &ApprovalDecision{
		TaskID:    task.ID,
		Result:    ApprovalResultRejected,
		Reason:    "too risky",
		DecidedBy: "user123",
		DecidedAt: time.Now().UTC(),
	}

	event, err := g.ProcessApproval(ctx, decision)
	require.NoError(t, err)
	assert.NotNil(t, event)
	assert.Equal(t, "TaskUserRejectedV1", event.EventType)
	assert.False(t, g.IsPending(ctx, task.ID))
}

func TestGuardian_ProcessApproval_Timeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	task := newTestTask("task1", "test task", 50.0)

	ctx := context.Background()
	_, err := g.RequestApproval(ctx, task, 0)
	require.NoError(t, err)

	event, err := g.HandleTimeout(ctx, task.ID)
	require.NoError(t, err)
	assert.NotNil(t, event)
	assert.Equal(t, "TaskConfirmationTimeoutV1", event.EventType)
	assert.False(t, g.IsPending(ctx, task.ID))
}

func TestGuardian_ProcessApproval_NilDecision(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	ctx := context.Background()
	event, err := g.ProcessApproval(ctx, nil)

	assert.Error(t, err)
	assert.Nil(t, event)
	assert.Contains(t, err.Error(), "decision is nil")
}

func TestGuardian_IsPending(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockWS := &mockWSPusher{}
	cfg := config.ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
	g := NewGuardian(cfg, logger, mockWS)

	task := newTestTask("task1", "test task", 50.0)

	ctx := context.Background()

	assert.False(t, g.IsPending(ctx, task.ID))

	_, err := g.RequestApproval(ctx, task, 0)
	require.NoError(t, err)
	assert.True(t, g.IsPending(ctx, task.ID))

	decision := &ApprovalDecision{
		TaskID:    task.ID,
		Result:    ApprovalResultApproved,
		Reason:    "approved",
		DecidedBy: "user",
		DecidedAt: time.Now().UTC(),
	}
	_, err = g.ProcessApproval(ctx, decision)
	require.NoError(t, err)
	assert.False(t, g.IsPending(ctx, task.ID))
}