// Package roles defines the six core Agent roles for the Cloud Agent Platform.
package roles

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"go.uber.org/zap"
)

// ObserverConfig holds configuration for the Observer role.
type ObserverConfig struct {
	// MaxContextSize is the maximum context size to collect.
	MaxContextSize int
	// Timeout is the timeout for observation.
	Timeout time.Duration
	// IncludeHistory determines whether to include execution history.
	IncludeHistory bool
}

// DefaultObserverConfig returns the default Observer configuration.
func DefaultObserverConfig() *ObserverConfig {
	return &ObserverConfig{
		MaxContextSize: 10000,
		Timeout:        30 * time.Second,
		IncludeHistory: false,
	}
}

// ObserverResult represents the result of an observation.
type ObserverResult struct {
	// TaskID is the ULID of the task being observed.
	TaskID string `json:"task_id"`
	// Observations contains the structured observations.
	Observations *Observations `json:"observations"`
	// RawContext contains the raw context gathered.
	RawContext map[string]any `json:"raw_context,omitempty"`
	// Duration is how long the observation took.
	Duration time.Duration `json:"duration"`
	// Error records any error that occurred.
	Error error `json:"error,omitempty"`
}

// Observations contains structured observations.
type Observations struct {
	// KeyContext contains the relevant context summary.
	KeyContext string `json:"key_context"`
	// Constraints contains the identified constraints.
	Constraints []string `json:"constraints"`
	// Requirements contains the functional requirements.
	Requirements []string `json:"requirements"`
	// SuccessCriteria defines how success is measured.
	SuccessCriteria []string `json:"success_criteria"`
}

// NewObserverAgent creates a new Observer agent.
func NewObserverAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	config := react.DefaultConfig()
	config.SystemPrompt = RoleDefinitions[RoleObserver].BuildPrompt()
	config.MaxIterations = 5
	config.Timeout = 30 * time.Second
	return react.NewAgent(llm, tools, config, logger)
}

// Observe executes the observation workflow.
func Observe(ctx context.Context, agent *react.Agent, taskID string, taskContext string) (*ObserverResult, error) {
	start := time.Now()
	result := &ObserverResult{
		TaskID: taskID,
	}

	// Execute the ReAct agent
	actResult, err := agent.Run(ctx, fmt.Sprintf("Observe this task and provide structured observations:\n\nTask ID: %s\n\nTask Context:\n%s", taskID, taskContext))
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result, err
	}

	// Parse observations from result
	result.Observations = parseObservations(actResult.Answer)
	result.Duration = time.Since(start)

	return result, nil
}

// parseObservations parses the structured observations from the agent's answer.
func parseObservations(answer string) *Observations {
	obs := &Observations{
		Constraints:     []string{},
		Requirements:    []string{},
		SuccessCriteria: []string{},
	}

	// Simple parsing - in production, this would be more sophisticated
	// Look for section markers in the output
	lines := splitLines(answer)

	for i, line := range lines {
		lower := toLower(line)
		switch {
		case contains(lower, "key context") || contains(lower, "context:"):
			// Collect context lines
			if i+1 < len(lines) {
				obs.KeyContext = cleanLine(lines[i+1])
			}
		case contains(lower, "constraint"):
			// Collect constraint lines
			if i+1 < len(lines) {
				obs.Constraints = append(obs.Constraints, cleanLine(lines[i+1]))
			}
		case contains(lower, "requirement"):
			// Collect requirement lines
			if i+1 < len(lines) {
				obs.Requirements = append(obs.Requirements, cleanLine(lines[i+1]))
			}
		case contains(lower, "success") || contains(lower, "criteria"):
			// Collect success criteria lines
			if i+1 < len(lines) {
				obs.SuccessCriteria = append(obs.SuccessCriteria, cleanLine(lines[i+1]))
			}
		}
	}

	return obs
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func cleanLine(s string) string {
	// Remove common prefixes
	s = containsPrefix(s, "- ")
	s = containsPrefix(s, "* ")
	s = containsPrefix(s, "> ")
	return trimSpace(s)
}

func containsPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// Verify ObserverResult implements the expected interface at compile time.
var _ = RoleResult((*ObserverResult)(nil))

// RoleResult is an interface for role results.
type RoleResult interface {
	GetTaskID() string
	GetDuration() time.Duration
	GetError() error
}

// GetTaskID returns the task ID.
func (r *ObserverResult) GetTaskID() string { return r.TaskID }

// GetDuration returns the observation duration.
func (r *ObserverResult) GetDuration() time.Duration { return r.Duration }

// GetError returns any error that occurred.
func (r *ObserverResult) GetError() error { return r.Error }
