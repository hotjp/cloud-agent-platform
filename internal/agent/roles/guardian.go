// Package roles defines the six core Agent roles for the Cloud Agent Platform.
package roles

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"go.uber.org/zap"
)

// GuardianConfig holds configuration for the Guardian role.
type GuardianConfig struct {
	// Timeout is the timeout for security review.
	Timeout time.Duration
	// RequireHumanApproval requires human approval for high-risk operations.
	RequireHumanApproval bool
	// HighRiskThreshold is the risk score threshold for requiring human approval.
	HighRiskThreshold RiskLevel
}

// DefaultGuardianConfig returns the default Guardian configuration.
func DefaultGuardianConfig() *GuardianConfig {
	return &GuardianConfig{
		Timeout:            60 * time.Second,
		RequireHumanApproval: true,
		HighRiskThreshold:  RiskLevelHigh,
	}
}

// GuardianResult represents the result of a security review.
type GuardianResult struct {
	// TaskID is the ULID of the task being reviewed.
	TaskID string `json:"task_id"`
	// Operation is the operation being reviewed.
	Operation string `json:"operation"`
	// RiskLevel is the assessed risk level.
	RiskLevel RiskLevel `json:"risk_level"`
	// Concerns contains identified concerns.
	Concerns []string `json:"concerns"`
	// CompliancePassed indicates if compliance checks passed.
	CompliancePassed bool `json:"compliance_passed"`
	// ComplianceIssues contains compliance issues found.
	ComplianceIssues []string `json:"compliance_issues"`
	// Decision is the approval decision.
	Decision ApprovalDecision `json:"decision"`
	// Rationale explains the decision.
	Rationale string `json:"rationale"`
	// Conditions contains conditions for approval.
	Conditions []string `json:"conditions"`
	// Duration is how long the review took.
	Duration time.Duration `json:"duration"`
	// Error records any error that occurred.
	Error error `json:"error,omitempty"`
}

// RiskLevel represents the risk level.
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// ApprovalDecision represents the approval decision.
type ApprovalDecision string

const (
	ApprovalApproved           ApprovalDecision = "approved"
	ApprovalDenied             ApprovalDecision = "denied"
	ApprovalRequiresHumanApproval ApprovalDecision = "requires_human_approval"
)

// NewGuardianAgent creates a new Guardian agent.
func NewGuardianAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	config := react.DefaultConfig()
	config.SystemPrompt = RoleDefinitions[RoleGuardian].BuildPrompt()
	config.MaxIterations = 10
	config.Timeout = 60 * time.Second
	return react.NewAgent(llm, tools, config, logger)
}

// Guard executes the security review workflow.
func Guard(ctx context.Context, agent *react.Agent, taskID string, operation string, context_ string) (*GuardianResult, error) {
	start := time.Now()
	result := &GuardianResult{
		TaskID:    taskID,
		Operation: operation,
		Concerns:  make([]string, 0),
		ComplianceIssues: make([]string, 0),
		Conditions: make([]string, 0),
	}

	// Execute the ReAct agent
	actResult, err := agent.Run(ctx, fmt.Sprintf("Review this operation for security and safety:\n\nTask ID: %s\n\nOperation:\n%s\n\nContext:\n%s", taskID, operation, context_))
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result, err
	}

	// Parse security review result
	parsed := parseSecurityReview(actResult.Answer)
	result.RiskLevel = parsed.RiskLevel
	result.Concerns = parsed.Concerns
	result.CompliancePassed = parsed.CompliancePassed
	result.ComplianceIssues = parsed.ComplianceIssues
	result.Decision = parsed.Decision
	result.Rationale = parsed.Rationale
	result.Conditions = parsed.Conditions
	result.Duration = time.Since(start)

	return result, nil
}

// parsedSecurityReview holds parsed security review data.
type parsedSecurityReviewData struct {
	RiskLevel        RiskLevel
	Concerns         []string
	CompliancePassed bool
	ComplianceIssues []string
	Decision         ApprovalDecision
	Rationale        string
	Conditions       []string
}

// parseSecurityReview parses the security review result from the agent's answer.
func parseSecurityReview(answer string) *parsedSecurityReviewData {
	data := &parsedSecurityReviewData{
		Concerns:         make([]string, 0),
		ComplianceIssues: make([]string, 0),
		Conditions:       make([]string, 0),
		RiskLevel:        RiskLevelLow,
		Decision:         ApprovalApproved,
	}

	lines := splitLines(answer)
	inConcerns := false
	inComplianceIssues := false
	inConditions := false

	for i, line := range lines {
		lower := toLower(line)

		switch {
		case contains(lower, "risk level"):
			if i+1 < len(lines) {
				data.RiskLevel = parseRiskLevel(cleanLine(lines[i+1]))
			}
		case contains(lower, "concern"):
			inConcerns = true
			inComplianceIssues = false
			inConditions = false
		case contains(lower, "compliance"):
			if contains(lower, "passed") || contains(lower, "check") {
				if i+1 < len(lines) {
					passed := contains(toLower(lines[i+1]), "yes")
					data.CompliancePassed = passed
				}
			} else if contains(lower, "issue") {
				inComplianceIssues = true
				inConcerns = false
				inConditions = false
			}
		case contains(lower, "decision"):
			if i+1 < len(lines) {
				data.Decision = parseApprovalDecision(cleanLine(lines[i+1]))
			}
		case contains(lower, "rationale"):
			if i+1 < len(lines) {
				data.Rationale = cleanLine(lines[i+1])
			}
		case contains(lower, "condition"):
			inConditions = true
			inConcerns = false
			inComplianceIssues = false
			if i+1 < len(lines) {
				data.Conditions = append(data.Conditions, cleanLine(lines[i+1]))
			}
		case inConcerns && !contains(lower, "concern"):
			if contains(lower, "-") || contains(lower, "*") {
				data.Concerns = append(data.Concerns, cleanLine(line))
			}
		case inComplianceIssues && !contains(lower, "issue"):
			if contains(lower, "-") || contains(lower, "*") {
				data.ComplianceIssues = append(data.ComplianceIssues, cleanLine(line))
			}
		case inConditions && !contains(lower, "condition"):
			if contains(lower, "-") || contains(lower, "*") || contains(lower, "must") {
				data.Conditions = append(data.Conditions, cleanLine(line))
			}
		}
	}

	return data
}

// parseRiskLevel parses a risk level from a string.
func parseRiskLevel(s string) RiskLevel {
	s = toLower(s)
	switch {
	case contains(s, "critical"):
		return RiskLevelCritical
	case contains(s, "high"):
		return RiskLevelHigh
	case contains(s, "medium"):
		return RiskLevelMedium
	default:
		return RiskLevelLow
	}
}

// parseApprovalDecision parses an approval decision from a string.
func parseApprovalDecision(s string) ApprovalDecision {
	s = toLower(s)
	switch {
	case contains(s, "denied"):
		return ApprovalDenied
	case contains(s, "requires human") || contains(s, "human approval"):
		return ApprovalRequiresHumanApproval
	case contains(s, "approved"):
		return ApprovalApproved
	default:
		return ApprovalRequiresHumanApproval
	}
}

// GetTaskID returns the task ID.
func (r *GuardianResult) GetTaskID() string { return r.TaskID }

// GetDuration returns the review duration.
func (r *GuardianResult) GetDuration() time.Duration { return r.Duration }

// GetError returns any error that occurred.
func (r *GuardianResult) GetError() error { return r.Error }
