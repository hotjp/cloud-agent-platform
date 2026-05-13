// Package roles defines the six core Agent roles for the Cloud Agent Platform.
package roles

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"go.uber.org/zap"
)

// ExecutorConfig holds configuration for the Executor role.
type ExecutorConfig struct {
	// Timeout is the timeout for execution.
	Timeout time.Duration
	// MaxRetries is the maximum number of retries on failure.
	MaxRetries int
	// RollbackOnFailure enables automatic rollback.
	RollbackOnFailure bool
}

// DefaultExecutorConfig returns the default Executor configuration.
func DefaultExecutorConfig() *ExecutorConfig {
	return &ExecutorConfig{
		Timeout:          120 * time.Second,
		MaxRetries:       2,
		RollbackOnFailure: false,
	}
}

// ExecutorResult represents the result of an execution.
type ExecutorResult struct {
	// TaskID is the ULID of the task being executed.
	TaskID string `json:"task_id"`
	// Subtask is the subtask that was executed.
	Subtask string `json:"subtask"`
	// Status is the execution status.
	Status ExecutorStatus `json:"status"`
	// Output describes what was generated.
	Output string `json:"output"`
	// FilesModified lists files that were modified.
	FilesModified []string `json:"files_modified"`
	// Duration is how long the execution took.
	Duration time.Duration `json:"duration"`
	// Error records any error that occurred.
	Error error `json:"error,omitempty"`
}

// ExecutorStatus represents the execution status.
type ExecutorStatus string

const (
	ExecutorStatusSuccess ExecutorStatus = "success"
	ExecutorStatusFailed ExecutorStatus = "failed"
	ExecutorStatusBlocked ExecutorStatus = "blocked"
)

// NewExecutorAgent creates a new Executor agent.
func NewExecutorAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	config := react.DefaultConfig()
	config.SystemPrompt = RoleDefinitions[RoleExecutor].BuildPrompt()
	config.MaxIterations = 15
	config.Timeout = 120 * time.Second
	return react.NewAgent(llm, tools, config, logger)
}

// Execute runs the executor workflow for a specific subtask.
func Execute(ctx context.Context, agent *react.Agent, taskID string, subtask string, specs string) (*ExecutorResult, error) {
	start := time.Now()
	result := &ExecutorResult{
		TaskID:   taskID,
		Subtask:  subtask,
		Status:   ExecutorStatusFailed,
		FilesModified: []string{},
	}

	// Execute the ReAct agent
	actResult, err := agent.Run(ctx, fmt.Sprintf("Execute this subtask:\n\nTask ID: %s\n\nSubtask: %s\n\nSpecifications:\n%s", taskID, subtask, specs))
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result, err
	}

	// Parse execution result
	result.Output = actResult.Answer
	result.FilesModified = extractFilesModified(actResult.Answer)
	result.Duration = time.Since(start)

	if actResult.StopReason == react.StopReasonFinalAnswer {
		result.Status = ExecutorStatusSuccess
	}

	return result, nil
}

// extractFilesModified extracts file names from the execution output.
func extractFilesModified(answer string) []string {
	var files []string
	lines := splitLines(answer)

	for _, line := range lines {
		lower := toLower(line)
		if contains(lower, "file:") || contains(lower, "modified:") {
			files = append(files, cleanLine(line))
		}
	}

	return files
}

// GetTaskID returns the task ID.
func (r *ExecutorResult) GetTaskID() string { return r.TaskID }

// GetDuration returns the execution duration.
func (r *ExecutorResult) GetDuration() time.Duration { return r.Duration }

// GetError returns any error that occurred.
func (r *ExecutorResult) GetError() error { return r.Error }
