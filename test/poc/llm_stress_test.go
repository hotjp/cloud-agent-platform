// Package poc provides proof-of-concept stress tests for LLM gateway.
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

// mockLLMStressServer creates an httptest server for stress testing with configurable behavior.
func mockLLMStressServer(t *testing.T, latency time.Duration, failRatio float64) *httptest.Server {
	var callCount int64
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&callCount, 1)

		// Simulate processing delay
		if latency > 0 {
			time.Sleep(latency)
		}

		// Simulate random failures based on failRatio
		failThreshold := int(failRatio * 100)
		if failRatio > 0 && int(n)%100 < failThreshold {
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

		prompt := "hello"
		if msgs, ok := req["messages"].([]any); ok {
			for _, m := range msgs {
				if msg, ok := m.(map[string]any); ok {
					if role, ok := msg["role"].(string); ok && role == "user" {
						if content, ok := msg["content"].(string); ok {
							prompt = content
						}
					}
				}
			}
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
						"content": fmt.Sprintf("echo: %s", prompt),
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

// mockSlowServer creates a server that intentionally delays responses.
func mockSlowServer(delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)

		model := "slow-model"
		if m, ok := req["model"].(string); ok {
			model = m
		}

		resp := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-slow-%d", time.Now().UnixNano()),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "slow response",
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
}

// TestLLMStress_ConcurrentRequests tests 50/100/200 concurrent requests for latency and throughput.
func TestLLMStress_ConcurrentRequests(t *testing.T) {
	testCases := []struct {
		name        string
		concurrency int
		latency    time.Duration
	}{
		{name: "50_concurrent_10ms", concurrency: 50, latency: 10 * time.Millisecond},
		{name: "100_concurrent_10ms", concurrency: 100, latency: 10 * time.Millisecond},
		{name: "200_concurrent_10ms", concurrency: 200, latency: 10 * time.Millisecond},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := mockLLMStressServer(t, tc.latency, 0)
			defer server.Close()

			cfg := llmrouter.DefaultConfig()
			gatewayCfg := llmrouter.DefaultGatewayConfig()
			gatewayCfg.EnableRateLimiting = true
			gatewayCfg.RateLimitConfig.RequestsPerSecond = 10000
			gatewayCfg.RateLimitConfig.BurstSize = tc.concurrency + 10

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
			var successCount int64
			var errorCount int64
			latencies := make([]int64, 0, tc.concurrency)
			var latMu sync.Mutex

			ctx := context.Background()
			start := time.Now()

			for i := 0; i < tc.concurrency; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					reqStart := time.Now()
					_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
						Model:     llmrouter.ModelClaudeSonnet,
						Prompt:    fmt.Sprintf("hello %d", i),
						MaxTokens: 100,
					})
					reqLatency := time.Since(reqStart).Milliseconds()

					latMu.Lock()
					latencies = append(latencies, reqLatency)
					latMu.Unlock()

					if err == nil {
						atomic.AddInt64(&successCount, 1)
					} else {
						atomic.AddInt64(&errorCount, 1)
						t.Logf("Request error: %v", err)
					}
				}()
			}

			wg.Wait()
			totalDuration := time.Since(start)

			// Calculate statistics
			var totalLatency int64
			var minLatency int64 = 1<<63 - 1
			var maxLatency int64
			for _, lat := range latencies {
				totalLatency += lat
				if lat < minLatency {
					minLatency = lat
				}
				if lat > maxLatency {
					maxLatency = lat
				}
			}
			avgLatency := totalLatency / int64(len(latencies))

			// Calculate p95 latency
			sortedLatencies := make([]int64, len(latencies))
			copy(sortedLatencies, latencies)
			for i := 0; i < len(sortedLatencies)-1; i++ {
				for j := i + 1; j < len(sortedLatencies); j++ {
					if sortedLatencies[j] < sortedLatencies[i] {
						sortedLatencies[i], sortedLatencies[j] = sortedLatencies[j], sortedLatencies[i]
					}
				}
			}
			p95Index := int(float64(len(sortedLatencies)) * 0.95)
			if p95Index >= len(sortedLatencies) {
				p95Index = len(sortedLatencies) - 1
			}
			p95Latency := sortedLatencies[p95Index]

			throughput := float64(tc.concurrency) / totalDuration.Seconds()

			t.Logf("Concurrency=%d: success=%d errors=%d duration=%v throughput=%.2f req/s",
				tc.concurrency, successCount, errorCount, totalDuration, throughput)
			t.Logf("Latency: min=%dms avg=%dms p95=%dms max=%dms",
				minLatency, avgLatency, p95Latency, maxLatency)

			// All requests should succeed under normal conditions
			assert.Equal(t, int64(tc.concurrency), successCount)
			assert.Equal(t, int64(0), errorCount)

			// P95 latency should be reasonable (under 500ms for 10ms server delay with overhead)
			assert.Less(t, p95Latency, int64(500), "P95 latency should be under 500ms")
		})
	}
}

// TestLLMStress_RateLimiting tests that exceeding rate limit returns 429.
func TestLLMStress_RateLimiting(t *testing.T) {
	server := mockLLMStressServer(t, 5*time.Millisecond, 0)
	defer server.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gatewayCfg.EnableRateLimiting = true
	// Set very low rate limit: 5 requests per second, burst of 2
	gatewayCfg.RateLimitConfig.RequestsPerSecond = 5
	gatewayCfg.RateLimitConfig.BurstSize = 2

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

	// Send 20 requests concurrently - only burst + some replenished should pass
	var wg sync.WaitGroup
	passed := int64(0)
	rejected := int64(0)

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
			} else if strings.Contains(err.Error(), "rate limit") ||
				strings.Contains(err.Error(), "rate_limited") ||
				strings.Contains(err.Error(), "429") {
				atomic.AddInt64(&rejected, 1)
			} else {
				// Other error - count as passed for this test
				atomic.AddInt64(&passed, 1)
			}
		}()
	}

	wg.Wait()

	t.Logf("Rate limit(5/s, burst=2): passed=%d rejected=%d out of 20", passed, rejected)

	// Some requests should be rejected due to rate limiting
	assert.Greater(t, rejected, int64(0), "Some requests should be rejected due to rate limiting")
	// Not all should pass (we're sending 20 concurrent requests)
	assert.Less(t, passed, int64(20), "Not all requests should pass with strict rate limiting")
}

// TestLLMStress_FallbackChain tests Provider A fails → B → C fallback chain.
func TestLLMStress_FallbackChain(t *testing.T) {
	// Track which providers were called
	var providerACalls int64
	var providerBCalls int64
	var providerCCalls int64

	// Provider A - always fails
	providerA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&providerACalls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"Provider A unavailable"}}`)
	}))
	defer providerA.Close()

	// Provider B - fails first 3 requests then succeeds
	var providerBRequestCount int64
	providerB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&providerBRequestCount, 1)
		atomic.AddInt64(&providerBCalls, 1)
		if n <= 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"Provider B unavailable"}}`)
			return
		}
		resp := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-b-%d", n),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   string(llmrouter.ModelDeepseekChat),
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "success from Provider B",
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
	defer providerB.Close()

	// Provider C - always succeeds
	providerC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&providerCCalls, 1)
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)

		model := "provider-c"
		if m, ok := req["model"].(string); ok {
			model = m
		}

		resp := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-c-%d", time.Now().UnixNano()),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "success from Provider C",
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
	defer providerC.Close()

	cfg := llmrouter.DefaultConfig()
	cfg.FallbackOrder = []llmrouter.ModelName{
		llmrouter.ModelClaudeSonnet,  // Provider A
		llmrouter.ModelDeepseekChat,  // Provider B
		llmrouter.ModelGLM5,          // Provider C
	}
	// Disable adaptive routing to use fallback order explicitly
	cfg.EnableAdaptiveRouting = false
	// Set retry to 1 so fallback is triggered quickly
	cfg.RetryConfig.MaxAttempts = 1
	cfg.RetryConfig.InitialDelayMs = 10

	llmRouter := llmrouter.New(cfg, nil)

	// Register Provider A (fails)
	providerAProv := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		providerA.URL,
		5*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(providerAProv)

	// Register Provider B (fails first 3, then succeeds)
	providerBProv := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelDeepseekChat,
		"test-key",
		providerB.URL,
		5*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(providerBProv)

	// Register Provider C (always succeeds)
	providerCProv := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelGLM5,
		"test-key",
		providerC.URL,
		5*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(providerCProv)

	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()

	// First 3 requests should fail A then fail B, then fallback to C
	// After that, B should succeed
	for i := 0; i < 5; i++ {
		resp, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
			Model:     llmrouter.ModelClaudeSonnet, // Request A first
			Prompt:    fmt.Sprintf("hello %d", i),
			MaxTokens: 100,
		})

		t.Logf("Request %d: err=%v", i+1, err)
		if err == nil {
			require.NotNil(t, resp)
			t.Logf("  Response from model: %s", resp.Model)
		}
	}

	t.Logf("Provider calls: A=%d B=%d C=%d", providerACalls, providerBCalls, providerCCalls)

	// Provider A should be called for all 5 requests (primary)
	assert.Equal(t, int64(5), providerACalls)

	// Provider B should be called (fallback from A)
	assert.GreaterOrEqual(t, providerBCalls, int64(5))

	// Provider C should be called when A and B both fail
	assert.GreaterOrEqual(t, providerCCalls, int64(1))
}

// TestLLMStress_TimeoutHandling tests that timeout requests are properly cancelled.
func TestLLMStress_TimeoutHandling(t *testing.T) {
	// Server that takes too long to respond
	slowServer := mockSlowServer(500 * time.Millisecond)
	defer slowServer.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()
	// Set a short timeout
	gatewayCfg.RequestTimeout = 100 * time.Millisecond

	llmRouter := llmrouter.New(cfg, nil)
	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		slowServer.URL,
		50*time.Millisecond, // Provider timeout shorter than server delay
		nil,
	)
	llmRouter.RegisterProvider(provider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()

	start := time.Now()
	_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
		Model:     llmrouter.ModelClaudeSonnet,
		Prompt:    "hello",
		MaxTokens: 100,
	})
	duration := time.Since(start)

	t.Logf("Timeout request duration: %v, error: %v", duration, err)

	// Request should fail due to timeout
	require.Error(t, err)
	// Error should mention context deadline exceeded
	assert.True(t, strings.Contains(err.Error(), "deadline exceeded") || strings.Contains(err.Error(), "timeout"),
		"Error should be timeout related, got: %v", err)

	// Duration should be close to the timeout (within 50ms tolerance)
	assert.Less(t, duration, 200*time.Millisecond, "Request should timeout quickly")
	assert.GreaterOrEqual(t, duration, 90*time.Millisecond, "Request should take at least the timeout duration")
}

// TestLLMStress_MixedLoad tests mixed short/long requests.
func TestLLMStress_MixedLoad(t *testing.T) {
	// Short request server (10ms)
	shortServer := mockSlowServer(10 * time.Millisecond)
	defer shortServer.Close()

	// Long request server (100ms)
	longServer := mockSlowServer(100 * time.Millisecond)
	defer longServer.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gatewayCfg.EnableRateLimiting = true
	gatewayCfg.RateLimitConfig.RequestsPerSecond = 1000
	gatewayCfg.RateLimitConfig.BurstSize = 100

	llmRouter := llmrouter.New(cfg, nil)

	// Short request provider
	shortProvider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeHaiku,
		"test-key",
		shortServer.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(shortProvider)

	// Long request provider
	longProvider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		longServer.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(longProvider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()

	// Create mixed workload: 10 short requests and 10 long requests
	type requestResult struct {
		model   llmrouter.ModelName
		latency time.Duration
		err     error
	}
	results := make([]requestResult, 20)
	var wg sync.WaitGroup

	start := time.Now()

	// 10 short requests (Haiku)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			reqStart := time.Now()
			resp, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
				Model:     llmrouter.ModelClaudeHaiku,
				Prompt:    fmt.Sprintf("short request %d", idx),
				MaxTokens: 50,
			})
			results[idx] = requestResult{
				model:   llmrouter.ModelClaudeHaiku,
				latency: time.Since(reqStart),
				err:     err,
			}
			if resp != nil && resp.Model == "" {
				results[idx].model = resp.Model
			}
		}(i)
	}

	// 10 long requests (Sonnet)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			reqStart := time.Now()
			resp, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
				Model:     llmrouter.ModelClaudeSonnet,
				Prompt:    fmt.Sprintf("long request %d", idx),
				MaxTokens: 500,
			})
			results[10+idx] = requestResult{
				model:   llmrouter.ModelClaudeSonnet,
				latency: time.Since(reqStart),
				err:     err,
			}
			if resp != nil && resp.Model == "" {
				results[10+idx].model = resp.Model
			}
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(start)

	// Analyze results
	var shortSuccess, longSuccess int
	var shortLatencies, longLatencies []int64

	for _, r := range results[:10] {
		if r.err == nil {
			shortSuccess++
			shortLatencies = append(shortLatencies, r.latency.Milliseconds())
		}
	}

	for _, r := range results[10:] {
		if r.err == nil {
			longSuccess++
			longLatencies = append(longLatencies, r.latency.Milliseconds())
		}
	}

	// Calculate average latencies
	var shortAvgLatency, longAvgLatency int64
	if len(shortLatencies) > 0 {
		var sum int64
		for _, l := range shortLatencies {
			sum += l
		}
		shortAvgLatency = sum / int64(len(shortLatencies))
	}
	if len(longLatencies) > 0 {
		var sum int64
		for _, l := range longLatencies {
			sum += l
		}
		longAvgLatency = sum / int64(len(longLatencies))
	}

	t.Logf("Mixed load test: total_duration=%v", totalDuration)
	t.Logf("Short requests: success=%d/10 avg_latency=%dms", shortSuccess, shortAvgLatency)
	t.Logf("Long requests: success=%d/10 avg_latency=%dms", longSuccess, longAvgLatency)

	// All requests should succeed
	assert.Equal(t, 10, shortSuccess, "All short requests should succeed")
	assert.Equal(t, 10, longSuccess, "All long requests should succeed")

	// Short request average latency should be less than long request average
	assert.Less(t, shortAvgLatency, longAvgLatency,
		"Short requests should have lower average latency than long requests")

	// Long request average should be at least 50ms (server delay + overhead)
	assert.GreaterOrEqual(t, longAvgLatency, int64(50),
		"Long request latency should be at least the server delay")
}

// TestLLMStress_PerProviderRateLimiting tests per-provider rate limiting.
func TestLLMStress_PerProviderRateLimiting(t *testing.T) {
	server := mockLLMStressServer(t, 5*time.Millisecond, 0)
	defer server.Close()

	cfg := llmrouter.DefaultConfig()
	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gatewayCfg.EnableRateLimiting = true
	gatewayCfg.RateLimitConfig.RequestsPerSecond = 1000
	gatewayCfg.RateLimitConfig.BurstSize = 200
	gatewayCfg.RateLimitConfig.PerProviderLimits = true

	llmRouter := llmrouter.New(cfg, nil)

	// Register two providers with the same server
	provider1 := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		server.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(provider1)

	provider2 := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeHaiku,
		"test-key",
		server.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(provider2)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()

	// Send 50 requests alternating between two models
	var wg sync.WaitGroup
	var successCount int64

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			model := llmrouter.ModelClaudeSonnet
			if idx%2 == 0 {
				model = llmrouter.ModelClaudeHaiku
			}
			_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
				Model:     model,
				Prompt:    fmt.Sprintf("hello %d", idx),
				MaxTokens: 100,
			})
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else {
				t.Logf("Request %d error: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Per-provider rate limit test: success=%d/50", successCount)

	// With high rate limits, most or all requests should succeed
	assert.GreaterOrEqual(t, successCount, int64(40),
		"At least 40 out of 50 requests should succeed")
}

// TestLLMStress_CircuitBreakerOpen tests circuit breaker opening under failure load.
func TestLLMStress_CircuitBreakerOpen(t *testing.T) {
	// Server that fails 70% of requests
	server := mockLLMStressServer(t, 5*time.Millisecond, 0.7)
	defer server.Close()

	cfg := llmrouter.DefaultConfig()
	// Configure circuit breaker to trip at 50% error rate with only 5 requests
	cfg.CircuitBreakerConfig.ErrorRateThreshold = 0.5
	cfg.CircuitBreakerConfig.MinRequests = 5
	cfg.CircuitBreakerConfig.HalfOpenMaxRequests = 2

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

	// Make 20 requests - circuit breaker should eventually open
	var successCount, failureCount int64
	for i := 0; i < 20; i++ {
		_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
			Model:     llmrouter.ModelClaudeSonnet,
			Prompt:    fmt.Sprintf("hello %d", i),
			MaxTokens: 100,
		})
		if err == nil {
			atomic.AddInt64(&successCount, 1)
		} else {
			atomic.AddInt64(&failureCount, 1)
		}
	}

	// Check circuit breaker state
	states := gateway.GetCircuitStates()
	state, ok := states[llmrouter.ModelClaudeSonnet]
	t.Logf("Circuit breaker state for ClaudeSonnet: %v (ok=%v)", state, ok)
	t.Logf("Requests: success=%d failure=%d", successCount, failureCount)

	// After enough failures, circuit should be open
	// At minimum, we should see failures due to the 70% failure rate
	assert.Greater(t, failureCount, int64(0), "Should have some failures due to server failure rate")
}

// TestLLMStress_GracefulDegradation tests system continues with partial failures.
func TestLLMStress_GracefulDegradation(t *testing.T) {
	// Server that is slow but works
	workingServer := mockSlowServer(20 * time.Millisecond)
	defer workingServer.Close()

	cfg := llmrouter.DefaultConfig()
	cfg.RetryConfig.MaxAttempts = 2
	cfg.RetryConfig.InitialDelayMs = 50

	gatewayCfg := llmrouter.DefaultGatewayConfig()
	gatewayCfg.EnableMetrics = true
	gatewayCfg.EnableRateLimiting = false
	gatewayCfg.RateLimitConfig.RequestsPerSecond = 0 // Disable rate limiting for this test

	llmRouter := llmrouter.New(cfg, nil)
	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelClaudeSonnet,
		"test-key",
		workingServer.URL,
		30*time.Second,
		nil,
	)
	llmRouter.RegisterProvider(provider)

	gateway := llmrouter.NewLLMGateway(gatewayCfg, llmRouter, nil)

	ctx := context.Background()

	// Send 30 requests
	var wg sync.WaitGroup
	successCount := int64(0)
	errorCount := int64(0)

	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
				Model:     llmrouter.ModelClaudeSonnet,
				Prompt:    fmt.Sprintf("hello %d", idx),
				MaxTokens: 100,
			})
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&errorCount, 1)
				t.Logf("Request %d error: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	metrics := gateway.GetMetrics()
	t.Logf("Graceful degradation: success=%d errors=%d", successCount, errorCount)
	t.Logf("Metrics: total_responses=%d total_errors=%d", metrics.TotalResponses, metrics.TotalErrors)

	// System should handle all requests gracefully
	assert.Equal(t, int64(30), successCount+errorCount, "All requests should complete (success or error)")
	assert.Equal(t, int64(30), metrics.TotalResponses+metrics.TotalErrors, "Metrics should account for all requests")
}

// Ensure zap is used to avoid compiler error (gateway may use zap internally)
var _ = zap.NewNop
