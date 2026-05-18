// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
package llmrouter

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

// RoutingStrategy defines how to select a model.
type RoutingStrategy string

const (
	// RoutingByTaskType selects model based on task type.
	RoutingByTaskType RoutingStrategy = "task_type"
	// RoutingByCost selects the cheapest model.
	RoutingByCost RoutingStrategy = "cost"
	// RoutingByLatency selects the fastest model.
	RoutingByLatency RoutingStrategy = "latency"
	// RoutingByCapability selects based on required capabilities.
	RoutingByCapability RoutingStrategy = "capability"
	// RoutingRoundRobin selects models in round-robin fashion across providers.
	RoutingRoundRobin RoutingStrategy = "round_robin"
)

// UpgradeRule defines when to upgrade to a more capable model.
type UpgradeRule struct {
	// ConsecutiveFailures is the number of consecutive failures to trigger upgrade.
	ConsecutiveFailures int
	// ErrorRateThreshold is the error rate to trigger upgrade.
	ErrorRateThreshold float64
}

// DowngradeRule defines when to downgrade to a cheaper model.
type DowngradeRule struct {
	// SuccessCount is the number of consecutive successes to allow downgrade.
	SuccessCount int
	// MaxCostSavings is the maximum cost savings percentage.
	MaxCostSavings float64
}

// ModelTier represents a tier of model capability.
type ModelTier int

const (
	TierLow ModelTier = iota
	TierMedium
	TierHigh
)

// GetModelTier returns the tier for a model.
func GetModelTier(model ModelName) ModelTier {
	switch model {
	case ModelClaudeHaiku, ModelGLM5Air:
		return TierLow
	case ModelClaudeSonnet, ModelGLM5:
		return TierMedium
	default:
		return TierMedium
	}
}

// UpgradeModel returns the next tier model for upgrades.
func UpgradeModel(model ModelName) ModelName {
	switch model {
	case ModelClaudeHaiku:
		return ModelClaudeSonnet
	case ModelGLM5Air:
		return ModelGLM5
	case ModelClaudeSonnet, ModelGLM5:
		// Already at high tier, stay at current model
		return model
	default:
		return ModelClaudeSonnet
	}
}

// DowngradeModel returns the next tier model for downgrades.
func DowngradeModel(model ModelName) ModelName {
	switch model {
	case ModelClaudeSonnet:
		return ModelClaudeHaiku
	case ModelGLM5:
		return ModelGLM5Air
	case ModelClaudeHaiku, ModelGLM5Air:
		// Already at low tier
		return model
	default:
		return ModelClaudeSonnet
	}
}

// ModelSelector selects the best model for a request.
type ModelSelector struct {
	config   *Config
	logger   *zap.Logger
	circuits map[ModelName]*CircuitBreaker
	mu       sync.RWMutex
	// roundRobinIndex is used for round-robin load balancing.
	roundRobinIndex uint32
	// availableModels is a list of currently available models for round-robin.
	availableModels []ModelName
}

// NewModelSelector creates a new model selector.
func NewModelSelector(config *Config, logger *zap.Logger) *ModelSelector {
	if logger == nil {
		logger = zap.NewNop()
	}
	selector := &ModelSelector{
		config:   config,
		logger:   logger,
		circuits: make(map[ModelName]*CircuitBreaker),
	}

	// Initialize circuit breakers for each provider
	for model := range config.ProviderConfigs {
		selector.circuits[model] = NewCircuitBreaker(config.CircuitBreakerConfig)
	}

	return selector
}

// SelectModel selects the best model for the given task type.
func (s *ModelSelector) SelectModel(taskType TaskType, strategy RoutingStrategy) ModelName {
	s.mu.RLock()
	defer s.mu.RUnlock()

	switch strategy {
	case RoutingByTaskType:
		return s.selectByTaskType(taskType)
	case RoutingByCost:
		return s.selectByCost()
	case RoutingByLatency:
		return s.selectByLatency()
	case RoutingRoundRobin:
		return s.selectByRoundRobin()
	default:
		return s.selectByTaskType(taskType)
	}
}

// selectByTaskType selects model based on task type preferences.
func (s *ModelSelector) selectByTaskType(taskType TaskType) ModelName {
	for model, cfg := range s.config.ProviderConfigs {
		// Check circuit breaker
		if cb, ok := s.circuits[model]; ok && cb.State() == CircuitStateOpen {
			continue
		}
		// Check if this model prefers this task type
		for _, pref := range cfg.TaskPreferences {
			if pref == taskType {
				return model
			}
		}
	}
	// Fallback to primary provider
	return s.config.PrimaryProvider
}

// selectByCost selects the cheapest available model.
func (s *ModelSelector) selectByCost() ModelName {
	var cheapest ModelName
	var minCost float64 = -1

	for model, cfg := range s.config.ProviderConfigs {
		if cb, ok := s.circuits[model]; ok && cb.State() == CircuitStateOpen {
			continue
		}
		if minCost < 0 || cfg.CostPer1KTokens < minCost {
			minCost = cfg.CostPer1KTokens
			cheapest = model
		}
	}

	if cheapest == "" {
		return s.config.PrimaryProvider
	}
	return cheapest
}

// selectByLatency selects the fastest available model.
func (s *ModelSelector) selectByLatency() ModelName {
	var fastest ModelName
	var minLatency int64 = -1

	for model, cfg := range s.config.ProviderConfigs {
		if cb, ok := s.circuits[model]; ok && cb.State() == CircuitStateOpen {
			continue
		}
		if minLatency < 0 || cfg.AvgLatencyMs < minLatency {
			minLatency = cfg.AvgLatencyMs
			fastest = model
		}
	}

	if fastest == "" {
		return s.config.PrimaryProvider
	}
	return fastest
}

// selectByRoundRobin selects the next available model in round-robin fashion.
func (s *ModelSelector) selectByRoundRobin() ModelName {
	// Build list of available models (those with closed circuits)
	var available []ModelName
	for model, cb := range s.circuits {
		if cb.State() == CircuitStateClosed {
			available = append(available, model)
		}
	}

	if len(available) == 0 {
		return s.config.PrimaryProvider
	}

	// Use atomic add to get next index (thread-safe)
	idx := atomic.AddUint32(&s.roundRobinIndex, 1) - 1
	return available[idx%uint32(len(available))]
}

// RecordSuccess records a successful request for the model.
func (s *ModelSelector) RecordSuccess(model ModelName) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if cb, ok := s.circuits[model]; ok {
		cb.RecordSuccess()
	}
}

// RecordFailure records a failed request for the model.
func (s *ModelSelector) RecordFailure(model ModelName) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if cb, ok := s.circuits[model]; ok {
		cb.RecordFailure()
	}
}

// GetCircuitState returns the circuit breaker state for a model.
func (s *ModelSelector) GetCircuitState(model ModelName) CircuitState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if cb, ok := s.circuits[model]; ok {
		return cb.State()
	}
	return CircuitStateClosed
}

// IsModelAvailable checks if a model is available (circuit not open).
func (s *ModelSelector) IsModelAvailable(model ModelName) bool {
	return s.GetCircuitState(model) != CircuitStateOpen
}

// Router handles LLM request routing with adaptive upgrade/downgrade.
type Router struct {
	config        *Config
	logger        *zap.Logger
	providers     map[ModelName]LLMProvider
	selector      *ModelSelector
	retryPolicy   *RetryPolicy
	consecutiveFails map[ModelName]int
	consecutiveSuccesses map[ModelName]int
	mu            sync.RWMutex
}

// NewRouter creates a new LLM router.
func NewRouter(config *Config, logger *zap.Logger) *Router {
	if logger == nil {
		logger = zap.NewNop()
	}
	if config == nil {
		config = DefaultConfig()
	}

	r := &Router{
		config:        config,
		logger:        logger,
		providers:     make(map[ModelName]LLMProvider),
		selector:      NewModelSelector(config, logger),
		retryPolicy:   NewRetryPolicy(config.RetryConfig),
		consecutiveFails: make(map[ModelName]int),
		consecutiveSuccesses: make(map[ModelName]int),
	}

	return r
}

// RegisterProvider registers an LLM provider.
func (r *Router) RegisterProvider(provider LLMProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.Name()] = provider
}

// Complete routes a request to the appropriate LLM provider.
func (r *Router) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	// If model is specified, use it directly
	model := req.Model
	if model == "" || !r.selector.IsModelAvailable(model) {
		model = r.selector.SelectModel(req.TaskType, RoutingByTaskType)
	}

	provider, ok := r.providers[model]
	if !ok {
		return nil, fmt.Errorf("no provider registered for model: %s", model)
	}

	// Check circuit breaker
	if !r.selector.IsModelAvailable(model) {
		// Try to find an alternative model
		altModel := r.findAlternativeModel(model)
		if altModel != "" {
			r.logger.Info("circuit open, switching to alternative model",
				zap.String("original", string(model)),
				zap.String("alternative", string(altModel)))
			model = altModel
			provider = r.providers[model]
		}
	}

	// Execute with retry
	var lastResp *LLMResponse
	var lastErr error

	err := DoWithRetry(ctx, r.retryPolicy, func() error {
		resp, err := provider.Complete(ctx, req)
		if err != nil {
			r.handleFailure(model)
			lastErr = err
			return err
		}
		lastResp = resp
		r.handleSuccess(model)
		return nil
	})

	if err != nil {
		// Try fallback providers if primary fails
		if fallbackResp, fallbackErr := r.tryFallback(ctx, req, model); fallbackResp != nil {
			r.logger.Info("primary provider failed, using fallback",
				zap.String("primary", string(model)),
				zap.Error(fallbackErr))
			return fallbackResp, nil
		}

		// Try upgrade if available
		if r.config.EnableAdaptiveRouting {
			upgradedModel := r.tryUpgrade(model)
			if upgradedModel != model && upgradedModel != "" {
				r.logger.Info("upgrading model after failures",
					zap.String("from", string(model)),
					zap.String("to", string(upgradedModel)))
				req.Model = upgradedModel
				return r.Complete(ctx, req)
			}
		}
		return nil, lastErr
	}

	return lastResp, nil
}

// tryFallback tries fallback providers in order when the primary fails.
func (r *Router) tryFallback(ctx context.Context, req *LLMRequest, failedModel ModelName) (*LLMResponse, error) {
	// Build fallback order: use configured order, but skip the failed model
	var fallbackOrder []ModelName
	if len(r.config.FallbackOrder) > 0 {
		fallbackOrder = r.config.FallbackOrder
	} else {
		// Default fallback order based on available providers
		for m := range r.providers {
			if m != failedModel {
				fallbackOrder = append(fallbackOrder, m)
			}
		}
	}

	for _, candidate := range fallbackOrder {
		if candidate == failedModel {
			continue
		}
		if !r.selector.IsModelAvailable(candidate) {
			continue
		}
		provider, ok := r.providers[candidate]
		if !ok {
			continue
		}

		r.logger.Info("trying fallback provider",
			zap.String("failed", string(failedModel)),
			zap.String("trying", string(candidate)))

		resp, err := provider.Complete(ctx, req)
		if err == nil {
			r.handleSuccess(candidate)
			r.logger.Info("fallback provider succeeded",
				zap.String("candidate", string(candidate)))
			return resp, nil
		}
		r.handleFailure(candidate)
	}

	return nil, fmt.Errorf("all fallback providers failed for model: %s", failedModel)
}

// Stream routes a streaming request to the appropriate LLM provider.
func (r *Router) Stream(ctx context.Context, req *LLMRequest, handler func(*StreamChunk) error) error {
	model := req.Model
	if model == "" || !r.selector.IsModelAvailable(model) {
		model = r.selector.SelectModel(req.TaskType, RoutingByTaskType)
	}

	provider, ok := r.providers[model]
	if !ok {
		return fmt.Errorf("no provider registered for model: %s", model)
	}

	return provider.Stream(ctx, req, handler)
}

// Embed routes an embedding request.
func (r *Router) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	model := r.selector.SelectModel(TaskTypeSimple, RoutingByCost)

	provider, ok := r.providers[model]
	if !ok {
		return nil, fmt.Errorf("no provider registered for model: %s", model)
	}

	var lastResp *EmbedResponse
	var lastErr error

	err := DoWithRetry(ctx, r.retryPolicy, func() error {
		resp, err := provider.Embed(ctx, req)
		if err != nil {
			lastErr = err
			return err
		}
		lastResp = resp
		return nil
	})

	if err != nil {
		return nil, lastErr
	}
	return lastResp, nil
}

// handleFailure records a failure and potentially triggers downgrade.
func (r *Router) handleFailure(model ModelName) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.consecutiveFails[model]++
	r.consecutiveSuccesses[model] = 0
	r.selector.RecordFailure(model)

	r.logger.Debug("recorded failure",
		zap.String("model", string(model)),
		zap.Int("consecutive_fails", r.consecutiveFails[model]))
}

// handleSuccess records a success and potentially allows downgrade.
func (r *Router) handleSuccess(model ModelName) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.consecutiveSuccesses[model]++
	r.selector.RecordSuccess(model)

	// Reset consecutive failures after success
	r.consecutiveFails[model] = 0

	r.logger.Debug("recorded success",
		zap.String("model", string(model)),
		zap.Int("consecutive_successes", r.consecutiveSuccesses[model]))
}

// tryUpgrade attempts to upgrade to a more capable model.
func (r *Router) tryUpgrade(current ModelName) ModelName {
	upgraded := UpgradeModel(current)
	if upgraded == current {
		return current
	}

	// Check if upgraded model is available
	if !r.selector.IsModelAvailable(upgraded) {
		return current
	}

	// Only upgrade after consecutive failures
	r.mu.RLock()
	fails := r.consecutiveFails[current]
	r.mu.RUnlock()

	if fails >= 3 {
		return upgraded
	}
	return current
}

// tryDowngrade attempts to downgrade to a cheaper model after sustained success.
func (r *Router) tryDowngrade(current ModelName) ModelName {
	downgraded := DowngradeModel(current)
	if downgraded == current {
		return current
	}

	// Check if downgraded model is available
	if !r.selector.IsModelAvailable(downgraded) {
		return current
	}

	// Only downgrade after consecutive successes
	r.mu.RLock()
	successes := r.consecutiveSuccesses[current]
	r.mu.RUnlock()

	if successes >= 10 {
		return downgraded
	}
	return current
}

// findAlternativeModel finds an available alternative model.
func (r *Router) findAlternativeModel(unavailable ModelName) ModelName {
	// Try to find any available model
	for model := range r.providers {
		if model != unavailable && r.selector.IsModelAvailable(model) {
			return model
		}
	}
	return ""
}

// GetStats returns statistics for all models.
func (r *Router) GetStats() map[ModelName]*ProviderStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[ModelName]*ProviderStats)
	for model, provider := range r.providers {
		stats[model] = provider.Stats()
		stats[model].FailedRequests = int64(r.consecutiveFails[model])
	}
	return stats
}

// GetCircuitStates returns the circuit breaker states.
func (r *Router) GetCircuitStates() map[ModelName]CircuitState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	states := make(map[ModelName]CircuitState)
	for model, cb := range r.selector.circuits {
		states[model] = cb.State()
	}
	return states
}
