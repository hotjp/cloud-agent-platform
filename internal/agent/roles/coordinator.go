// Package roles defines the six core Agent roles for the Cloud Agent Platform.
package roles

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"go.uber.org/zap"
)

// CoordinatorConfig holds configuration for the Coordinator role.
type CoordinatorConfig struct {
	// Timeout is the timeout for coordination.
	Timeout time.Duration
	// MaxParallelAgents is the maximum number of parallel agents.
	MaxParallelAgents int
	// EnableAutoRetry enables automatic retry on failure.
	EnableAutoRetry bool
	// MaxRetries is the maximum number of retries.
	MaxRetries int
}

// DefaultCoordinatorConfig returns the default Coordinator configuration.
func DefaultCoordinatorConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		Timeout:           180 * time.Second,
		MaxParallelAgents: 3,
		EnableAutoRetry:   true,
		MaxRetries:        2,
	}
}

// CoordinatorResult represents the result of coordination.
type CoordinatorResult struct {
	// TaskID is the ULID of the task being coordinated.
	TaskID string `json:"task_id"`
	// Status is the workflow status.
	Status WorkflowStatus `json:"status"`
	// Assignments contains active agent assignments.
	Assignments []*Assignment `json:"assignments"`
	// Completed contains completed items.
	Completed []string `json:"completed"`
	// Pending contains pending items.
	Pending []string `json:"pending"`
	// NextActions contains the next actions.
	NextActions []string `json:"next_actions"`
	// Decisions contains executive decisions made.
	Decisions []*Decision `json:"decisions"`
	// Duration is how long the coordination took.
	Duration time.Duration `json:"duration"`
	// Error records any error that occurred.
	Error error `json:"error,omitempty"`
}

// WorkflowStatus represents the workflow status.
type WorkflowStatus string

const (
	WorkflowStatusRunning     WorkflowStatus = "running"
	WorkflowStatusCompleted  WorkflowStatus = "completed"
	WorkflowStatusFailed     WorkflowStatus = "failed"
	WorkflowStatusBlocked    WorkflowStatus = "blocked"
	WorkflowStatusPaused     WorkflowStatus = "paused"
)

// Assignment represents an agent assignment.
type Assignment struct {
	// AgentID is the agent identifier.
	AgentID string `json:"agent_id"`
	// Role is the role assigned.
	Role RoleType `json:"role"`
	// Task is the task assigned.
	Task string `json:"task"`
	// Status is the assignment status.
	Status AssignmentStatus `json:"status"`
}

// AssignmentStatus represents the assignment status.
type AssignmentStatus string

const (
	AssignmentStatusPending   AssignmentStatus = "pending"
	AssignmentStatusRunning  AssignmentStatus = "running"
	AssignmentStatusComplete AssignmentStatus = "complete"
	AssignmentStatusFailed   AssignmentStatus = "failed"
)

// Decision represents an executive decision.
type Decision struct {
	// Description describes what was decided.
	Description string `json:"description"`
	// Rationale explains why this was decided.
	Rationale string `json:"rationale"`
}

// NewCoordinatorAgent creates a new Coordinator agent.
func NewCoordinatorAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	config := react.DefaultConfig()
	config.SystemPrompt = RoleDefinitions[RoleCoordinator].BuildPrompt()
	config.MaxIterations = 15
	config.Timeout = 180 * time.Second
	return react.NewAgent(llm, tools, config, logger)
}

// Coordinate executes the coordination workflow.
func Coordinate(ctx context.Context, agent *react.Agent, taskID string, currentState string, availableAgents string) (*CoordinatorResult, error) {
	start := time.Now()
	result := &CoordinatorResult{
		TaskID: taskID,
		Status: WorkflowStatusRunning,
		Assignments: make([]*Assignment, 0),
		Completed: make([]string, 0),
		Pending: make([]string, 0),
		NextActions: make([]string, 0),
		Decisions: make([]*Decision, 0),
	}

	// Execute the ReAct agent
	actResult, err := agent.Run(ctx, fmt.Sprintf("Coordinate this workflow:\n\nTask ID: %s\n\nCurrent State:\n%s\n\nAvailable Agents:\n%s", taskID, currentState, availableAgents))
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result, err
	}

	// Parse coordination result
	parsed := parseCoordination(actResult.Answer)
	result.Status = parsed.Status
	result.Assignments = parsed.Assignments
	result.Completed = parsed.Completed
	result.Pending = parsed.Pending
	result.NextActions = parsed.NextActions
	result.Decisions = parsed.Decisions
	result.Duration = time.Since(start)

	return result, nil
}

// parsedCoordination holds parsed coordination data.
type parsedCoordinationData struct {
	Status      WorkflowStatus
	Assignments []*Assignment
	Completed   []string
	Pending     []string
	NextActions []string
	Decisions   []*Decision
}

// parseCoordination parses the coordination result from the agent's answer.
func parseCoordination(answer string) *parsedCoordinationData {
	data := &parsedCoordinationData{
		Status:      WorkflowStatusRunning,
		Assignments: make([]*Assignment, 0),
		Completed:   make([]string, 0),
		Pending:     make([]string, 0),
		NextActions: make([]string, 0),
		Decisions:  make([]*Decision, 0),
	}

	lines := splitLines(answer)
	inAssignments := false
	inNextActions := false
	inDecisions := false
	currentAssignment := (*Assignment)(nil)
	currentDecision := (*Decision)(nil)

	for i, line := range lines {
		lower := toLower(line)

		switch {
		case contains(lower, "status"):
			// Determine workflow status
			if contains(lower, "complete") {
				data.Status = WorkflowStatusCompleted
			} else if contains(lower, "failed") {
				data.Status = WorkflowStatusFailed
			} else if contains(lower, "blocked") {
				data.Status = WorkflowStatusBlocked
			} else if contains(lower, "paused") {
				data.Status = WorkflowStatusPaused
			}
		case contains(lower, "current phase"):
			// Track current phase - could update status
		case contains(lower, "active agent"):
			inAssignments = true
			inNextActions = false
			inDecisions = false
		case contains(lower, "completed:"):
			inAssignments = false
			inNextActions = false
			inDecisions = false
		case contains(lower, "pending:"):
			inAssignments = false
			inNextActions = false
			inDecisions = false
		case contains(lower, "next action"):
			inAssignments = false
			inNextActions = true
			inDecisions = false
		case contains(lower, "decision"):
			inAssignments = false
			inNextActions = false
			inDecisions = true
			if currentDecision != nil {
				data.Decisions = append(data.Decisions, currentDecision)
			}
			currentDecision = &Decision{}
		case inAssignments:
			if contains(lower, "agent:") {
				if currentAssignment != nil {
					data.Assignments = append(data.Assignments, currentAssignment)
				}
				currentAssignment = &Assignment{Status: AssignmentStatusPending}
				if i+1 < len(lines) {
					currentAssignment.AgentID = cleanLine(lines[i+1])
				}
			} else if currentAssignment != nil {
				if contains(lower, "role:") {
					if i+1 < len(lines) {
						currentAssignment.Role = parseAgentRole(cleanLine(lines[i+1]))
					}
				} else if contains(lower, "task:") {
					if i+1 < len(lines) {
						currentAssignment.Task = cleanLine(lines[i+1])
					}
				} else if contains(lower, "status:") {
					if i+1 < len(lines) {
						currentAssignment.Status = parseAssignmentStatus(cleanLine(lines[i+1]))
					}
				}
			}
		case inNextActions:
			if !contains(lower, "next action") && !contains(lower, ":") {
				data.NextActions = append(data.NextActions, cleanLine(line))
			}
		case inDecisions:
			if currentDecision != nil {
				if currentDecision.Description == "" && !contains(lower, "decision") {
					currentDecision.Description = cleanLine(line)
				} else if contains(lower, "rationale") && i+1 < len(lines) {
					currentDecision.Rationale = cleanLine(lines[i+1])
				}
			}
		}
	}

	if currentAssignment != nil {
		data.Assignments = append(data.Assignments, currentAssignment)
	}
	if currentDecision != nil {
		data.Decisions = append(data.Decisions, currentDecision)
	}

	return data
}

// parseAssignmentStatus parses an assignment status from a string.
func parseAssignmentStatus(s string) AssignmentStatus {
	s = toLower(s)
	switch {
	case contains(s, "pending"):
		return AssignmentStatusPending
	case contains(s, "running"):
		return AssignmentStatusRunning
	case contains(s, "complete"):
		return AssignmentStatusComplete
	case contains(s, "failed"):
		return AssignmentStatusFailed
	default:
		return AssignmentStatusPending
	}
}

// GetTaskID returns the task ID.
func (r *CoordinatorResult) GetTaskID() string { return r.TaskID }

// GetDuration returns the coordination duration.
func (r *CoordinatorResult) GetDuration() time.Duration { return r.Duration }

// GetError returns any error that occurred.
func (r *CoordinatorResult) GetError() error { return r.Error }
