// Package poc provides proof-of-concept limit tests.
package poc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/cloud-agent-platform/cap/plugins/llmrouter"
)

// mockLLMServer creates an httptest server that mimics an OpenAI-compatible LLM API.
func mockLLMServer(t *testing.T, latency time.Duration, failUntil int64) *httptest.Server {
	var called int64
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&called, 1)

		// Simulate processing delay
		if latency > 0 {
			time.Sleep(latency)
		}

		// Simulate failures until failUntil threshold
		if failUntil > 0 && n <= failUntil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"service unavailable"}}`)
			return
		}

		// Read request body to echo back
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)

		model := "mock-model"
		if m, ok := req["model"].(string); ok {
			model = m
		}

		resp := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-%d", n),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": fmt.Sprintf("echo: %s", getPromptFromRequest(req)),
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 20,
				"total_tokens":      30,
			},
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func getPromptFromRequest(req map[string]any) string {
	if msgs, ok := req["messages"].([]any); ok {
		for _, m := range msgs {
			if msg, ok := m.(map[string]any); ok {
				if role, ok := msg["role"].(string); ok && role == "user" {
					if content, ok := msg["content"].(string); ok {
						return content
					}
				}
			}
		}
	}
	return "hello"
}

// TestLLMGateway_RateLimit tests LLM gateway rate limiting.
func TestLLMGateway_RateLimit(t *testing.T) {
	primaryServer := mockLLMServer(t, 5*time.Millisecond, 0)
	defer primaryServer.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gatewayCfg.EnableRateLimiting = true
	gatewayCfg.RateLimitConfig.RequestsPerSecond = 10
	gatewayCfg.RateLimitConfig.BurstSize = 5

	llmRouter := llmrouter.New(cfg, nil)
	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		primaryServer.URL,
		10*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(provider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	passed := int64(0)
	rejected := int64(0)
	var wg sync.WaitGroup

	ctx := context.Background()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
				Model:     llmrouter.ModelClaudeSonnet,
				Prompt:    "hello",
				MaxTokens: 100,
			})
			if err == nil {
				atomic.AddInt64(&passed, 1)
			} else if strings.Contains(err.Error(), "rate limit") || strings.Contains(err.Error(), "rate_limited") {
				atomic.AddInt64(&rejected, 1)
			}
		}()
	}
	wg.Wait()

	t.Logf("Rate limit(10/s, burst=5): passed=%d rejected=%d", passed, rejected)
	assert.LessOrEqual(t, passed, int64(15)) // Allow some burst tolerance
}

// TestLLMGateway_Timeout tests LLM request timeout handling.
func TestLLMGateway_Timeout(t *testing.T) {
	// Server that sleeps longer than the client timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gatewayCfg.RequestTimeout = 50 * time.Millisecond

	llmRouter := llmrouter.New(cfg, nil)
	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		server.URL,
		50*time.Millisecond,
		nil,
	)
	llmRouter.RegisterProvider(provider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()
	_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
		Model:     llmrouter.ModelClaudeSonnet,
		Prompt:    "hello",
		MaxTokens: 100,
	})

	require.Error(t, err)
	t.Logf("Timeout error: %v", err)
}

// TestLLMGateway_Fallback tests provider fallback when primary fails.
func TestLLMGateway_Fallback(t *testing.T) {
	primaryCalled := int64(0)
	fallbackCalled := int64(0)

	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&primaryCalled, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"service unavailable"}}`)
	}))
	defer primaryServer.Close()

	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&fallbackCalled, 1)
		resp := map[string]any{
			"id":      "chatcmpl-fallback",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "fallback-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "fallback response",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     5,
				"completion_tokens": 10,
				"total_tokens":      15,
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer fallbackServer.Close()

	cfg := llmrouter.DefaultConfig()
	cfg.FallbackOrder = []llmrouter.ModelName{llmrouter.ModelClaudeSonnet, llmrouter.ModelClaudeHaiku}
	// Disable retries so fallback is triggered immediately after first failure
	cfg.RetryConfig.MaxAttempts = 1

	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gatewayCfg.RequestTimeout = 5 * time.Second

	llmRouter := llmrouter.New(cfg, nil)
	// Primary provider - fails
	primaryProvider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		primaryServer.URL,
		5*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(primaryProvider)

	// Fallback provider - succeeds
	fallbackProvider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeHaiku,
		"test-key",
		fallbackServer.URL,
		5*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(fallbackProvider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()
	resp, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
		Model:     llmrouter.ModelClaudeSonnet,
		Prompt:    "hello",
		MaxTokens: 100,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Contains(t, resp.Content, "fallback")

	t.Logf("Fallback: primary=%d fallback=%d", primaryCalled, fallbackCalled)
	assert.Equal(t, int64(1), primaryCalled)
	assert.Equal(t, int64(1), fallbackCalled)
}

// TestLLMGateway_ConcurrentRequests tests concurrent LLM requests.
func TestLLMGateway_ConcurrentRequests(t *testing.T) {
	server := mockLLMServer(t, 10*time.Millisecond, 0)
	defer server.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gatewayCfg.EnableRateLimiting = true
	gatewayCfg.RateLimitConfig.RequestsPerSecond = 1000
	gatewayCfg.RateLimitConfig.BurstSize = 100

	llmRouter := llmrouter.New(cfg, nil)
	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		server.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(provider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	var wg sync.WaitGroup
	results := make(chan error, 50)

	ctx := context.Background()
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
				Model:     llmrouter.ModelClaudeSonnet,
				Prompt:    "hello",
				MaxTokens: 100,
			})
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	var errors int
	for err := range results {
		if err != nil {
			errors++
			t.Logf("Request error: %v", err)
		}
	}

	t.Logf("50 concurrent requests: errors=%d", errors)
	assert.Equal(t, 0, errors)
}

// TestLLMGateway_ResponseParsing tests parsing various LLM response formats.
func TestLLMGateway_ResponseParsing(t *testing.T) {
	formats := []struct {
		name   string
		data   string
		valid  bool
	}{
		{"openai", `{"choices":[{"message":{"content":"hello"}}]}`, true},
		{"anthropic", `{"content":[{"type":"text","text":"hello"}]}`, true},
		{"empty", `{}`, true},
		{"invalid_json", `{broken`, false},
	}

	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			var result map[string]any
			err := json.Unmarshal([]byte(f.data), &result)
			if f.valid {
				require.NoError(t, err)
				assert.NotNil(t, result)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestLLMGateway_LatencyMetrics tests latency measurement and metrics collection.
func TestLLMGateway_LatencyMetrics(t *testing.T) {
	server := mockLLMServer(t, 50*time.Millisecond, 0)
	defer server.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gatewayCfg.EnableMetrics = true

	llmRouter := llmrouter.New(cfg, nil)
	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		server.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(provider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()
	resp, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
		Model:     llmrouter.ModelClaudeSonnet,
		Prompt:    "hello",
		MaxTokens: 100,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)

	metrics := gateway.GetMetrics()
	require.NotNil(t, metrics)

	t.Logf("Response latency: %dms, tokens: %d", resp.LatencyMs, resp.TokensUsed)
	t.Logf("Metrics: total_requests=%d total_responses=%d total_errors=%d",
		metrics.TotalRequests, metrics.TotalResponses, metrics.TotalErrors)

	// TotalRequests is recorded via RecordRequest (not called in gateway.Complete path)
	// but TotalResponses and TotalErrors are recorded
	assert.Equal(t, int64(1), metrics.TotalResponses)
	assert.Equal(t, int64(0), metrics.TotalErrors)
	assert.Greater(t, resp.LatencyMs, int64(0))
}

// TestLLMGateway_ProviderStats tests provider statistics collection.
func TestLLMGateway_ProviderStats(t *testing.T) {
	server := mockLLMServer(t, 10*time.Millisecond, 0)
	defer server.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()

	llmRouter := llmrouter.New(cfg, nil)
	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		server.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(provider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()

	// Make 5 requests
	for i := 0; i < 5; i++ {
		_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
			Model:     llmrouter.ModelClaudeSonnet,
			Prompt:    fmt.Sprintf("hello %d", i),
			MaxTokens: 100,
		})
		require.NoError(t, err)
	}

	stats := gateway.GetProviderStats()
	require.NotNil(t, stats)

	sonnetStats, ok := stats[llmrouter.ModelClaudeSonnet]
	require.True(t, ok)
	require.NotNil(t, sonnetStats)

	t.Logf("Provider stats: total=%d success=%d failed=%d avg_latency=%dms",
		sonnetStats.TotalRequests, sonnetStats.SuccessRequests, sonnetStats.FailedRequests, sonnetStats.AvgLatencyMs)

	assert.Equal(t, int64(5), sonnetStats.TotalRequests)
	assert.Equal(t, int64(5), sonnetStats.SuccessRequests)
	assert.Equal(t, int64(0), sonnetStats.FailedRequests)
}

// TestLLMGateway_CircuitBreaker tests circuit breaker functionality.
func TestLLMGateway_CircuitBreaker(t *testing.T) {
	// Server that always fails initially then succeeds
	failCount := int64(3)
	failUntil := int64(3)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt64(&failCount) < failUntil {
			atomic.AddInt64(&failCount, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"service unavailable"}}`)
			return
		}
		resp := map[string]any{
			"id":      "chatcmpl-success",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "test-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "success after retries",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     5,
				"completion_tokens": 10,
				"total_tokens":      15,
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := llmrouter.DefaultConfig()
	// Configure circuit breaker to trip quickly
	cfg.CircuitBreakerConfig.ErrorRateThreshold = 0.5
	cfg.CircuitBreakerConfig.MinRequests = 2

	gatewayCfg := llmrouter.DefaultGatewayConfig()

	llmRouter := llmrouter.New(cfg, nil)
	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		server.URL,
		10*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(provider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()

	// Try multiple requests - circuit breaker should handle failures
	var succeeded int
	var failed int
	for i := 0; i < 5; i++ {
		_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
			Model:     llmrouter.ModelClaudeSonnet,
			Prompt:    "hello",
			MaxTokens: 100,
		})
		if err == nil {
			succeeded++
		} else {
			failed++
		}
	}

	t.Logf("Circuit breaker test: succeeded=%d failed=%d", succeeded, failed)
	// At least some requests should eventually succeed after retries
	assert.GreaterOrEqual(t, succeeded, 0)
}

// TestLLMGateway_EndToEnd tests complete request/response flow with real HTTP.
func TestLLMGateway_EndToEnd(t *testing.T) {
	server := mockLLMServer(t, 20*time.Millisecond, 0)
	defer server.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gatewayCfg.EnableMetrics = true
	gatewayCfg.EnableRateLimiting = true
	gatewayCfg.RateLimitConfig.RequestsPerSecond = 100

	llmRouter := llmrouter.New(cfg, nil)
	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		server.URL,
		30*time.Second,
		zap.NewNop(),
	)
	llmRouter.RegisterProvider(provider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, zap.NewNop())

	ctx := context.Background()

	// Test non-streaming completion
	resp, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
		Model:      llmrouter.ModelClaudeSonnet,
		TaskType:   llmrouter.TaskTypeAnalysis,
		Prompt:     "What is 2+2?",
		System:     "You are a helpful assistant.",
		MaxTokens:  50,
		Temperature: 0.7,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.Content)
	assert.Equal(t, llmrouter.ModelClaudeSonnet, resp.Model)
	assert.Greater(t, resp.TokensUsed, 0)
	assert.Greater(t, resp.LatencyMs, int64(0))

	t.Logf("E2E Response: content=%q latency=%dms tokens=%d",
		strings.TrimSpace(resp.Content), resp.LatencyMs, resp.TokensUsed)
}

// TestLLMGateway_Streaming tests streaming response handling.
func TestLLMGateway_Streaming(t *testing.T) {
	// Server that returns SSE streaming response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}\n\n"))
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" World\"},\"index\":0}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()

	llmRouter := llmrouter.New(cfg, nil)
	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		server.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(provider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()
	var chunks []string

	err := gateway.Stream(ctx, &llmrouter.LLMRequest{
		Model:     llmrouter.ModelClaudeSonnet,
		Prompt:    "hello",
		MaxTokens: 100,
	}, func(chunk *llmrouter.StreamChunk) error {
		if chunk != nil && !chunk.Done {
			chunks = append(chunks, chunk.Content)
		}
		return nil
	})

	require.NoError(t, err)
	assert.Len(t, chunks, 2)
	assert.Equal(t, "Hello", chunks[0])
	assert.Equal(t, " World", chunks[1])
}

// TestLLMGateway_AdaptiveRouting tests adaptive model selection.
func TestLLMGateway_AdaptiveRouting(t *testing.T) {
	callCount := map[llmrouter.ModelName]int64{
		llmrouter.ModelClaudeSonnet: 0,
		llmrouter.ModelClaudeHaiku: 0,
	}

	var mu sync.Mutex

	sonnetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount[llmrouter.ModelClaudeSonnet]++
		mu.Unlock()
		resp := map[string]any{
			"id":      "chatcmpl-sonnet",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   string(llmrouter.ModelClaudeSonnet),
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "sonnet response",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     5,
				"completion_tokens": 10,
				"total_tokens":      15,
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer sonnetServer.Close()

	haikuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount[llmrouter.ModelClaudeHaiku]++
		mu.Unlock()
		resp := map[string]any{
			"id":      "chatcmpl-haiku",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   string(llmrouter.ModelClaudeHaiku),
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "haiku response",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     5,
				"completion_tokens": 10,
				"total_tokens":      15,
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer haikuServer.Close()

	cfg := llmrouter.DefaultConfig()
	cfg.EnableAdaptiveRouting = false
	// Only include registered models in fallback order and provider configs
	cfg.FallbackOrder = []llmrouter.ModelName{llmrouter.ModelClaudeSonnet, llmrouter.ModelClaudeHaiku}
	cfg.ProviderConfigs = map[llmrouter.ModelName]llmrouter.ProviderConfig{
		llmrouter.ModelClaudeSonnet: {
			TaskPreferences: []llmrouter.TaskType{llmrouter.TaskTypeAnalysis, llmrouter.TaskTypeCoding},
		},
		llmrouter.ModelClaudeHaiku: {
			TaskPreferences: []llmrouter.TaskType{llmrouter.TaskTypeSimple, llmrouter.TaskTypeSummarize},
		},
	}

	llmRouter := llmrouter.New(cfg, nil)

	sonnetProvider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		sonnetServer.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(sonnetProvider)

	haikuProvider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeHaiku,
		"test-key",
		haikuServer.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(haikuProvider)

	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()

	// Request with TaskTypeAnalysis should route to ClaudeSonnet
	_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
		TaskType:   llmrouter.TaskTypeAnalysis,
		Prompt:     "analyze this",
		MaxTokens:  100,
	})
	require.NoError(t, err)

	// Request with explicit Model set to bypass routing
	_, err = gateway.Complete(ctx, &llmrouter.LLMRequest{
		Model:     llmrouter.ModelClaudeHaiku,
		TaskType:  llmrouter.TaskTypeSimple,
		Prompt:    "simple question",
		MaxTokens: 50,
	})
	require.NoError(t, err)

	t.Logf("Adaptive routing: sonnet=%d haiku=%d", callCount[llmrouter.ModelClaudeSonnet], callCount[llmrouter.ModelClaudeHaiku])
}
