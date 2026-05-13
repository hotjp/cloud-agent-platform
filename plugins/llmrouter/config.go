// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
package llmrouter

import (
	"time"
)

// Config holds the configuration for the LLM router.
type Config struct {
	// PrimaryProvider is the primary LLM provider.
	PrimaryProvider ModelName
	// EnableAdaptiveRouting enables adaptive model selection.
	EnableAdaptiveRouting bool
	// RetryConfig holds retry settings.
	RetryConfig RetryConfig
	// CircuitBreakerConfig holds circuit breaker settings.
	CircuitBreakerConfig CircuitBreakerConfig
	// ProviderConfigs holds per-provider settings.
	ProviderConfigs map[ModelName]ProviderConfig
}

// ProviderConfig holds configuration for a specific provider.
type ProviderConfig struct {
	// APIKey is the API key for the provider.
	APIKey string
	// Endpoint is the API endpoint.
	Endpoint string
	// Timeout is the request timeout.
	Timeout time.Duration
	// MaxRetries is the maximum number of retries.
	MaxRetries int
	// RateLimit is the requests per second limit.
	RateLimit float64
	// TaskPreferences maps task types to this provider.
	TaskPreferences []TaskType
	// CostPer1KTokens is the cost per 1K input tokens.
	CostPer1KTokens float64
	// AvgLatencyMs is the average latency in milliseconds.
	AvgLatencyMs int64
	// MaxTokens is the maximum tokens for this model.
	MaxTokens int
	// Capabilities supported by this provider.
	Capabilities ProviderCapabilities
}

// ProviderCapabilities describes what a provider supports.
type ProviderCapabilities struct {
	SupportsStreaming bool
	SupportsReasoning bool
	SupportsEmbeddings bool
	MaxContextTokens int
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		PrimaryProvider: ModelClaudeSonnet,
		EnableAdaptiveRouting: true,
		RetryConfig: RetryConfig{
			MaxAttempts:      3,
			InitialDelayMs:   100,
			MaxDelayMs:       5000,
			Multiplier:       2.0,
			JitterPercent:    20,
		},
		CircuitBreakerConfig: CircuitBreakerConfig{
			ErrorRateThreshold: 0.5,
			MinRequests:         10,
			HalfOpenMaxRequests: 3,
			OpenTimeoutMs:       30000,
		},
		ProviderConfigs: map[ModelName]ProviderConfig{
			ModelClaudeSonnet: {
				TaskPreferences: []TaskType{TaskTypeAnalysis, TaskTypeCoding, TaskTypeResearch},
				CostPer1KTokens:  0.015,
				AvgLatencyMs:     2000,
				MaxTokens:        200000,
				Capabilities: ProviderCapabilities{
					SupportsStreaming: true,
					SupportsReasoning: false,
					SupportsEmbeddings: true,
					MaxContextTokens: 200000,
				},
			},
			ModelClaudeHaiku: {
				TaskPreferences: []TaskType{TaskTypeSimple, TaskTypeSummarize},
				CostPer1KTokens:  0.003,
				AvgLatencyMs:     500,
				MaxTokens:        200000,
				Capabilities: ProviderCapabilities{
					SupportsStreaming: true,
					SupportsReasoning: false,
					SupportsEmbeddings: true,
					MaxContextTokens: 200000,
				},
			},
			ModelGLM5: {
				TaskPreferences: []TaskType{TaskTypeReview, TaskTypeTesting},
				CostPer1KTokens:  0.01,
				AvgLatencyMs:     1500,
				MaxTokens:        128000,
				Capabilities: ProviderCapabilities{
					SupportsStreaming: true,
					SupportsReasoning: true,
					SupportsEmbeddings: true,
					MaxContextTokens: 128000,
				},
			},
			ModelGLM5Air: {
				TaskPreferences: []TaskType{TaskTypeSimple, TaskTypeSummarize},
				CostPer1KTokens:  0.002,
				AvgLatencyMs:     300,
				MaxTokens:        128000,
				Capabilities: ProviderCapabilities{
					SupportsStreaming: true,
					SupportsReasoning: true,
					SupportsEmbeddings: true,
					MaxContextTokens: 128000,
				},
			},
		},
	}
}

// GetProviderConfig returns the configuration for a specific provider.
func (c *Config) GetProviderConfig(name ModelName) ProviderConfig {
	if cfg, ok := c.ProviderConfigs[name]; ok {
		return cfg
	}
	return ProviderConfig{}
}
