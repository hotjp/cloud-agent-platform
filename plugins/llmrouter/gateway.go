// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
package llmrouter

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/knadh/koanf/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"go.uber.org/zap"
)

// LLMGateway is the unified entry point for LLM routing with interception.
type LLMGateway struct {
	router      *LLMRouter
	logger      *zap.Logger
	interceptor *InterceptorChain
	rateLimiter *RateLimiter
	config      *GatewayConfig
	mu          sync.RWMutex
}

// GatewayConfig holds configuration for the LLMGateway.
type GatewayConfig struct {
	// EnableLogging enables request/response logging.
	EnableLogging bool
	// EnableMetrics enables metrics collection.
	EnableMetrics bool
	// EnableRateLimiting enables rate limiting.
	EnableRateLimiting bool
	// RequestTimeout is the global request timeout.
	RequestTimeout time.Duration
	// RateLimitConfig holds rate limiting configuration.
	RateLimitConfig RateLimitConfig
	// InterceptorConfig holds interceptor configuration.
	InterceptorConfig InterceptorConfig
}

// RateLimitConfig holds rate limiting configuration.
type RateLimitConfig struct {
	// RequestsPerSecond is the global requests per second limit.
	RequestsPerSecond float64
	// BurstSize is the burst size for token bucket.
	BurstSize int
	// PerProviderLimits enables per-provider rate limiting.
	PerProviderLimits bool
}

// InterceptorConfig holds interceptor configuration.
type InterceptorConfig struct {
	// LogRequests enables request logging.
	LogRequests bool
	// LogResponses enables response logging.
	LogResponses bool
	// LogLatency enables latency logging.
	LogLatency bool
	// MetricsEnabled enables metrics collection.
	MetricsEnabled bool
}

// RateLimiter implements a simple token bucket rate limiter.
type RateLimiter struct {
	config  RateLimitConfig
	buckets map[ModelName]*tokenBucket
	mu      sync.RWMutex
	global  *tokenBucket
}

// tokenBucket implements the token bucket algorithm.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket.
func NewTokenBucket(maxTokens float64, refillRate float64) *tokenBucket {
	return &tokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed.
func (tb *tokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

// refill adds tokens based on elapsed time.
func (tb *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now
}

// InterceptorChain holds a chain of interceptors.
type InterceptorChain struct {
	config  InterceptorConfig
	logger  *zap.Logger
	metrics *GatewayMetrics
}

// GatewayMetrics holds gateway-level metrics.
type GatewayMetrics struct {
	TotalRequests    int64
	TotalResponses   int64
	TotalErrors      int64
	TotalLatencyMs   int64
	RequestsByModel  map[ModelName]int64
	ErrorsByModel    map[ModelName]int64
	LatencyByModel   map[ModelName]int64
	mu               sync.Mutex
}

// NewGatewayMetrics creates a new metrics collector.
func NewGatewayMetrics() *GatewayMetrics {
	return &GatewayMetrics{
		RequestsByModel: make(map[ModelName]int64),
		ErrorsByModel:   make(map[ModelName]int64),
		LatencyByModel:  make(map[ModelName]int64),
	}
}

// RecordRequest records a request.
func (m *GatewayMetrics) RecordRequest(model ModelName) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalRequests++
	m.RequestsByModel[model]++
}

// RecordResponse records a response.
func (m *GatewayMetrics) RecordResponse(model ModelName, latencyMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalResponses++
	m.LatencyByModel[model] += latencyMs
}

// RecordError records an error.
func (m *GatewayMetrics) RecordError(model ModelName) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalErrors++
	m.ErrorsByModel[model]++
}

// NewLLMGateway creates a new LLM gateway.
func NewLLMGateway(cfg *GatewayConfig, router *LLMRouter, logger *zap.Logger) *LLMGateway {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg == nil {
		cfg = DefaultGatewayConfig()
	}

	g := &LLMGateway{
		router:      router,
		logger:      logger,
		interceptor: NewInterceptorChain(cfg.InterceptorConfig, logger),
		rateLimiter: NewRateLimiter(cfg.RateLimitConfig),
		config:      cfg,
	}

	return g
}

// NewInterceptorChain creates a new interceptor chain.
func NewInterceptorChain(cfg InterceptorConfig, logger *zap.Logger) *InterceptorChain {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &InterceptorChain{
		config:  cfg,
		logger:  logger,
		metrics: NewGatewayMetrics(),
	}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config:  cfg,
		buckets: make(map[ModelName]*tokenBucket),
	}

	// Initialize global bucket
	if cfg.RequestsPerSecond > 0 {
		rl.global = NewTokenBucket(float64(cfg.BurstSize), cfg.RequestsPerSecond)
	}

	return rl
}

// Allow checks if a request to the given model is allowed.
func (rl *RateLimiter) Allow(model ModelName) bool {
	if rl == nil || rl.global == nil {
		return true
	}

	// Check global rate limit
	if !rl.global.Allow() {
		return false
	}

	// Check per-provider rate limit if enabled
	if rl.config.PerProviderLimits {
		rl.mu.RLock()
		bucket, exists := rl.buckets[model]
		rl.mu.RUnlock()

		if !exists {
			rl.mu.Lock()
			// Double-check after acquiring lock
			if bucket, exists = rl.buckets[model]; !exists {
				bucket = NewTokenBucket(float64(rl.config.BurstSize), rl.config.RequestsPerSecond)
				rl.buckets[model] = bucket
			}
			rl.mu.Unlock()
		}

		return bucket.Allow()
	}

	return true
}

// Complete processes a request through the gateway.
func (g *LLMGateway) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	start := time.Now()
	model := req.Model
	if model == "" {
		model = ModelClaudeSonnet
	}

	// Apply request timeout
	if g.config.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.config.RequestTimeout)
		defer cancel()
	}

	// Check rate limit
	if g.rateLimiter != nil && !g.rateLimiter.Allow(model) {
		g.interceptor.metrics.RecordError(model)
		return nil, &GatewayError{
			Code:    ErrRateLimited,
			Message: "rate limit exceeded",
			Model:   model,
		}
	}

	// Pre-request interception
	g.interceptor.OnRequest(req)

	// Execute request
	resp, err := g.router.Complete(ctx, req)

	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		// Post-error interception
		g.interceptor.OnError(req, err)
		g.interceptor.metrics.RecordError(model)
		g.logger.Error("LLM request failed",
			zap.String("model", string(model)),
			zap.String("task_type", string(req.TaskType)),
			zap.Int64("latency_ms", latencyMs),
			zap.Error(err))
		return nil, &GatewayError{
			Code:    ErrProviderError,
			Message: err.Error(),
			Model:   model,
			Cause:   err,
		}
	}

	// Post-response interception
	g.interceptor.OnResponse(req, resp)

	if resp != nil {
		g.interceptor.metrics.RecordResponse(model, latencyMs)
	}

	g.logger.Info("LLM request completed",
		zap.String("model", string(model)),
		zap.String("task_type", string(req.TaskType)),
		zap.Int64("latency_ms", latencyMs),
		zap.Int("tokens_used", resp.TokensUsed))

	return resp, nil
}

// Stream processes a streaming request through the gateway.
func (g *LLMGateway) Stream(ctx context.Context, req *LLMRequest, handler func(*StreamChunk) error) error {
	start := time.Now()
	model := req.Model
	if model == "" {
		model = ModelClaudeSonnet
	}

	// Apply request timeout
	if g.config.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.config.RequestTimeout)
		defer cancel()
	}

	// Check rate limit
	if g.rateLimiter != nil && !g.rateLimiter.Allow(model) {
		g.interceptor.metrics.RecordError(model)
		return &GatewayError{
			Code:    ErrRateLimited,
			Message: "rate limit exceeded",
			Model:   model,
		}
	}

	// Wrap handler to intercept streaming chunks
	wrappedHandler := func(chunk *StreamChunk) error {
		if chunk != nil {
			g.interceptor.OnStreamChunk(chunk)
		}
		return handler(chunk)
	}

	err := g.router.Stream(ctx, req, wrappedHandler)

	if err != nil {
		g.interceptor.OnError(req, err)
		g.interceptor.metrics.RecordError(model)
		return &GatewayError{
			Code:    ErrProviderError,
			Message: err.Error(),
			Model:   model,
			Cause:   err,
		}
	}

	g.interceptor.metrics.RecordResponse(model, time.Since(start).Milliseconds())
	return nil
}

// Embed processes an embedding request through the gateway.
func (g *LLMGateway) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	start := time.Now()
	model := ModelClaudeSonnet // Default for embeddings

	// Apply request timeout
	if g.config.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.config.RequestTimeout)
		defer cancel()
	}

	// Check rate limit
	if g.rateLimiter != nil && !g.rateLimiter.Allow(model) {
		g.interceptor.metrics.RecordError(model)
		return nil, &GatewayError{
			Code:    ErrRateLimited,
			Message: "rate limit exceeded",
			Model:   model,
		}
	}

	resp, err := g.router.Embed(ctx, req)

	if err != nil {
		g.interceptor.metrics.RecordError(model)
		return nil, &GatewayError{
			Code:    ErrProviderError,
			Message: err.Error(),
			Model:   model,
			Cause:   err,
		}
	}

	g.interceptor.metrics.RecordResponse(model, time.Since(start).Milliseconds())
	return resp, nil
}

// OnRequest is called before a request is sent to the provider.
func (ic *InterceptorChain) OnRequest(req *LLMRequest) {
	if ic.config.LogRequests {
		ic.logger.Debug("LLM request",
			zap.String("model", string(req.Model)),
			zap.String("task_type", string(req.TaskType)),
			zap.Int("max_tokens", req.MaxTokens),
			zap.Float64("temperature", req.Temperature))
	}
}

// OnResponse is called after a response is received.
func (ic *InterceptorChain) OnResponse(req *LLMRequest, resp *LLMResponse) {
	if ic.config.LogResponses && resp != nil {
		contentLen := len(resp.Content)
		ic.logger.Debug("LLM response",
			zap.String("model", string(resp.Model)),
			zap.Int("tokens_used", resp.TokensUsed),
			zap.Int64("latency_ms", resp.LatencyMs),
			zap.Int("content_len", contentLen))
	}
}

// OnStreamChunk is called for each streaming chunk.
func (ic *InterceptorChain) OnStreamChunk(chunk *StreamChunk) {
	// Streaming chunks are high-volume, only log if debug is enabled
	if chunk.Done {
		ic.logger.Debug("stream completed")
	}
}

// OnError is called when an error occurs.
func (ic *InterceptorChain) OnError(req *LLMRequest, err error) {
	ic.logger.Error("LLM error",
		zap.String("model", string(req.Model)),
		zap.String("task_type", string(req.TaskType)),
		zap.Error(err))
}

// GetMetrics returns the gateway metrics.
func (g *LLMGateway) GetMetrics() *GatewayMetrics {
	return g.interceptor.metrics
}

// GetProviderStats returns stats for all providers.
func (g *LLMGateway) GetProviderStats() map[ModelName]*ProviderStats {
	return g.router.GetStats()
}

// GetCircuitStates returns the circuit breaker states.
func (g *LLMGateway) GetCircuitStates() map[ModelName]CircuitState {
	return g.router.GetCircuitStates()
}

// DefaultGatewayConfig returns the default gateway configuration.
func DefaultGatewayConfig() *GatewayConfig {
	return &GatewayConfig{
		EnableLogging:       true,
		EnableMetrics:       true,
		EnableRateLimiting:  true,
		RequestTimeout:      60 * time.Second,
		RateLimitConfig: RateLimitConfig{
			RequestsPerSecond: 100,
			BurstSize:        20,
			PerProviderLimits: false,
		},
		InterceptorConfig: InterceptorConfig{
			LogRequests:    true,
			LogResponses:   false,
			LogLatency:     true,
			MetricsEnabled: true,
		},
	}
}

// GatewayErrorCode represents an error code.
type GatewayErrorCode string

const (
	// ErrRateLimited indicates rate limit was exceeded.
	ErrRateLimited GatewayErrorCode = "rate_limited"
	// ErrProviderError indicates a provider error.
	ErrProviderError GatewayErrorCode = "provider_error"
	// ErrTimeout indicates a timeout.
	ErrTimeout GatewayErrorCode = "timeout"
	// ErrInvalidRequest indicates an invalid request.
	ErrInvalidRequest GatewayErrorCode = "invalid_request"
)

// GatewayError represents a gateway-level error.
type GatewayError struct {
	Code    GatewayErrorCode
	Message string
	Model   ModelName
	Cause   error
}

func (e *GatewayError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *GatewayError) Unwrap() error {
	return e.Cause
}

// LoadConfigFromKoanf loads configuration from koanf.
func LoadConfigFromKoanf(k *koanf.Koanf) (*Config, *GatewayConfig, error) {
	// Load base config
	cfg := DefaultConfig()

	// Override from koanf if present
	if k.Exists("llm.primary_provider") {
		if v := k.Get("llm.primary_provider"); v != nil {
			if s, ok := v.(string); ok {
				cfg.PrimaryProvider = ModelName(s)
			}
		}
	}

	if k.Exists("llm.enable_adaptive_routing") {
		if v := k.Get("llm.enable_adaptive_routing"); v != nil {
			if b, ok := v.(bool); ok {
				cfg.EnableAdaptiveRouting = b
			}
		}
	}

	// Load provider configs
	if k.Exists("llm.providers") {
		providerCfgs := k.Get("llm.providers")
		if providerMap, ok := providerCfgs.(map[string]any); ok {
			for modelName, pcfg := range providerMap {
				model := ModelName(modelName)
				if p, ok := pcfg.(map[string]any); ok {
					provCfg := ProviderConfig{}
					if apiKey, ok := p["api_key"].(string); ok {
						provCfg.APIKey = apiKey
					}
					if endpoint, ok := p["endpoint"].(string); ok {
						provCfg.Endpoint = endpoint
					}
					if timeout, ok := p["timeout"].(string); ok {
						if d, err := time.ParseDuration(timeout); err == nil {
							provCfg.Timeout = d
						}
					}
					if rateLimit, ok := p["rate_limit"].(float64); ok {
						provCfg.RateLimit = rateLimit
					}
					cfg.ProviderConfigs[model] = provCfg
				}
			}
		}
	}

	// Load fallback order
	if k.Exists("llm.fallback_order") {
		if v := k.Get("llm.fallback_order"); v != nil {
			if fallbacks, ok := v.([]any); ok {
				fallbackOrder := make([]ModelName, 0, len(fallbacks))
				for _, fb := range fallbacks {
					if s, ok := fb.(string); ok {
						fallbackOrder = append(fallbackOrder, ModelName(s))
					}
				}
				cfg.FallbackOrder = fallbackOrder
			}
		}
	}

	// Load gateway config
	gatewayCfg := DefaultGatewayConfig()

	if k.Exists("gateway.enable_logging") {
		if v := k.Get("gateway.enable_logging"); v != nil {
			if b, ok := v.(bool); ok {
				gatewayCfg.EnableLogging = b
			}
		}
	}

	if k.Exists("gateway.enable_metrics") {
		if v := k.Get("gateway.enable_metrics"); v != nil {
			if b, ok := v.(bool); ok {
				gatewayCfg.EnableMetrics = b
			}
		}
	}

	if k.Exists("gateway.enable_rate_limiting") {
		if v := k.Get("gateway.enable_rate_limiting"); v != nil {
			if b, ok := v.(bool); ok {
				gatewayCfg.EnableRateLimiting = b
			}
		}
	}

	if k.Exists("gateway.request_timeout") {
		if v := k.Get("gateway.request_timeout"); v != nil {
			if timeout, ok := v.(string); ok {
				if d, err := time.ParseDuration(timeout); err == nil {
					gatewayCfg.RequestTimeout = d
				}
			}
		}
	}

	if k.Exists("gateway.rate_limit") {
		if v := k.Get("gateway.rate_limit"); v != nil {
			if rlMap, ok := v.(map[string]any); ok {
				if rps, ok := rlMap["requests_per_second"].(float64); ok {
					gatewayCfg.RateLimitConfig.RequestsPerSecond = rps
				}
				if burst, ok := rlMap["burst_size"].(float64); ok {
					gatewayCfg.RateLimitConfig.BurstSize = int(burst)
				}
			}
		}
	}

	return cfg, gatewayCfg, nil
}

// LoadKoanfFromFiles loads configuration from files with koanf.
func LoadKoanfFromFiles(configFiles ...string) (*koanf.Koanf, error) {
	k := koanf.New(".")

	// Load default config via structs (using default configs)
	if err := k.Load(structs.Provider(DefaultConfigWithKoanf(), "koanf"), nil); err != nil {
		return nil, err
	}

	// Load from environment variables with LLMR_ prefix
	if err := k.Load(env.Provider("LLMR_", ".", func(s string) string {
		// Convert LLMR_RATE_LIMIT_RPS to llm.rate_limit.requests_per_second
		s = strings.ToLower(s)
		s = strings.TrimPrefix(s, "llmr_")
		s = strings.ReplaceAll(s, "_", ".")
		return s
	}), nil); err != nil {
		return nil, err
	}

	// Load from config files (YAML format)
	for _, f := range configFiles {
		if err := k.Load(file.Provider(f), yaml.Parser()); err != nil {
			return nil, err
		}
	}

	return k, nil
}

// DefaultConfigWithKoanf returns a struct with default values for koanf loading.
func DefaultConfigWithKoanf() *KoanfDefaults {
	return &KoanfDefaults{
		LLMGateway: KoanfLLMGatewayDefaults{
			PrimaryProvider:        string(ModelClaudeSonnet),
			EnableAdaptiveRouting: true,
		},
		Gateway: KoanfGatewayDefaults{
			EnableLogging:       true,
			EnableMetrics:       true,
			EnableRateLimiting: true,
			RequestTimeout:      "60s",
			RateLimit: KoanfRateLimitDefaults{
				RequestsPerSecond: 100.0,
				BurstSize:        20.0,
			},
		},
	}
}

// KoanfDefaults holds default values for koanf configuration.
type KoanfDefaults struct {
	LLMGateway KoanfLLMGatewayDefaults `koanf:"llm"`
	Gateway     KoanfGatewayDefaults    `koanf:"gateway"`
}

// KoanfLLMGatewayDefaults holds LLM gateway defaults.
type KoanfLLMGatewayDefaults struct {
	PrimaryProvider        string `koanf:"primary_provider"`
	EnableAdaptiveRouting bool   `koanf:"enable_adaptive_routing"`
}

// KoanfGatewayDefaults holds gateway defaults.
type KoanfGatewayDefaults struct {
	EnableLogging       bool                 `koanf:"enable_logging"`
	EnableMetrics       bool                 `koanf:"enable_metrics"`
	EnableRateLimiting  bool                 `koanf:"enable_rate_limiting"`
	RequestTimeout      string               `koanf:"request_timeout"`
	RateLimit           KoanfRateLimitDefaults `koanf:"rate_limit"`
}

// KoanfRateLimitDefaults holds rate limit defaults.
type KoanfRateLimitDefaults struct {
	RequestsPerSecond float64 `koanf:"requests_per_second"`
	BurstSize         float64 `koanf:"burst_size"`
}

// ProviderRouter is an alias for Router for backwards compatibility.
type ProviderRouter = Router

// Ensure LLMGateway implements the gateway interface.
var _ interface {
	Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error)
	Stream(ctx context.Context, req *LLMRequest, handler func(*StreamChunk) error) error
	Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error)
	GetMetrics() *GatewayMetrics
	GetProviderStats() map[ModelName]*ProviderStats
	GetCircuitStates() map[ModelName]CircuitState
} = (*LLMGateway)(nil)