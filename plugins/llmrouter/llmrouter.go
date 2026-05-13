// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
// Selects optimal model based on task complexity, cost efficiency, and latency.
package llmrouter

import (
	"context"

	"go.uber.org/zap"

	"github.com/cloud-agent-platform/cap/plugins"
)

// LLMRouter handles multi-model LLM routing.
type LLMRouter struct {
	router  *Router
	logger  *zap.Logger
	enabled bool
}

// New creates a new LLMRouter instance.
func New(cfg *Config, logger *zap.Logger) *LLMRouter {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	r := &LLMRouter{
		router:  NewRouter(cfg, logger),
		logger:  logger,
		enabled: true,
	}

	return r
}

// NewFromConfig creates a new LLMRouter with the given config and registers providers.
func NewFromConfig(cfg *Config, logger *zap.Logger) *LLMRouter {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	router := NewRouter(cfg, logger)

	// Register default providers if API keys are configured
	for model, providerCfg := range cfg.ProviderConfigs {
		if providerCfg.APIKey != "" {
			switch {
			case model == ModelClaudeSonnet || model == ModelClaudeHaiku:
				provider := NewClaudeProvider(model, providerCfg.APIKey, providerCfg.Endpoint, providerCfg.Timeout, logger)
				router.RegisterProvider(provider)
				logger.Info("registered Claude provider", zap.String("model", string(model)))
			case model == ModelGLM5 || model == ModelGLM5Air:
				provider := NewGLMProvider(model, providerCfg.APIKey, providerCfg.Endpoint, providerCfg.Timeout, logger)
				router.RegisterProvider(provider)
				logger.Info("registered GLM provider", zap.String("model", string(model)))
			}
		}
	}

	return &LLMRouter{
		router:  router,
		logger:  logger,
		enabled: cfg.PrimaryProvider != "",
	}
}

// Complete generates a complete response.
func (r *LLMRouter) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	if !r.enabled {
		return nil, nil
	}
	return r.router.Complete(ctx, req)
}

// Stream generates a streaming response.
func (r *LLMRouter) Stream(ctx context.Context, req *LLMRequest, handler func(*StreamChunk) error) error {
	if !r.enabled {
		return nil
	}
	return r.router.Stream(ctx, req, handler)
}

// Embed generates embeddings.
func (r *LLMRouter) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	if !r.enabled {
		return nil, nil
	}
	return r.router.Embed(ctx, req)
}

// GetStats returns statistics for all models.
func (r *LLMRouter) GetStats() map[ModelName]*ProviderStats {
	if !r.enabled {
		return nil
	}
	return r.router.GetStats()
}

// GetCircuitStates returns the circuit breaker states.
func (r *LLMRouter) GetCircuitStates() map[ModelName]CircuitState {
	if !r.enabled {
		return nil
	}
	return r.router.GetCircuitStates()
}

// RegisterProvider registers an LLM provider.
func (r *LLMRouter) RegisterProvider(provider LLMProvider) {
	r.router.RegisterProvider(provider)
}

// Initialize implements plugins.Plugin.
func (r *LLMRouter) Initialize() error {
	r.logger.Info("LLMRouter initialized", zap.Bool("enabled", r.enabled))
	return nil
}

// Shutdown implements plugins.Plugin.
func (r *LLMRouter) Shutdown() error {
	r.logger.Info("LLMRouter shutting down")
	return nil
}

// Enabled implements plugins.Plugin.
func (r *LLMRouter) Enabled() bool {
	return r.enabled
}

// Ensure LLMRouter implements plugins.Plugin interface.
var _ plugins.Plugin = (*LLMRouter)(nil)
