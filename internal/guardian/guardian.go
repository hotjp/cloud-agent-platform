// Package guardian implements the Guardian approval mechanism for high-risk operations.
// It manages the confirming state, timeout mechanism, and approval workflow.
package guardian

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cloud-agent-platform/cap/internal/config"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"go.uber.org/zap"
)

// Guardian manages approval requests and timeout handling.
type Guardian struct {
	cfg        ApprovalConfig
	logger     *zap.Logger
	wsPusher   WSPusher
	pendingReq map[string]*ApprovalRequest
	mu         sync.RWMutex
}

// NewGuardian creates a new Guardian instance.
func NewGuardian(cfg config.ApprovalConfig, logger *zap.Logger, wsPusher WSPusher) *Guardian {
	approvalCfg := ApprovalConfig{
		DefaultTimeout:       cfg.DefaultTimeout,
		MaxTimeout:           cfg.MaxTimeout,
		HighRiskCostThreshold: cfg.HighRiskCostThreshold,
	}
	if approvalCfg.DefaultTimeout == 0 {
		approvalCfg.DefaultTimeout = 5 * time.Minute
	}
	if approvalCfg.MaxTimeout == 0 {
		approvalCfg.MaxTimeout = 30 * time.Minute
	}
	if approvalCfg.HighRiskCostThreshold == 0 {
		approvalCfg.HighRiskCostThreshold = 1.0
	}

	return &Guardian{
		cfg:        approvalCfg,
		logger:     logger,
		wsPusher:   wsPusher,
		pendingReq: make(map[string]*ApprovalRequest),
	}
}

// NeedsApproval checks if an operation requires approval based on risk assessment.
func (g *Guardian) NeedsApproval(ctx context.Context, task *domain.Task) bool {
	if task == nil {
		return false
	}

	// Check if task has requireApproval flag set
	// This could be a field on the task or determined by other means

	// Check if estimated cost exceeds threshold
	if task.EstimatedCost > g.cfg.HighRiskCostThreshold {
		return true
	}

	return false
}

// EvaluateRiskLevel evaluates the risk level of an operation.
func (g *Guardian) EvaluateRiskLevel(task *domain.Task) RiskLevel {
	if task == nil {
		return RiskLevelLow
	}

	// High cost operations
	if task.EstimatedCost > 100 {
		return RiskLevelHigh
	}
	if task.EstimatedCost > 10 {
		return RiskLevelMedium
	}

	return RiskLevelLow
}

// RequestApproval creates an approval request and transitions the task to confirming state.
// It pushes the approval request via WebSocket and sets up the timeout handler.
func (g *Guardian) RequestApproval(ctx context.Context, task *domain.Task, customTimeout time.Duration) (*ApprovalRequest, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	// Determine timeout
	timeout := g.cfg.DefaultTimeout
	if customTimeout > 0 {
		if customTimeout > g.cfg.MaxTimeout {
			timeout = g.cfg.MaxTimeout
		} else {
			timeout = customTimeout
		}
	}

	now := time.Now().UTC()
	// Create a result channel for blocking wait
	resultCh := make(chan ApprovalResult, 1)

	req := &ApprovalRequest{
		TaskID:         task.ID,
		TaskGoal:       task.Goal,
		EstimatedCost:  task.EstimatedCost,
		RiskLevel:      g.EvaluateRiskLevel(task),
		RequestedAt:    now,
		ExpiresAt:      now.Add(timeout),
		RequireApproval: true,
		Timeout:        timeout,
		ResultCh:       resultCh,
	}

	// Store pending request
	g.mu.Lock()
	g.pendingReq[task.ID] = req
	g.mu.Unlock()

	// Push approval required event via WebSocket
	if g.wsPusher != nil {
		if err := g.wsPusher.PushApprovalRequired(ctx, task.ID, req.TaskGoal, req.EstimatedCost, string(req.RiskLevel), req.RequestedAt, req.ExpiresAt, req.Timeout); err != nil {
			g.logger.Error("failed to push approval required event",
				zap.String("layer", "L4"),
				zap.String("task_id", task.ID),
				zap.Error(err),
			)
			// Don't fail the request, just log the error
		}
	}

	g.logger.Info("approval requested",
		zap.String("layer", "L4"),
		zap.String("task_id", task.ID),
		zap.Float64("estimated_cost", task.EstimatedCost),
		zap.String("risk_level", string(req.RiskLevel)),
		zap.Duration("timeout", timeout),
	)

	return req, nil
}

// GetApprovalRequest retrieves a pending approval request by task ID.
func (g *Guardian) GetApprovalRequest(ctx context.Context, taskID string) (*ApprovalRequest, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	req, ok := g.pendingReq[taskID]
	if !ok {
		return nil, fmt.Errorf("approval request not found for task: %s", taskID)
	}

	return req, nil
}

// ProcessApproval processes an approval decision and returns the appropriate domain event.
// It signals the result to any waiting caller via the result channel.
func (g *Guardian) ProcessApproval(ctx context.Context, decision *ApprovalDecision) (*domain.DomainEvent, error) {
	if decision == nil {
		return nil, fmt.Errorf("decision is nil")
	}

	// Get the pending request and result channel before deleting
	g.mu.Lock()
	req, exists := g.pendingReq[decision.TaskID]
	if exists {
		delete(g.pendingReq, decision.TaskID)
	}
	g.mu.Unlock()

	// Signal the result to any waiting caller
	if req != nil && req.ResultCh != nil {
		select {
		case req.ResultCh <- decision.Result:
		default:
			// Channel already has a value or is closed
		}
		close(req.ResultCh)
	}

	// Create domain event based on decision
	var eventType string
	var payload ApprovalResultPayload

	switch decision.Result {
	case ApprovalResultApproved:
		eventType = "TaskUserApprovedV1"
		payload = ApprovalResultPayload{
			TaskID:    decision.TaskID,
			Result:    string(ApprovalResultApproved),
			Reason:    decision.Reason,
			DecidedBy: decision.DecidedBy,
			DecidedAt: decision.DecidedAt,
		}
	case ApprovalResultRejected:
		eventType = "TaskUserRejectedV1"
		payload = ApprovalResultPayload{
			TaskID:    decision.TaskID,
			Result:    string(ApprovalResultRejected),
			Reason:    decision.Reason,
			DecidedBy: decision.DecidedBy,
			DecidedAt: decision.DecidedAt,
		}
	case ApprovalResultTimeout:
		eventType = "TaskConfirmationTimeoutV1"
		payload = ApprovalResultPayload{
			TaskID:    decision.TaskID,
			Result:    string(ApprovalResultTimeout),
			Reason:    "approval timeout",
			DecidedBy: "system",
			DecidedAt: decision.DecidedAt,
		}
	case ApprovalResultEscalated:
		eventType = "TaskEscalatedV1"
		payload = ApprovalResultPayload{
			TaskID:    decision.TaskID,
			Result:    string(ApprovalResultEscalated),
			Reason:    decision.Reason,
			DecidedBy: decision.DecidedBy,
			DecidedAt: decision.DecidedAt,
		}
	default:
		return nil, fmt.Errorf("unknown approval result: %s", decision.Result)
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal approval result payload: %w", err)
	}

	event, err := domain.NewDomainEvent("Task", decision.TaskID, eventType, payloadBytes, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to create domain event: %w", err)
	}

	g.logger.Info("approval processed",
		zap.String("layer", "L4"),
		zap.String("task_id", decision.TaskID),
		zap.String("result", string(decision.Result)),
		zap.String("decided_by", decision.DecidedBy),
	)

	return event, nil
}

// HandleTimeout handles an approval timeout for a task.
func (g *Guardian) HandleTimeout(ctx context.Context, taskID string) (*domain.DomainEvent, error) {
	decision := &ApprovalDecision{
		TaskID:    taskID,
		Result:    ApprovalResultTimeout,
		Reason:    "approval timeout exceeded",
		DecidedBy: "system",
		DecidedAt: time.Now().UTC(),
	}

	return g.ProcessApproval(ctx, decision)
}

// IsPending checks if there is a pending approval request for a task.
func (g *Guardian) IsPending(ctx context.Context, taskID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.pendingReq[taskID]
	return ok
}

// ApprovalResultPayload is the payload for approval result events.
type ApprovalResultPayload struct {
	TaskID    string    `json:"task_id"`
	Result    string    `json:"result"`
	Reason    string    `json:"reason"`
	DecidedBy string    `json:"decided_by"`
	DecidedAt time.Time `json:"decided_at"`
}