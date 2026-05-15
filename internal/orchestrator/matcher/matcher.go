// Package matcher implements L4-Agent matching: capability scoring,
// historical success rate weighting, and cost efficiency ranking.
package matcher

import (
	"context"
	"sort"

	"github.com/cloud-agent-platform/cap/internal/domain"

	"go.uber.org/zap"
)

// Weights for the scoring formula.
const (
	WeightCapability   = 0.5
	WeightSuccessRate  = 0.3
	WeightCostEfficiency = 0.2
)

// DefaultSuccessRate is used when an agent has no historical data (cold start).
const DefaultSuccessRate = 0.5

// DefaultCostEfficiency is used when budget is not specified.
const DefaultCostEfficiency = 0.5

// AgentProfile represents an agent's profile for matching.
// Contains capability tags, historical performance, and cost information.
type AgentProfile struct {
	// AgentTemplateID is the template ID (e.g., "executor", "tester").
	AgentTemplateID string
	// Role is the agent role for capability mapping.
	Role domain.AgentRole
	// CapabilityTags are the tags describing agent capabilities.
	CapabilityTags []string
	// HistoricalSuccessCount is the number of successful executions.
	HistoricalSuccessCount int
	// HistoricalFailureCount is the number of failed executions.
	HistoricalFailureCount int
	// CostPerExecution is the average cost per execution in yuan.
	CostPerExecution float64
	// ModelName is the LLM model used by this agent.
	ModelName string
}

// SuccessRate returns the historical success rate (0-1).
// Returns DefaultSuccessRate if no historical data exists.
func (p *AgentProfile) SuccessRate() float64 {
	total := p.HistoricalSuccessCount + p.HistoricalFailureCount
	if total == 0 {
		return DefaultSuccessRate
	}
	return float64(p.HistoricalSuccessCount) / float64(total)
}

// TaskRequirements specifies what a task/subtask needs from an agent.
type TaskRequirements struct {
	// RequiredCapabilities are the capabilities needed (e.g., "coding", "testing").
	RequiredCapabilities []string
	// RequiredRole is the preferred agent role.
	RequiredRole domain.AgentRole
	// Budget is the maximum cost acceptable in yuan (0 = no limit).
	Budget float64
	// TaskType is the type of task for role-based matching.
	TaskType domain.SubtaskType
	// Tags are additional tags for matching.
	Tags []string
}

// MatchResult represents the result of matching an agent to a task.
type MatchResult struct {
	// Agent is the matched agent profile.
	Agent *AgentProfile
	// TotalScore is the weighted total score (0-1).
	TotalScore float64
	// CapabilityScore is the capability match score (0-1).
	CapabilityScore float64
	// SuccessRateScore is the historical success rate score (0-1).
	SuccessRateScore float64
	// CostEfficiencyScore is the cost efficiency score (0-1).
	CostEfficiencyScore float64
}

// MatchOption allows configuring the matcher.
type MatchOption func(*Matcher)

// WithLogger sets the logger for the matcher.
func WithLogger(logger *zap.Logger) MatchOption {
	return func(m *Matcher) {
		m.logger = logger
	}
}

// Matcher matches agents to tasks based on capability, success rate, and cost.
type Matcher struct {
	logger *zap.Logger
}

// New creates a new Matcher.
func New(opts ...MatchOption) *Matcher {
	m := &Matcher{}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// FindBestAgent finds the best matching agent from a list of profiles.
// Returns nil if no agents are available.
func (m *Matcher) FindBestAgent(ctx context.Context, reqs *TaskRequirements, profiles []*AgentProfile) *AgentProfile {
	if len(profiles) == 0 {
		if m.logger != nil {
			m.logger.Warn("no agent profiles available for matching",
				zap.String("layer", "L4"),
				zap.String("task_type", string(reqs.TaskType)))
		}
		return nil
	}

	results := m.ScoreAll(ctx, reqs, profiles)
	if len(results) == 0 {
		return nil
	}

	// Sort by total score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalScore > results[j].TotalScore
	})

	best := results[0]
	if m.logger != nil {
		m.logger.Info("best agent matched",
			zap.String("layer", "L4"),
			zap.String("agent_id", best.Agent.AgentTemplateID),
			zap.Float64("total_score", best.TotalScore),
			zap.Float64("capability_score", best.CapabilityScore),
			zap.Float64("success_rate_score", best.SuccessRateScore),
			zap.Float64("cost_efficiency_score", best.CostEfficiencyScore))
	}

	return best.Agent
}

// FindTopKAgents finds the top K best matching agents.
// Returns empty slice if no agents are available or K <= 0.
func (m *Matcher) FindTopKAgents(ctx context.Context, reqs *TaskRequirements, profiles []*AgentProfile, k int) []*AgentProfile {
	if len(profiles) == 0 || k <= 0 {
		return nil
	}

	results := m.ScoreAll(ctx, reqs, profiles)
	if len(results) == 0 {
		return nil
	}

	// Sort by total score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalScore > results[j].TotalScore
	})

	// Limit to K
	if k > len(results) {
		k = len(results)
	}

	topK := make([]*AgentProfile, k)
	for i := 0; i < k; i++ {
		topK[i] = results[i].Agent
	}

	if m.logger != nil {
		m.logger.Info("top K agents matched",
			zap.String("layer", "L4"),
			zap.Int("k", k),
			zap.Int("available", len(profiles)),
			zap.Float64("top_score", results[0].TotalScore))
	}

	return topK
}

// ScoreAll calculates scores for all agent profiles.
func (m *Matcher) ScoreAll(ctx context.Context, reqs *TaskRequirements, profiles []*AgentProfile) []MatchResult {
	results := make([]MatchResult, 0, len(profiles))

	for _, profile := range profiles {
		result := m.Score(ctx, reqs, profile)
		if result != nil {
			results = append(results, *result)
		}
	}

	return results
}

// Score calculates the match score for a single agent profile.
// Returns nil if the agent cannot possibly match (e.g., incompatible role).
func (m *Matcher) Score(ctx context.Context, reqs *TaskRequirements, profile *AgentProfile) *MatchResult {
	// Quick rejection: check role compatibility with task type
	if !m.isRoleCompatible(profile.Role, reqs.TaskType) {
		if m.logger != nil {
			m.logger.Debug("agent role incompatible with task type",
				zap.String("agent_role", string(profile.Role)),
				zap.String("task_type", string(reqs.TaskType)))
		}
		return nil
	}

	capabilityScore := m.calculateCapabilityScore(profile, reqs)
	successRateScore := profile.SuccessRate()
	costEfficiencyScore := m.calculateCostEfficiency(profile, reqs)

	// Weighted total score
	totalScore := capabilityScore*WeightCapability +
		successRateScore*WeightSuccessRate +
		costEfficiencyScore*WeightCostEfficiency

	return &MatchResult{
		Agent:              profile,
		TotalScore:         totalScore,
		CapabilityScore:    capabilityScore,
		SuccessRateScore:   successRateScore,
		CostEfficiencyScore: costEfficiencyScore,
	}
}

// calculateCapabilityScore calculates how well agent capabilities match requirements.
// Returns a score between 0 and 1.
func (m *Matcher) calculateCapabilityScore(profile *AgentProfile, reqs *TaskRequirements) float64 {
	if len(reqs.RequiredCapabilities) == 0 && len(reqs.Tags) == 0 {
		// No specific requirements, return default high score for matching role
		return 0.8
	}

	var matchCount float64
	var totalWeight float64

	// Check required capabilities with weighting based on task type
	for _, required := range reqs.RequiredCapabilities {
		totalWeight++
		if m.hasCapability(profile, required) {
			matchCount++
		}
	}

	// Check tags
	for _, tag := range reqs.Tags {
		totalWeight++
		if m.hasCapability(profile, tag) {
			matchCount++
		}
	}

	if totalWeight == 0 {
		return 0.8
	}

	return matchCount / totalWeight
}

// hasCapability checks if the profile has the given capability.
func (m *Matcher) hasCapability(profile *AgentProfile, capability string) bool {
	for _, tag := range profile.CapabilityTags {
		if tag == capability {
			return true
		}
	}
	return false
}

// calculateCostEfficiency calculates cost efficiency based on budget.
// Returns a score between 0 and 1, where higher is more efficient.
// If budget is 0 (no limit), returns DefaultCostEfficiency.
func (m *Matcher) calculateCostEfficiency(profile *AgentProfile, reqs *TaskRequirements) float64 {
	if reqs.Budget <= 0 {
		return DefaultCostEfficiency
	}

	// Cost efficiency = how much cheaper than budget (clamped to 0-1)
	// If cost is at or below budget: efficiency = 1 - (cost/budget - 1) = 2 - cost/budget
	// If cost exceeds budget: efficiency decreases linearly
	if profile.CostPerExecution <= reqs.Budget {
		// More cost-efficient than budget allows
		ratio := profile.CostPerExecution / reqs.Budget
		return 1.0 - (ratio * 0.5) // Still gives high score but rewards lower cost
	}

	// Exceeds budget - linear penalty
	excess := profile.CostPerExecution / reqs.Budget
	if excess >= 1.0 {
		// Linear decrease: at 2x budget = 0, more than 2x = negative (clamped to 0)
		return max(0, 1.0-excess)
	}

	return 1.0 - excess*0.5
}

// isRoleCompatible checks if agent role is compatible with task type.
func (m *Matcher) isRoleCompatible(role domain.AgentRole, taskType domain.SubtaskType) bool {
	// Define role-task type compatibility matrix
	switch taskType {
	case domain.SubtaskTypeAnalysis:
		return role == domain.AgentRoleObserver || role == domain.AgentRoleStrategist || role == domain.AgentRoleResearcher
	case domain.SubtaskTypeCoding:
		return role == domain.AgentRoleExecutor
	case domain.SubtaskTypeReview:
		return role == domain.AgentRoleGuardian || role == domain.AgentRoleStrategist
	case domain.SubtaskTypeTesting:
		return role == domain.AgentRoleTester
	case domain.SubtaskTypeResearch:
		return role == domain.AgentRoleResearcher || role == domain.AgentRoleObserver
	default:
		return true // Default allow
	}
}

// Verify interface implementation.
var _ *Matcher = (*Matcher)(nil)