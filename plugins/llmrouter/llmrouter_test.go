// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
package llmrouter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockProvider is a mock LLM provider for testing.
type mockProvider struct {
	name            ModelName
	shouldFail      bool
	failCount       int
	successCount    int
	consecutiveFails int
	mu              sync.Mutex
	calledWith     []*LLMRequest
}

func (m *mockProvider) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	m.mu.Lock()
	m.calledWith = append(m.calledWith, req)
	m.mu.Unlock()

	if m.shouldFail && m.failCount < m.consecutiveFails {
		m.mu.Lock()
		m.failCount++
		m.mu.Unlock()
		return nil, &RetryableError{Err: errors.New("mock error"), Retryable: true}
	}

	m.mu.Lock()
	m.successCount++
	m.failCount = 0
	m.mu.Unlock()

	return &LLMResponse{
		Content:    "mock response",
		Model:      m.name,
		TokensUsed: 100,
		LatencyMs:  500,
	}, nil
}

func (m *mockProvider) Stream(ctx context.Context, req *LLMRequest, handler func(*StreamChunk) error) error {
	m.mu.Lock()
	m.calledWith = append(m.calledWith, req)
	m.mu.Unlock()
	return nil
}

func (m *mockProvider) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	return &EmbedResponse{
		Embedding: []float32{0.1, 0.2, 0.3},
		Model:     m.name,
	}, nil
}

func (m *mockProvider) Name() ModelName {
	return m.name
}

func (m *mockProvider) Stats() *ProviderStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &ProviderStats{
		TotalRequests:   int64(m.successCount + m.failCount),
		SuccessRequests: int64(m.successCount),
		FailedRequests:  int64(m.failCount),
	}
}

// TestCircuitBreakerStateTransitions tests circuit breaker state transitions.
func TestCircuitBreakerStateTransitions(t *testing.T) {
	config := CircuitBreakerConfig{
		ErrorRateThreshold: 0.5,
		MinRequests:        5,
		HalfOpenMaxRequests: 3,
		OpenTimeoutMs:      100, // Short timeout for testing
	}

	cb := NewCircuitBreaker(config)
	logger := zap.NewNop()

	t.Run("initial state is closed", func(t *testing.T) {
		assert.Equal(t, CircuitStateClosed, cb.State())
	})

	t.Run("allows requests when closed", func(t *testing.T) {
		assert.True(t, cb.Allow())
	})

	t.Run("opens after error threshold", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			ErrorRateThreshold: 0.5,
			MinRequests:        5,
			HalfOpenMaxRequests: 3,
			OpenTimeoutMs:      10000,
		})

		// Record failures
		for i := 0; i < 3; i++ {
			cb.RecordFailure()
		}
		// Record some successes
		for i := 0; i < 2; i++ {
			cb.RecordSuccess()
		}

		// Error rate = 3/5 = 0.6 > 0.5, should open
		assert.Equal(t, CircuitStateOpen, cb.State())
	})

	t.Run("transitions to half-open after timeout", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			ErrorRateThreshold: 0.5,
			MinRequests:        5,
			HalfOpenMaxRequests: 3,
			OpenTimeoutMs:      10, // Very short timeout
		})

		// Force to open
		for i := 0; i < 10; i++ {
			cb.RecordFailure()
		}
		assert.Equal(t, CircuitStateOpen, cb.State())

		// Wait for timeout
		time.Sleep(20 * time.Millisecond)

		// Should allow request and go to half-open
		assert.True(t, cb.Allow())
		assert.Equal(t, CircuitStateHalfOpen, cb.State())
	})

	t.Run("closes after successes in half-open", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			ErrorRateThreshold: 0.5,
			MinRequests:        5,
			HalfOpenMaxRequests: 3,
			OpenTimeoutMs:      10000,
		})

		// Force to half-open
		cb.state = CircuitStateHalfOpen
		cb.lastStateChange = time.Now()

		// Record successes
		for i := 0; i < 3; i++ {
			cb.Allow() // Consume half-open slot
			cb.RecordSuccess()
		}

		assert.Equal(t, CircuitStateClosed, cb.State())
	})

	logger.Info("circuit breaker transitions test passed", zap.Any("states", map[string]CircuitState{
		"closed":     CircuitStateClosed,
		"open":       CircuitStateOpen,
		"half-open":  CircuitStateHalfOpen,
	}))
}

// TestRetryPolicy tests retry logic with exponential backoff and jitter.
func TestRetryPolicy(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:    3,
		InitialDelayMs: 100,
		MaxDelayMs:     1000,
		Multiplier:     2.0,
		JitterPercent:  20,
	}

	policy := NewRetryPolicy(config)

	t.Run("should not retry after max attempts", func(t *testing.T) {
		err := &RetryableError{Err: errors.New("test"), Retryable: true}
		assert.False(t, policy.ShouldRetry(3, err))
		assert.False(t, policy.ShouldRetry(4, err))
	})

	t.Run("should retry on retryable errors", func(t *testing.T) {
		err := &RetryableError{Err: errors.New("test"), Retryable: true}
		assert.True(t, policy.ShouldRetry(0, err))
		assert.True(t, policy.ShouldRetry(1, err))
		assert.True(t, policy.ShouldRetry(2, err))
	})

	t.Run("should not retry on non-retryable errors", func(t *testing.T) {
		err := &RetryableError{Err: errors.New("test"), Retryable: false}
		assert.False(t, policy.ShouldRetry(0, err))
	})

	t.Run("exponential backoff delay", func(t *testing.T) {
		delay0 := policy.NextDelay(0)
		delay1 := policy.NextDelay(1)
		delay2 := policy.NextDelay(2)

		// No delay on first attempt
		assert.Equal(t, time.Duration(0), delay0)

		// Subsequent delays should increase
		assert.Greater(t, delay1, delay0)
		assert.GreaterOrEqual(t, delay2, delay1)

		// Delay should be within bounds (with jitter)
		assert.LessOrEqual(t, delay1.Milliseconds(), int64(200)) // 100ms * 2 * 1.2 (jitter)
		assert.LessOrEqual(t, delay2.Milliseconds(), int64(500)) // 100ms * 2^2 * 1.2
	})

	t.Run("max delay cap", func(t *testing.T) {
		delay := policy.NextDelay(10)
		assert.LessOrEqual(t, delay.Milliseconds(), int64(1000))
	})
}

// TestModelTierFunctions tests model tier upgrade/downgrade functions.
func TestModelTierFunctions(t *testing.T) {
	t.Run("get model tier", func(t *testing.T) {
		assert.Equal(t, TierLow, GetModelTier(ModelClaudeHaiku))
		assert.Equal(t, TierLow, GetModelTier(ModelGLM5Air))
		assert.Equal(t, TierMedium, GetModelTier(ModelClaudeSonnet))
		assert.Equal(t, TierMedium, GetModelTier(ModelGLM5))
	})

	t.Run("upgrade model", func(t *testing.T) {
		assert.Equal(t, ModelClaudeSonnet, UpgradeModel(ModelClaudeHaiku))
		assert.Equal(t, ModelGLM5, UpgradeModel(ModelGLM5Air))
		assert.Equal(t, ModelClaudeSonnet, UpgradeModel(ModelClaudeSonnet)) // Stays at high tier
		assert.Equal(t, ModelGLM5, UpgradeModel(ModelGLM5))
	})

	t.Run("downgrade model", func(t *testing.T) {
		assert.Equal(t, ModelClaudeHaiku, DowngradeModel(ModelClaudeSonnet))
		assert.Equal(t, ModelGLM5Air, DowngradeModel(ModelGLM5))
		assert.Equal(t, ModelClaudeHaiku, DowngradeModel(ModelClaudeHaiku)) // Stays at low tier
	})
}

// TestModelSelector tests the model selector.
func TestModelSelector(t *testing.T) {
	logger := zap.NewNop()

	t.Run("select by task type", func(t *testing.T) {
		// Use minimal config to avoid interference from new models
		config := &Config{
			PrimaryProvider: ModelClaudeSonnet,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeSonnet: {
					TaskPreferences: []TaskType{TaskTypeAnalysis, TaskTypeCoding},
				},
				ModelGLM5: {
					TaskPreferences: []TaskType{TaskTypeReview, TaskTypeTesting},
				},
				ModelClaudeHaiku: {
					TaskPreferences: []TaskType{TaskTypeSimple, TaskTypeSummarize},
				},
			},
		}
		selector := NewModelSelector(config, logger)

		// Analysis should go to Claude Sonnet (preferred)
		model := selector.SelectModel(TaskTypeAnalysis, RoutingByTaskType)
		assert.Equal(t, ModelClaudeSonnet, model)

		// Review should go to GLM (preferred)
		model = selector.SelectModel(TaskTypeReview, RoutingByTaskType)
		assert.Equal(t, ModelGLM5, model)
	})

	t.Run("select by cost", func(t *testing.T) {
		config := &Config{
			PrimaryProvider: ModelClaudeHaiku,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeHaiku: {CostPer1KTokens: 0.003},
				ModelGLM5Air:     {CostPer1KTokens: 0.002},
			},
		}
		selector := NewModelSelector(config, logger)
		model := selector.SelectModel(TaskTypeSimple, RoutingByCost)
		// Should select the cheapest available model
		assert.Contains(t, []ModelName{ModelClaudeHaiku, ModelGLM5Air}, model)
	})

	t.Run("select by latency", func(t *testing.T) {
		config := &Config{
			PrimaryProvider: ModelClaudeHaiku,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeHaiku: {AvgLatencyMs: 500},
				ModelGLM5Air:     {AvgLatencyMs: 300},
			},
		}
		selector := NewModelSelector(config, logger)
		model := selector.SelectModel(TaskTypeSimple, RoutingByLatency)
		// Should select the fastest available model
		assert.Contains(t, []ModelName{ModelClaudeHaiku, ModelGLM5Air}, model)
	})
}

// TestRouterComplete tests the router Complete method.
func TestRouterComplete(t *testing.T) {
	logger := zap.NewNop()

	t.Run("routes to preferred provider", func(t *testing.T) {
		config := &Config{
			PrimaryProvider: ModelClaudeSonnet,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeSonnet: {TaskPreferences: []TaskType{TaskTypeAnalysis}},
			},
			CircuitBreakerConfig: CircuitBreakerConfig{
				ErrorRateThreshold: 0.5,
				MinRequests:         5,
				HalfOpenMaxRequests: 3,
				OpenTimeoutMs:       30000,
			},
			RetryConfig: RetryConfig{
				MaxAttempts: 3,
			},
		}
		router := NewRouter(config, logger)

		mockProvider := &mockProvider{name: ModelClaudeSonnet}
		router.RegisterProvider(mockProvider)

		resp, err := router.Complete(context.Background(), &LLMRequest{
			Model:   ModelClaudeSonnet, // Explicitly set model
			TaskType: TaskTypeAnalysis,
			Prompt:   "test prompt",
		})

		require.NoError(t, err)
		assert.Equal(t, "mock response", resp.Content)
		assert.Equal(t, ModelClaudeSonnet, resp.Model)
	})

	t.Run("falls back to alternative when circuit open", func(t *testing.T) {
		config := &Config{
			PrimaryProvider: ModelClaudeSonnet,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeSonnet: {TaskPreferences: []TaskType{TaskTypeAnalysis}},
				ModelClaudeHaiku:  {TaskPreferences: []TaskType{TaskTypeSimple}},
			},
			CircuitBreakerConfig: CircuitBreakerConfig{
				ErrorRateThreshold: 0.5,
				MinRequests:         5,
				HalfOpenMaxRequests: 3,
				OpenTimeoutMs:       30000,
			},
			RetryConfig: RetryConfig{
				MaxAttempts: 3,
			},
		}
		router := NewRouter(config, logger)

		primaryProvider := &mockProvider{name: ModelClaudeSonnet}
		fallbackProvider := &mockProvider{name: ModelClaudeHaiku}
		router.RegisterProvider(primaryProvider)
		router.RegisterProvider(fallbackProvider)

		// Force circuit open for primary provider
		router.selector.circuits[ModelClaudeSonnet].SetStateForTest(CircuitStateOpen)

		resp, err := router.Complete(context.Background(), &LLMRequest{
			TaskType: TaskTypeAnalysis,
			Prompt:   "test prompt",
		})

		// Should succeed via fallback since primary circuit is open
		require.NoError(t, err)
		assert.Equal(t, "mock response", resp.Content)
		assert.Equal(t, ModelClaudeHaiku, resp.Model)
	})
}

// TestAdaptiveUpgradeDowngrade tests adaptive upgrade/downgrade behavior.
func TestAdaptiveUpgradeDowngrade(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultConfig()
	config.EnableAdaptiveRouting = true

	t.Run("consecutive failures trigger upgrade consideration", func(t *testing.T) {
		router := NewRouter(config, logger)

		// Register both models
		claudeProvider := &mockProvider{name: ModelClaudeSonnet}
		haikuProvider := &mockProvider{name: ModelClaudeHaiku}
		router.RegisterProvider(claudeProvider)
		router.RegisterProvider(haikuProvider)

		// Simulate failures
		router.mu.Lock()
		router.consecutiveFails[ModelClaudeHaiku] = 5
		router.mu.Unlock()

		// Should consider upgrading
		upgraded := router.tryUpgrade(ModelClaudeHaiku)
		assert.Equal(t, ModelClaudeSonnet, upgraded)
	})

	t.Run("sustained success allows downgrade", func(t *testing.T) {
		router := NewRouter(config, logger)

		claudeProvider := &mockProvider{name: ModelClaudeSonnet}
		haikuProvider := &mockProvider{name: ModelClaudeHaiku}
		router.RegisterProvider(claudeProvider)
		router.RegisterProvider(haikuProvider)

		// Simulate sustained success
		router.mu.Lock()
		router.consecutiveSuccesses[ModelClaudeSonnet] = 15 // > 10 threshold
		router.mu.Unlock()

		// Should consider downgrading
		downgraded := router.tryDowngrade(ModelClaudeSonnet)
		assert.Equal(t, ModelClaudeHaiku, downgraded)
	})
}

// TestFindAlternativeModel tests finding alternative models.
func TestFindAlternativeModel(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultConfig()
	router := NewRouter(config, logger)

	provider1 := &mockProvider{name: ModelClaudeSonnet}
	provider2 := &mockProvider{name: ModelGLM5}
	provider3 := &mockProvider{name: ModelClaudeHaiku}
	router.RegisterProvider(provider1)
	router.RegisterProvider(provider2)
	router.RegisterProvider(provider3)

	// Open circuit for ModelClaudeSonnet
	for i := 0; i < 100; i++ {
		router.selector.circuits[ModelClaudeSonnet].RecordFailure()
	}

	// Should find an alternative
	alt := router.findAlternativeModel(ModelClaudeSonnet)
	assert.NotEqual(t, ModelClaudeSonnet, alt)
	assert.NotEmpty(t, alt)
}

// TestRetryableError tests the RetryableError type.
func TestRetryableError(t *testing.T) {
	t.Run("wrap error", func(t *testing.T) {
		err := &RetryableError{
			Err:       errors.New("original"),
			Retryable: true,
		}
		assert.True(t, IsRetryable(err))
		assert.Equal(t, "original", err.Error())
		assert.Equal(t, "original", err.Unwrap().Error())
	})

	t.Run("is non-retryable", func(t *testing.T) {
		err := &RetryableError{
			Err:       errors.New("fatal"),
			Retryable: false,
		}
		assert.False(t, IsRetryable(err))
		assert.True(t, IsNonRetryable(err))
	})
}

// TestParseModelName tests model name parsing.
func TestParseModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected ModelName
	}{
		{"claude-sonnet-4", ModelClaudeSonnet},
		{"claude-haiku-3", ModelClaudeHaiku},
		{"glm-5.1-air", ModelGLM5Air},
		{"glm-5.1", ModelGLM5},
		{"unknown", ModelClaudeSonnet}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseModelName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTruncatePrompt tests prompt truncation.
func TestTruncatePrompt(t *testing.T) {
	t.Run("truncates long prompt", func(t *testing.T) {
		longPrompt := "This is a very long prompt that should be truncated because it exceeds the maximum token limit."
		truncated := TruncatePrompt(longPrompt, 10)
		assert.LessOrEqual(t, len(truncated), 50) // Rough char limit
		assert.True(t, len(truncated) < len(longPrompt))
	})

	t.Run("keeps short prompt", func(t *testing.T) {
		shortPrompt := "Short prompt"
		truncated := TruncatePrompt(shortPrompt, 100)
		assert.Equal(t, shortPrompt, truncated)
	})
}

// TestProviderStats tests provider statistics.
func TestProviderStats(t *testing.T) {
	provider := &mockProvider{name: ModelClaudeSonnet}

	stats := provider.Stats()
	assert.Equal(t, int64(0), stats.TotalRequests)
	assert.Equal(t, int64(0), stats.SuccessRequests)
	assert.Equal(t, int64(0), stats.FailedRequests)

	// Do some requests
	provider.Complete(context.Background(), &LLMRequest{Prompt: "test"})

	stats = provider.Stats()
	assert.Equal(t, int64(1), stats.TotalRequests)
	assert.Equal(t, int64(1), stats.SuccessRequests)
}

// TestDoWithRetry tests the retry wrapper function.
func TestDoWithRetry(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:    3,
		InitialDelayMs: 10,
		MaxDelayMs:     100,
		Multiplier:     2.0,
		JitterPercent:  0,
	}
	policy := NewRetryPolicy(config)

	t.Run("succeeds on first try", func(t *testing.T) {
		calls := 0
		err := DoWithRetry(context.Background(), policy, func() error {
			calls++
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 1, calls)
	})

	t.Run("retries on retryable error then succeeds", func(t *testing.T) {
		calls := 0
		err := DoWithRetry(context.Background(), policy, func() error {
			calls++
			if calls < 2 {
				return &RetryableError{Err: errors.New("retryable"), Retryable: true}
			}
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 2, calls)
	})

	t.Run("fails after max retries", func(t *testing.T) {
		calls := 0
		err := DoWithRetry(context.Background(), policy, func() error {
			calls++
			return &RetryableError{Err: errors.New("retryable"), Retryable: true}
		})
		assert.Error(t, err)
		assert.Equal(t, 3, calls) // MaxAttempts
	})

	t.Run("fails immediately on non-retryable error", func(t *testing.T) {
		calls := 0
		err := DoWithRetry(context.Background(), policy, func() error {
			calls++
			return &RetryableError{Err: errors.New("non-retryable"), Retryable: false}
		})
		assert.Error(t, err)
		assert.Equal(t, 1, calls) // No retries
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		err := DoWithRetry(ctx, policy, func() error {
			calls++
			cancel()
			return &RetryableError{Err: errors.New("retryable"), Retryable: true}
		})
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

// TestRoundRobinRouting tests round-robin load balancing.
func TestRoundRobinRouting(t *testing.T) {
	logger := zap.NewNop()

	t.Run("round-robin selects different models", func(t *testing.T) {
		config := &Config{
			PrimaryProvider: ModelClaudeSonnet,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeSonnet: {TaskPreferences: []TaskType{TaskTypeSimple}},
				ModelClaudeHaiku:  {TaskPreferences: []TaskType{TaskTypeSimple}},
				ModelGLM5:        {TaskPreferences: []TaskType{TaskTypeSimple}},
			},
			CircuitBreakerConfig: CircuitBreakerConfig{
				ErrorRateThreshold: 0.5,
				MinRequests:         5,
				HalfOpenMaxRequests: 3,
				OpenTimeoutMs:       30000,
			},
		}
		selector := NewModelSelector(config, logger)

		// First selection
		first := selector.SelectModel(TaskTypeSimple, RoutingRoundRobin)
		// Subsequent selections should eventually cycle through
		seen := map[ModelName]bool{first: true}
		for i := 0; i < 10; i++ {
			seen[selector.SelectModel(TaskTypeSimple, RoutingRoundRobin)] = true
		}

		// With round-robin and 3 models, we should see multiple models
		assert.GreaterOrEqual(t, len(seen), 2, "should see multiple models in round-robin")
	})

	t.Run("round-robin skips open circuits", func(t *testing.T) {
		config := &Config{
			PrimaryProvider: ModelClaudeSonnet,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeSonnet: {TaskPreferences: []TaskType{TaskTypeSimple}},
				ModelClaudeHaiku:  {TaskPreferences: []TaskType{TaskTypeSimple}},
			},
			CircuitBreakerConfig: CircuitBreakerConfig{
				ErrorRateThreshold: 0.5,
				MinRequests:         5,
				HalfOpenMaxRequests: 3,
				OpenTimeoutMs:       30000,
			},
		}
		selector := NewModelSelector(config, logger)

		// Force one circuit to open
		selector.circuits[ModelClaudeSonnet].SetStateForTest(CircuitStateOpen)

		// All selections should go to the available one
		for i := 0; i < 5; i++ {
			selected := selector.SelectModel(TaskTypeSimple, RoutingRoundRobin)
			assert.Equal(t, ModelClaudeHaiku, selected, "should skip open circuit")
		}
	})
}

// TestFallbackRouting tests automatic fallback when primary fails.
func TestFallbackRouting(t *testing.T) {
	logger := zap.NewNop()

	t.Run("falls back to alternative when primary fails", func(t *testing.T) {
		config := &Config{
			PrimaryProvider: ModelClaudeSonnet,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeSonnet: {TaskPreferences: []TaskType{TaskTypeAnalysis}},
				ModelGLM5:        {TaskPreferences: []TaskType{TaskTypeAnalysis}},
			},
			FallbackOrder: []ModelName{ModelClaudeSonnet, ModelGLM5},
			RetryConfig: RetryConfig{
				MaxAttempts: 3,
			},
		}
		router := NewRouter(config, logger)

		failingProvider := &mockProvider{name: ModelClaudeSonnet, shouldFail: true, consecutiveFails: 5}
		workingProvider := &mockProvider{name: ModelGLM5}
		router.RegisterProvider(failingProvider)
		router.RegisterProvider(workingProvider)

		resp, err := router.Complete(context.Background(), &LLMRequest{
			TaskType: TaskTypeAnalysis,
			Prompt:   "test prompt",
		})

		require.NoError(t, err)
		assert.Equal(t, "mock response", resp.Content)
		assert.Equal(t, ModelGLM5, resp.Model)
	})

	t.Run("returns error when all providers fail", func(t *testing.T) {
		config := &Config{
			PrimaryProvider: ModelClaudeSonnet,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeSonnet: {TaskPreferences: []TaskType{TaskTypeAnalysis}},
				ModelGLM5:        {TaskPreferences: []TaskType{TaskTypeAnalysis}},
			},
			FallbackOrder: []ModelName{ModelClaudeSonnet, ModelGLM5},
			RetryConfig: RetryConfig{
				MaxAttempts: 3,
			},
		}
		router := NewRouter(config, logger)

		failingProvider1 := &mockProvider{name: ModelClaudeSonnet, shouldFail: true, consecutiveFails: 10}
		failingProvider2 := &mockProvider{name: ModelGLM5, shouldFail: true, consecutiveFails: 10}
		router.RegisterProvider(failingProvider1)
		router.RegisterProvider(failingProvider2)

		_, err := router.Complete(context.Background(), &LLMRequest{
			TaskType: TaskTypeAnalysis,
			Prompt:   "test prompt",
		})

		assert.Error(t, err)
	})
}

// TestTryFallback tests the tryFallback method.
func TestTryFallback(t *testing.T) {
	logger := zap.NewNop()

	t.Run("tries fallback providers in order", func(t *testing.T) {
		config := &Config{
			PrimaryProvider: ModelClaudeSonnet,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeSonnet: {TaskPreferences: []TaskType{TaskTypeAnalysis}},
				ModelGLM5:        {TaskPreferences: []TaskType{TaskTypeAnalysis}},
			},
			FallbackOrder: []ModelName{ModelClaudeSonnet, ModelGLM5},
		}
		router := NewRouter(config, logger)

		failingProvider := &mockProvider{name: ModelClaudeSonnet, shouldFail: true, consecutiveFails: 10}
		workingProvider := &mockProvider{name: ModelGLM5}
		router.RegisterProvider(failingProvider)
		router.RegisterProvider(workingProvider)

		resp, err := router.tryFallback(context.Background(), &LLMRequest{Prompt: "test"}, ModelClaudeSonnet)

		require.NoError(t, err)
		assert.Equal(t, ModelGLM5, resp.Model)
	})

	t.Run("returns nil when no fallback available", func(t *testing.T) {
		config := &Config{
			PrimaryProvider: ModelClaudeSonnet,
			ProviderConfigs: map[ModelName]ProviderConfig{
				ModelClaudeSonnet: {TaskPreferences: []TaskType{TaskTypeAnalysis}},
			},
			FallbackOrder: []ModelName{ModelClaudeSonnet},
		}
		router := NewRouter(config, logger)

		// Only register the failed model
		failingProvider := &mockProvider{name: ModelClaudeSonnet, shouldFail: true, consecutiveFails: 10}
		router.RegisterProvider(failingProvider)

		resp, err := router.tryFallback(context.Background(), &LLMRequest{Prompt: "test"}, ModelClaudeSonnet)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})
}

// TestOpenAICompatibleProvider tests the OpenAI-compatible provider.
func TestOpenAICompatibleProvider(t *testing.T) {
	logger := zap.NewNop()

	t.Run("creates provider with single endpoint", func(t *testing.T) {
		provider := NewOpenAICompatibleProvider(
			ModelDeepseekChat,
			"test-key",
			"https://api.deepseek.com",
			10*time.Second,
			logger,
		)
		assert.Equal(t, ModelDeepseekChat, provider.Name())
		assert.Equal(t, "https://api.deepseek.com", provider.endpoint)
	})

	t.Run("creates provider with multiple endpoints", func(t *testing.T) {
		endpoints := []string{
			"https://api.deepseek.com",
			"https://api.deepseek.com/backups",
		}
		provider := NewOpenAICompatibleProviderWithEndpoints(
			ModelDeepseekChat,
			"test-key",
			endpoints,
			10*time.Second,
			logger,
		)
		assert.Equal(t, ModelDeepseekChat, provider.Name())
		assert.Equal(t, 2, len(provider.endpoints))
	})

	t.Run("round-robin cycles through endpoints", func(t *testing.T) {
		endpoints := []string{"ep1", "ep2", "ep3"}
		provider := NewOpenAICompatibleProviderWithEndpoints(
			ModelDeepseekChat,
			"test-key",
			endpoints,
			10*time.Second,
			logger,
		)

		seen := make(map[string]bool)
		for i := 0; i < 6; i++ {
			seen[provider.getNextEndpoint()] = true
		}

		// All endpoints should be used
		assert.True(t, seen["ep1"] || seen["ep2"] || seen["ep3"])
	})

	t.Run("stats are tracked", func(t *testing.T) {
		provider := NewOpenAICompatibleProvider(
			ModelDeepseekChat,
			"test-key",
			"https://api.deepseek.com",
			10*time.Second,
			logger,
		)

		stats := provider.Stats()
		assert.Equal(t, int64(0), stats.TotalRequests)
		assert.Equal(t, int64(0), stats.SuccessRequests)
		assert.Equal(t, int64(0), stats.FailedRequests)
	})
}

// TestModelNameConstants tests the new model name constants.
func TestModelNameConstants(t *testing.T) {
	t.Run("Deepseek models are defined", func(t *testing.T) {
		assert.Equal(t, ModelName("deepseek-chat"), ModelDeepseekChat)
		assert.Equal(t, ModelName("deepseek-coder"), ModelDeepseekCoder)
	})

	t.Run("Qwen models are defined", func(t *testing.T) {
		assert.Equal(t, ModelName("qwen-turbo"), ModelQwenTurbo)
		assert.Equal(t, ModelName("qwen-max"), ModelQwenMax)
		assert.Equal(t, ModelName("qwen-plus"), ModelQwenPlus)
		assert.Equal(t, ModelName("qwen2.5-coder"), ModelQwen2Coder)
	})

	t.Run("Yi models are defined", func(t *testing.T) {
		assert.Equal(t, ModelName("yi-turbo"), ModelYiTurbo)
		assert.Equal(t, ModelName("yi-lightning"), ModelYiLightning)
	})
}

// TestConfigWithFallbackOrder tests config with fallback order.
func TestConfigWithFallbackOrder(t *testing.T) {
	t.Run("default config has fallback order", func(t *testing.T) {
		config := DefaultConfig()
		assert.NotEmpty(t, config.FallbackOrder)
		assert.Contains(t, config.FallbackOrder, ModelClaudeSonnet)
		assert.Contains(t, config.FallbackOrder, ModelDeepseekChat)
	})

	t.Run("get provider config for new models", func(t *testing.T) {
		config := DefaultConfig()

		cfg := config.GetProviderConfig(ModelDeepseekChat)
		assert.NotEmpty(t, cfg.TaskPreferences)
		assert.Equal(t, TaskTypeCoding, cfg.TaskPreferences[0])

		cfg = config.GetProviderConfig(ModelQwenTurbo)
		assert.NotEmpty(t, cfg.TaskPreferences)
	})
}
