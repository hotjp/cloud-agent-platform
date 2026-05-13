// Package react implements the ReAct (Reasoning + Acting) agent pattern.
package react

import (
	"context"
)

// ----------------------------------------------------------------------------
// LLM Interface
// ----------------------------------------------------------------------------

// LLM defines the interface for interacting with a Language Model.
// Implementations can wrap various LLM providers (OpenAI, Anthropic, local models, etc.).
type LLM interface {
	// Generate generates a response from the LLM given a list of messages.
	// It should return the generated text content and the token usage.
	Generate(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error)

	// GenerateStream generates a streaming response from the LLM.
	// It calls the callback for each chunk of the response.
	GenerateStream(ctx context.Context, messages []*Message, opts *GenerateOptions, callback func(chunk string) error) (*GenerateResult, error)
}

// GenerateOptions contains options for LLM generation.
type GenerateOptions struct {
	// MaxTokens is the maximum number of tokens to generate.
	MaxTokens int
	// Temperature controls randomness (0.0-2.0).
	Temperature float64
	// StopSequences is a list of sequences that stop generation when encountered.
	StopSequences []string
	// Model is the model to use (if empty, uses default).
	Model string
}

// DefaultGenerateOptions returns the default generation options.
func DefaultGenerateOptions() *GenerateOptions {
	return &GenerateOptions{
		MaxTokens:     4096,
		Temperature:   0.7,
		StopSequences: []string{},
		Model:         "",
	}
}

// GenerateResult contains the result of an LLM generation.
type GenerateResult struct {
	Content      string `json:"content"`
	StopReason   string `json:"stop_reason"`
	TotalTokens  int    `json:"total_tokens"`
	PromptTokens int    `json:"prompt_tokens"`
	// ToolCalls contains any tool calls requested by the LLM.
	ToolCalls []*ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool call requested by the LLM.
type ToolCall struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Args     map[string]any `json:"args"`
	Valid    bool           `json:"valid"`    // Whether arguments were parsed successfully
	ParseErr string         `json:"parse_err,omitempty"` // Error message if parsing failed
}

// Verify LLM interface at compile time.
var _ LLM = (*LLMAdapter)(nil)

// LLMAdapter is an adapter that wraps a function-based LLM for convenience.
// It allows using simple functions as LLM implementations.
type LLMAdapter struct {
	GenerateFunc func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error)
}

// Generate implements LLM by delegating to the wrapped function.
func (a *LLMAdapter) Generate(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
	if a.GenerateFunc == nil {
		return nil, &AppError{
			Code:    CodeL4LLMInvoke,
			Message: "LLM GenerateFunc is nil",
			Layer:   LayerService,
		}
	}
	return a.GenerateFunc(ctx, messages, opts)
}

// GenerateStream implements LLM by delegating to the wrapped function.
func (a *LLMAdapter) GenerateStream(ctx context.Context, messages []*Message, opts *GenerateOptions, callback func(chunk string) error) (*GenerateResult, error) {
	// Default implementation: call Generate and invoke callback once with full content
	result, err := a.Generate(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	if callback != nil {
		if err := callback(result.Content); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// AppError represents a service-layer error.
type AppError struct {
	Code    string
	Message string
	Layer   string
}

// Error implements the error interface.
func (e *AppError) Error() string {
	return e.Code + ": " + e.Message
}

// Layer identifiers for error codes.
const (
	LayerService = "L4"
)

// Error codes for L4-Service.
const (
	CodeL4LLMInvoke = "L1609"
	CodeL4ToolDenied = "L1613"
	CodeL4MaxIterations = "L1617"
	CodeL4ContextExceeded = "L1606"
)