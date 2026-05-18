// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
// Selects optimal model based on task complexity, cost, and latency.
package llmrouter

import (
	"context"
	"time"
)

// TaskType represents the type of task for LLM routing.
type TaskType string

const (
	TaskTypeAnalysis  TaskType = "analysis"  // Deep reasoning tasks
	TaskTypeCoding    TaskType = "coding"    // Code generation
	TaskTypeReview    TaskType = "review"    // Security/policy review
	TaskTypeTesting   TaskType = "testing"   // Test generation
	TaskTypeResearch  TaskType = "research"  // Technical research
	TaskTypeSummarize TaskType = "summarize" // Summary generation
	TaskTypeSimple    TaskType = "simple"   // Simple/template tasks
)

// ModelName represents the available LLM models.
type ModelName string

const (
	ModelClaudeSonnet ModelName = "claude-sonnet-4"
	ModelClaudeHaiku  ModelName = "claude-haiku-3"
	ModelGLM5        ModelName = "glm-5.1"
	ModelGLM5Air     ModelName = "glm-5.1-air"
	ModelDeepseekChat ModelName = "deepseek-chat"
	ModelDeepseekCoder ModelName = "deepseek-coder"
	ModelQwenTurbo    ModelName = "qwen-turbo"
	ModelQwenMax      ModelName = "qwen-max"
	ModelQwenPlus     ModelName = "qwen-plus"
	ModelQwen2Coder   ModelName = "qwen2.5-coder"
	ModelYiTurbo      ModelName = "yi-turbo"
	ModelYiLightning  ModelName = "yi-lightning"
)

// LLMRequest represents a request to the LLM.
type LLMRequest struct {
	TaskType   TaskType
	Model      ModelName // Optional: specific model, overrides routing
	Prompt     string
	System     string
	MaxTokens  int
	Temperature float64
	StopWords  []string
}

// LLMResponse represents a response from the LLM.
type LLMResponse struct {
	Content    string
	Model      ModelName
	TokensUsed int
	LatencyMs  int64
	Reasoning  string // For models that support reasoning
}

// StreamChunk represents a chunk in a streaming response.
type StreamChunk struct {
	Content    string
	Done       bool
	TokensUsed int
}

// EmbedRequest represents an embedding request.
type EmbedRequest struct {
	Text string
}

// EmbedResponse represents an embedding response.
type EmbedResponse struct {
	Embedding []float32
	Model     ModelName
	TokensUsed int
}

// ProviderStats holds statistics for a provider.
type ProviderStats struct {
	TotalRequests   int64
	SuccessRequests int64
	FailedRequests  int64
	TotalTokens     int64
	AvgLatencyMs    int64
	LastError       string
	LastErrorTime   time.Time
}

// LLMProvider is the interface for LLM providers.
type LLMProvider interface {
	// Complete generates a complete response.
	Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error)

	// Stream generates a streaming response.
	Stream(ctx context.Context, req *LLMRequest, handler func(*StreamChunk) error) error

	// Embed generates embeddings for the given text.
	Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error)

	// Name returns the provider name.
	Name() ModelName

	// Stats returns the provider statistics.
	Stats() *ProviderStats
}
