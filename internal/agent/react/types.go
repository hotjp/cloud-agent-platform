// Package react implements the ReAct (Reasoning + Acting) agent pattern.
// It provides an LLM-driven agent loop: Thought → Action → Observation → FinalAnswer.
// The agent uses tools to interact with external systems and produces structured outputs.
package react

import (
	"context"
	"time"

	"github.com/oklog/ulid/v2"
)

// ----------------------------------------------------------------------------
// Message types for ReAct conversation
// ----------------------------------------------------------------------------

// Role represents the role of a message sender in the ReAct conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single message in the ReAct conversation.
type Message struct {
	Role    Role       `json:"role"`
	Content string     `json:"content"`
	Name    string     `json:"name,omitempty"`    // tool name for RoleTool messages
	ToolUse *ToolUse   `json:"tool_use,omitempty"` // tool call for RoleAssistant messages
}

// ToolUse represents a tool call made by the assistant.
type ToolUse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args any    `json:"args"` // map[string]any
}

// ----------------------------------------------------------------------------
// Step types for ReAct execution
// ----------------------------------------------------------------------------

// StepType represents the type of a ReAct step.
type StepType string

const (
	StepTypeThought    StepType = "thought"
	StepTypeAction     StepType = "action"
	StepTypeObservation StepType = "observation"
	StepTypeFinalAnswer StepType = "final_answer"
	StepTypeError      StepType = "error"
)

// Step represents a single step in the ReAct execution loop.
type Step struct {
	Type        StepType `json:"type"`
	Content     string   `json:"content"`
	ToolName    string   `json:"tool_name,omitempty"`
	ToolArgs    any      `json:"tool_args,omitempty"`
	ToolResult  any      `json:"tool_result,omitempty"`
	Error       string   `json:"error,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	Accumulated string   `json:"accumulated,omitempty"` // Running conversation for next LLM call
}

// NewStep creates a new Step with the current timestamp.
func NewStep(stepType StepType, content string) *Step {
	return &Step{
		Type:      stepType,
		Content:   content,
		Timestamp: time.Now().UTC(),
	}
}

// WithToolInfo adds tool information to the step.
func (s *Step) WithToolInfo(toolName string, toolArgs any) *Step {
	s.ToolName = toolName
	s.ToolArgs = toolArgs
	return s
}

// WithToolResult adds the tool execution result to the step.
func (s *Step) WithToolResult(result any) *Step {
	s.ToolResult = result
	return s
}

// WithError sets the error message for the step.
func (s *Step) WithError(err string) *Step {
	s.Error = err
	return s
}

// WithAccumulated sets the accumulated conversation context.
func (s *Step) WithAccumulated(acc string) *Step {
	s.Accumulated = acc
	return s
}

// ----------------------------------------------------------------------------
// Execution result
// ----------------------------------------------------------------------------

// Result represents the final result of a ReAct agent execution.
type Result struct {
	Answer           string        `json:"answer"`
	Steps            []*Step      `json:"steps"`
	TotalSteps       int          `json:"total_steps"`
	TotalTokensUsed  int          `json:"total_tokens_used"`
	ExecutionTime    time.Duration `json:"execution_time"`
	Iterations       int          `json:"iterations"`
	StopReason       StopReason   `json:"stop_reason"`
	Error            error        `json:"error,omitempty"`
}

// StopReason explains why the ReAct loop stopped.
type StopReason string

const (
	StopReasonFinalAnswer    StopReason = "final_answer"    // LLM returned final answer
	StopReasonMaxIterations  StopReason = "max_iterations"   // Hit max iterations limit
	StopReasonContextEmpty  StopReason = "context_empty"    // No input provided
	StopReasonError         StopReason = "error"            // Error occurred
	StopReasonCancelled     StopReason = "cancelled"        // Context cancelled
	StopReasonTimeout       StopReason = "timeout"          // Execution timeout reached
)

// NewResult creates a new execution result.
func NewResult(answer string, steps []*Step, iterations int, stopReason StopReason) *Result {
	return &Result{
		Answer:     answer,
		Steps:      steps,
		TotalSteps: len(steps),
		Iterations: iterations,
		StopReason: stopReason,
	}
}

// AddTokens adds token usage to the result.
func (r *Result) AddTokens(tokens int) {
	r.TotalTokensUsed += tokens
}

// SetExecutionTime sets the total execution time.
func (r *Result) SetExecutionTime(d time.Duration) {
	r.ExecutionTime = d
}

// SetError sets the error for the result.
func (r *Result) SetError(err error) {
	r.Error = err
}

// ----------------------------------------------------------------------------
// Configuration
// ----------------------------------------------------------------------------

// Config holds the configuration for a ReAct agent.
type Config struct {
	// MaxIterations is the maximum number of thought/action loops.
	// Default: 10
	MaxIterations int

	// MaxTokens is the maximum tokens allowed in a single LLM call.
	// Default: 4096
	MaxTokens int

	// Temperature controls randomness in LLM responses.
	// Default: 0.7
	Temperature float64

	// Timeout is the timeout for a single LLM call or tool execution.
	// Default: 60 seconds
	Timeout time.Duration

	// MaxExecutionTime is the maximum total execution time for the entire ReAct loop.
	// When reached, the agent stops regardless of iteration count.
	// Default: 0 (no limit)
	MaxExecutionTime time.Duration

	// SystemPrompt is the system prompt for the ReAct agent.
	SystemPrompt string

	// EnableStreaming enables streaming of intermediate steps.
	EnableStreaming bool

	// StreamingCallback is called for each step when streaming is enabled.
	StreamingCallback func(ctx context.Context, step *Step) error

	// OnIterationEnd is called after each iteration completes (optional).
	OnIterationEnd func(ctx context.Context, iteration int, step *Step)

	// OnError is called when an error occurs.
	OnError func(ctx context.Context, err error, step *Step)
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		MaxIterations:    10,
		MaxTokens:        4096,
		Temperature:      0.7,
		Timeout:          60 * time.Second,
		MaxExecutionTime: 0, // No limit by default
		EnableStreaming:  false,
		StreamingCallback: nil,
		OnIterationEnd:   nil,
		OnError:          nil,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.MaxIterations <= 0 {
		c.MaxIterations = 10
	}
	if c.MaxIterations > 100 {
		c.MaxIterations = 100 // Cap at reasonable maximum
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 4096
	}
	if c.Temperature < 0 || c.Temperature > 2 {
		c.Temperature = 0.7
	}
	if c.Timeout <= 0 {
		c.Timeout = 60 * time.Second
	}
	// MaxExecutionTime of 0 means no limit (valid)
	if c.MaxExecutionTime < 0 {
		c.MaxExecutionTime = 0
	}
	return nil
}

// ----------------------------------------------------------------------------
// ULID generation helpers
// ----------------------------------------------------------------------------

// newULID generates a new ULID string.
func newULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), nil).String()
}