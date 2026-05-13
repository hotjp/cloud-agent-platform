// Package roles defines the six core Agent roles for the Cloud Agent Platform.
package roles

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"go.uber.org/zap"
)

// StrategistConfig holds configuration for the Strategist role.
type StrategistConfig struct {
	// MaxSubtasks is the maximum number of subtasks to create.
	MaxSubtasks int
	// Timeout is the timeout for strategy development.
	Timeout time.Duration
	// EnableParallelPlanning enables parallel subtask identification.
	EnableParallelPlanning bool
}

// DefaultStrategistConfig returns the default Strategist configuration.
func DefaultStrategistConfig() *StrategistConfig {
	return &StrategistConfig{
		MaxSubtasks:            10,
		Timeout:                60 * time.Second,
		EnableParallelPlanning: true,
	}
}

// StrategistResult represents the result of strategy development.
type StrategistResult struct {
	// TaskID is the ULID of the task being planned.
	TaskID string `json:"task_id"`
	// Strategy contains the execution strategy.
	Strategy *Strategy `json:"strategy"`
	// Duration is how long the strategy development took.
	Duration time.Duration `json:"duration"`
	// Error records any error that occurred.
	Error error `json:"error,omitempty"`
}

// Strategy contains the execution strategy.
type Strategy struct {
	// Phases contains the ordered execution phases.
	Phases []*Phase `json:"phases"`
	// Risks contains identified risks and mitigations.
	Risks []*Risk `json:"risks"`
	// ExecutionOrder explains the sequence rationale.
	ExecutionOrder string `json:"execution_order"`
}

// Phase represents a single execution phase.
type Phase struct {
	// Name is the phase name.
	Name string `json:"name"`
	// Subtask is the subtask description.
	Subtask string `json:"subtask"`
	// AgentRole is the role that should execute this phase.
	AgentRole RoleType `json:"agent_role"`
	// Dependencies lists phase names this depends on.
	Dependencies []string `json:"dependencies"`
	// Description provides more context.
	Description string `json:"description,omitempty"`
	// Priority indicates execution priority (lower = earlier).
	Priority int `json:"priority"`
}

// Risk represents an identified risk.
type Risk struct {
	// Description describes the risk.
	Description string `json:"description"`
	// Mitigation is how to mitigate the risk.
	Mitigation string `json:"mitigation"`
	// Severity indicates impact level.
	Severity string `json:"severity"` // "high", "medium", "low"
}

// NewStrategistAgent creates a new Strategist agent.
func NewStrategistAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	config := react.DefaultConfig()
	config.SystemPrompt = RoleDefinitions[RoleStrategist].BuildPrompt()
	config.MaxIterations = 10
	config.Timeout = 60 * time.Second
	return react.NewAgent(llm, tools, config, logger)
}

// Plan executes the strategy development workflow.
func Plan(ctx context.Context, agent *react.Agent, taskID string, observations string) (*StrategistResult, error) {
	start := time.Now()
	result := &StrategistResult{
		TaskID: taskID,
	}

	// Execute the ReAct agent
	actResult, err := agent.Run(ctx, fmt.Sprintf("Develop an execution strategy for this task:\n\nTask ID: %s\n\nObservations:\n%s", taskID, observations))
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result, err
	}

	// Parse strategy from result
	result.Strategy = parseStrategy(actResult.Answer)
	result.Duration = time.Since(start)

	return result, nil
}

// parseStrategy parses the structured strategy from the agent's answer.
func parseStrategy(answer string) *Strategy {
	strategy := &Strategy{
		Phases: make([]*Phase, 0),
		Risks:  make([]*Risk, 0),
	}

	lines := splitLines(answer)
	inPhases := false
	inRisks := false
	currentPhase := (*Phase)(nil)

	for i, line := range lines {
		lower := toLower(line)

		switch {
		case contains(lower, "phase"):
			inPhases = true
			inRisks = false
		case contains(lower, "risk"):
			inRisks = true
			inPhases = false
		case contains(lower, "execution order"):
			if i+1 < len(lines) {
				strategy.ExecutionOrder = cleanLine(lines[i+1])
			}
		case inPhases && contains(lower, "subtask"):
			if currentPhase != nil {
				strategy.Phases = append(strategy.Phases, currentPhase)
			}
			currentPhase = &Phase{Priority: len(strategy.Phases) + 1}
			if i+1 < len(lines) {
				currentPhase.Subtask = cleanLine(lines[i+1])
			}
		case currentPhase != nil:
			if contains(lower, "agent:") {
				if i+1 < len(lines) {
					agentLine := cleanLine(lines[i+1])
					currentPhase.AgentRole = parseAgentRole(agentLine)
				}
			} else if contains(lower, "dependenc") {
				if i+1 < len(lines) {
					currentPhase.Dependencies = append(currentPhase.Dependencies, cleanLine(lines[i+1]))
				}
			}
		case inRisks && contains(lower, "risk"):
			risk := &Risk{}
			if i+1 < len(lines) {
				risk.Description = cleanLine(lines[i+1])
			}
			if i+2 < len(lines) {
				risk.Mitigation = cleanLine(lines[i+2])
			}
			strategy.Risks = append(strategy.Risks, risk)
		}
	}

	if currentPhase != nil {
		strategy.Phases = append(strategy.Phases, currentPhase)
	}

	return strategy
}

// parseAgentRole parses an agent role from a string.
func parseAgentRole(s string) RoleType {
	s = toLower(s)
	switch {
	case contains(s, "observer"):
		return RoleObserver
	case contains(s, "strategist"):
		return RoleStrategist
	case contains(s, "executor"):
		return RoleExecutor
	case contains(s, "reviewer"):
		return RoleReviewer
	case contains(s, "learner"):
		return RoleLearner
	case contains(s, "coordinator"):
		return RoleCoordinator
	default:
		return RoleExecutor
	}
}

// GetTaskID returns the task ID.
func (r *StrategistResult) GetTaskID() string { return r.TaskID }

// GetDuration returns the planning duration.
func (r *StrategistResult) GetDuration() time.Duration { return r.Duration }

// GetError returns any error that occurred.
func (r *StrategistResult) GetError() error { return r.Error }
