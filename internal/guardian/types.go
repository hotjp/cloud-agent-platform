// Package guardian implements the Guardian approval mechanism for high-risk operations.
// It manages the confirming state, timeout mechanism, and approval workflow.
package guardian

import (
	"context"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
)

// ApprovalConfig holds configuration for approval timeouts.
type ApprovalConfig struct {
	// DefaultTimeout is the default timeout for approval requests.
	DefaultTimeout time.Duration
	// MaxTimeout is the maximum allowed timeout.
	MaxTimeout time.Duration
	// HighRiskCostThreshold is the cost threshold above which approval is required.
	HighRiskCostThreshold float64
}

// DefaultApprovalConfig returns the default approval configuration.
func DefaultApprovalConfig() ApprovalConfig {
	return ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0,
	}
}

// ApprovalRequest represents a pending approval request.
type ApprovalRequest struct {
	TaskID         string
	TaskGoal       string
	EstimatedCost  float64
	RiskLevel      RiskLevel
	RequestedAt    time.Time
	ExpiresAt      time.Time
	RequireApproval bool
	Timeout        time.Duration
	// ResultCh is used to signal the approval result to a waiting caller.
	// It is sent the result when ProcessApproval is called.
	// The channel is closed after sending to indicate the request is fulfilled.
	ResultCh chan<- ApprovalResult
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
	ApprovalResultApproved  ApprovalResult = "approved"
	ApprovalResultRejected ApprovalResult = "rejected"
	ApprovalResultTimeout  ApprovalResult = "timeout"
	ApprovalResultEscalated ApprovalResult = "escalated"
)

// ApprovalDecision represents a decision on an approval request.
type ApprovalDecision struct {
	TaskID      string
	Result      ApprovalResult
	Reason      string
	DecidedBy   string
	DecidedAt   time.Time
}

// WSPusher is the interface for pushing WebSocket events.
type WSPusher interface {
	// PushApprovalRequired pushes an approval required event to the client.
	// It accepts the raw fields needed to create the WebSocket event.
	PushApprovalRequired(ctx context.Context, taskID string, taskGoal string, estimatedCost float64, riskLevel string, requestedAt, expiresAt time.Time, timeout time.Duration) error
}

// DomainEventPublisher is the interface for publishing domain events.
type DomainEventPublisher interface {
	// Publish publishes a domain event via the Outbox pattern.
	Publish(ctx context.Context, event *domain.DomainEvent) error
}