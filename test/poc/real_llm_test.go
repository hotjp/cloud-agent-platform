//go:build real_llm

package poc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/cloud-agent-platform/cap/plugins/llmrouter"
)

// TestRealMiniMax_Call verifies end-to-end LLM call through real MiniMax API.
// Run with: go test -tags=real_llm -run TestRealMiniMax_Call -v ./test/poc/
func TestRealMiniMax_Call(t *testing.T) {
	apiKey := getEnvOrDefault("MINIMAX_API_KEY", "YOUR_MINIMAX_API_KEY")
	endpoint := getEnvOrDefault("MINIMAX_ENDPOINT", "https://api.minimaxi.com/v1")
	modelName := llmrouter.ModelName("MiniMax-M2.7-highspeed")

	logger, _ := zap.NewDevelopment()

	provider := llmrouter.NewOpenAICompatibleProvider(
		modelName,
		apiKey,
		endpoint,
		30*time.Second,
		logger,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, &llmrouter.LLMRequest{
		Prompt:     "Reply with exactly: PONG",
		MaxTokens:  64,
		Temperature: 0.0,
	})

	require.NoError(t, err, "Real API call failed")
	assert.NotEmpty(t, resp.Content, "Response content should not be empty")
	assert.Equal(t, modelName, resp.Model)
	assert.Greater(t, resp.TokensUsed, 0, "Should report tokens used")
	assert.Greater(t, resp.LatencyMs, int64(0), "Should report latency")

	t.Logf("✅ Real MiniMax call succeeded!")
	t.Logf("   Content:    %q", resp.Content)
	t.Logf("   Tokens:     %d", resp.TokensUsed)
	t.Logf("   Latency:    %dms", resp.LatencyMs)
}

// TestRealMiniMax_ThroughGateway verifies the full gateway → router → provider chain.
func TestRealMiniMax_ThroughGateway(t *testing.T) {
	apiKey := getEnvOrDefault("MINIMAX_API_KEY", "YOUR_MINIMAX_API_KEY")
	endpoint := getEnvOrDefault("MINIMAX_ENDPOINT", "https://api.minimaxi.com/v1")
	model := llmrouter.ModelName("MiniMax-M2.7-highspeed")

	logger, _ := zap.NewDevelopment()

	cfg := llmrouter.DefaultConfig()
	cfg.PrimaryProvider = model

	lr := llmrouter.New(cfg, logger)
	lr.RegisterProvider(llmrouter.NewOpenAICompatibleProvider(model, apiKey, endpoint, 30*time.Second, logger))

	gateway := llmrouter.NewLLMGateway(llmrouter.DefaultGatewayConfig(), lr, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
		Model:       model,
		TaskType:    llmrouter.TaskTypeSimple,
		Prompt:      "What is 2+2? Reply with just the number.",
		MaxTokens:   32,
		Temperature: 0.0,
	})

	require.NoError(t, err, "Gateway call failed")
	assert.NotEmpty(t, resp.Content)

	// Verify gateway metrics (note: TotalRequests tracks RecordRequest calls, not actual requests)
	metrics := gateway.GetMetrics()
	assert.Equal(t, int64(1), metrics.TotalResponses, "Should have 1 response")
	assert.Equal(t, int64(0), metrics.TotalErrors, "Should have 0 errors")

	t.Logf("✅ Full gateway chain succeeded!")
	t.Logf("   Content:    %q", resp.Content)
	t.Logf("   Tokens:     %d", resp.TokensUsed)
	t.Logf("   Latency:    %dms", resp.LatencyMs)
	t.Logf("   Metrics:    req=%d resp=%d err=%d", metrics.TotalRequests, metrics.TotalResponses, metrics.TotalErrors)

	// Print provider stats
	stats := gateway.GetProviderStats()
	for m, s := range stats {
		t.Logf("   Provider %s: success=%d fail=%d avg_latency=%dms", m, s.SuccessRequests, s.FailedRequests, s.AvgLatencyMs)
	}
}

// TestRealMiniMax_ErrorHandling verifies error handling with invalid key.
func TestRealMiniMax_ErrorHandling(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	provider := llmrouter.NewOpenAICompatibleProvider(
		llmrouter.ModelName("MiniMax-M2.7-highspeed"),
		"invalid-key-12345",
		"https://api.minimaxi.com/v1",
		10*time.Second,
		logger,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := provider.Complete(ctx, &llmrouter.LLMRequest{
		Prompt:    "test",
		MaxTokens: 10,
	})

	assert.Error(t, err, "Should fail with invalid key")
	assert.Nil(t, resp)
	t.Logf("✅ Error handling works: %v", err)
}

// TestRealMiniMax_Concurrent verifies concurrent requests through gateway.
func TestRealMiniMax_Concurrent(t *testing.T) {
	apiKey := getEnvOrDefault("MINIMAX_API_KEY", "YOUR_MINIMAX_API_KEY")
	endpoint := getEnvOrDefault("MINIMAX_ENDPOINT", "https://api.minimaxi.com/v1")
	model := llmrouter.ModelName("MiniMax-M2.7-highspeed")

	logger, _ := zap.NewDevelopment()

	cfg := llmrouter.DefaultConfig()
	cfg.PrimaryProvider = model

	lr := llmrouter.New(cfg, logger)
	lr.RegisterProvider(llmrouter.NewOpenAICompatibleProvider(model, apiKey, endpoint, 30*time.Second, logger))

	gateway := llmrouter.NewLLMGateway(&llmrouter.GatewayConfig{
		EnableLogging:      true,
		EnableMetrics:      true,
		EnableRateLimiting: false, // Disable rate limiting for concurrent test
		RequestTimeout:     30 * time.Second,
	}, lr, logger)

	const numRequests = 5
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			_, err := gateway.Complete(ctx, &llmrouter.LLMRequest{
				Model:       model,
				TaskType:    llmrouter.TaskTypeSimple,
				Prompt:      "Reply with a single word: OK",
				MaxTokens:   16,
				Temperature: 0.0,
			})
			results <- err
		}(i)
	}

	successCount := 0
	for i := 0; i < numRequests; i++ {
		err := <-results
		if err == nil {
			successCount++
		} else {
			t.Logf("Request %d failed: %v", i, err)
		}
	}

	assert.GreaterOrEqual(t, successCount, 3, "At least 3 of 5 concurrent requests should succeed")

	metrics := gateway.GetMetrics()
	t.Logf("✅ Concurrent test: %d/%d succeeded", successCount, numRequests)
	t.Logf("   Metrics: req=%d resp=%d err=%d", metrics.TotalRequests, metrics.TotalResponses, metrics.TotalErrors)
}
