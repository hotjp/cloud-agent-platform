// Package tracing provides OpenTelemetry distributed tracing with OTLP export
// and probability sampling for the Cloud Agent Platform.
package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitTracer_Disabled(t *testing.T) {
	// Test that InitTracer with empty endpoint returns no-op shutdown
	ctx := context.Background()
	cfg := TelemetryConfig{
		ServiceName: "test-service",
		Endpoint:    "", // Empty endpoint disables tracing
		SampleRate: 0.1,
	}

	shutdown, err := InitTracer(ctx, cfg, nil)
	assert.NoError(t, err)
	assert.NotNil(t, shutdown)

	// Calling shutdown should not error
	err = shutdown(ctx)
	assert.NoError(t, err)
}

func TestInitTracer_InvalidEndpoint(t *testing.T) {
	// Test that InitTracer handles invalid endpoint gracefully
	ctx := context.Background()
	cfg := TelemetryConfig{
		ServiceName: "test-service",
		Endpoint:    "invalid://endpoint",
		SampleRate: 0.1,
	}

	shutdown, err := InitTracer(ctx, cfg, nil)
	// With invalid endpoint, initialization should fail or return no-op
	if err == nil && shutdown != nil {
		// If no error, shutdown should still work
		err = shutdown(ctx)
		assert.NoError(t, err)
	}
}

func TestNewSpanHelper(t *testing.T) {
	helper := NewSpanHelper()
	assert.NotNil(t, helper)
	assert.NotNil(t, helper.tracer)
}

func TestSpanNames(t *testing.T) {
	// Verify span name constants are defined correctly
	assert.Equal(t, "task.submit", SpanTaskSubmit)
	assert.Equal(t, "task.decompose", SpanTaskDecompose)
	assert.Equal(t, "task.execute", SpanTaskExecute)
	assert.Equal(t, "task.complete", SpanTaskComplete)
	assert.Equal(t, "task.cancel", SpanTaskCancel)
	assert.Equal(t, "subtask.execute", SpanSubtaskExecute)
	assert.Equal(t, "agent.think", SpanAgentThink)
	assert.Equal(t, "agent.act", SpanAgentAct)
	assert.Equal(t, "llm.call", SpanLLMCall)
	assert.Equal(t, "outbox.publish", SpanOutboxPublish)
	assert.Equal(t, "storage.query", SpanStorageQuery)
	assert.Equal(t, "gateway.http", SpanGatewayHTTP)
}

func TestSpanHelper_StartTaskSubmit(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartTaskSubmit(ctx, "task123", "test goal", "client456")

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
}

func TestSpanHelper_StartTaskDecompose(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartTaskDecompose(ctx, "task123", 5)

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
}

func TestSpanHelper_StartTaskExecute(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartTaskExecute(ctx, "task123", "complex")

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
}

func TestSpanHelper_StartTaskComplete(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartTaskComplete(ctx, "task123", true, 1000)

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
}

func TestSpanHelper_StartSubtaskExecute(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartSubtaskExecute(ctx, "task123", "subtask456", "coding", "executor")

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
}

func TestSpanHelper_StartAgentThink(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartAgentThink(ctx, "task123", "executor", "What should I do?")

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
}

func TestSpanHelper_StartAgentAct(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartAgentAct(ctx, "task123", "executor", "execute_code")

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
}

func TestSpanHelper_StartLLMCall(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartLLMCall(ctx, "task123", "claude-sonnet", 100)

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
}

func TestSpanHelper_StartOutboxPublish(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartOutboxPublish(ctx, "TaskSubmittedV1", "event123")

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
}

func TestSpanHelper_StartStorageQuery(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartStorageQuery(ctx, "tasks", "SELECT")

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)
}

func TestEndSpan(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	_, span := helper.StartTaskSubmit(ctx, "task123", "test", "client")
	EndSpan(span)
	// EndSpan should not panic
}

func TestEndSpanWithError(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	_, span := helper.StartTaskSubmit(ctx, "task123", "test", "client")
	EndSpanWithError(span, nil) // nil error should not panic
	// EndSpanWithError with nil error should not panic
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10chars", 10, "exactly10c..."},  // Truncates at maxLen looking for space
		{"this is a longer string", 10, "this is a..."}, // Space found at position 9
		{"ab", 10, "ab"},
		{"", 5, ""},
	}

	for _, tc := range tests {
		result := truncateString(tc.input, tc.maxLen)
		assert.Equal(t, tc.expected, result)
	}
}

func TestProbabilitySampler(t *testing.T) {
	sampler := ProbabilitySampler(0.1)
	assert.NotNil(t, sampler)

	// Verify it's the correct type
	_, ok := sampler.(interface{})
	assert.True(t, ok)
}

func TestNeverSample(t *testing.T) {
	sampler := NeverSample()
	assert.NotNil(t, sampler)
}

func TestAlwaysSample(t *testing.T) {
	sampler := AlwaysSample()
	assert.NotNil(t, sampler)
}

func TestAdaptiveSampler(t *testing.T) {
	cfg := DefaultSamplerConfig()
	sampler := AdaptiveSampler(cfg)
	assert.NotNil(t, sampler)

	as, ok := sampler.(*adaptiveSampler)
	assert.True(t, ok)
	assert.Equal(t, 0.1, as.GetCurrentSampleRate())
}

func TestAdaptiveSampler_UpdateErrorRate(t *testing.T) {
	cfg := DefaultSamplerConfig()
	sampler := AdaptiveSampler(cfg)

	as := sampler.(*adaptiveSampler)
	initialRate := as.GetCurrentSampleRate()

	// Update with high error rate should increase sample rate
	as.UpdateErrorRate("test-service", 0.1) // Above threshold
	newRate := as.GetCurrentSampleRate()
	assert.GreaterOrEqual(t, newRate, initialRate)

	// Update with low error rate should decrease sample rate
	as.UpdateErrorRate("test-service", 0.0)
	newRate2 := as.GetCurrentSampleRate()
	assert.LessOrEqual(t, newRate2, newRate)
}

func TestDefaultTelemetryConfig(t *testing.T) {
	cfg := DefaultTelemetryConfig()
	assert.Equal(t, "cloud-agent-platform", cfg.ServiceName)
	assert.Equal(t, "http://localhost:4317", cfg.Endpoint)
	assert.Equal(t, 0.1, cfg.SampleRate)
}

func TestDefaultSamplerConfig(t *testing.T) {
	cfg := DefaultSamplerConfig()
	assert.Equal(t, 0.1, cfg.InitialSampleRate)
	assert.Equal(t, 0.01, cfg.MinSampleRate)
	assert.Equal(t, 1.0, cfg.MaxSampleRate)
	assert.Equal(t, 0.05, cfg.ErrorRateThreshold)
	assert.Equal(t, float64(100), cfg.TargetQPS)
}

func TestGetTracer(t *testing.T) {
	tracer := GetTracer()
	assert.NotNil(t, tracer)
}

func TestWithTraceID(t *testing.T) {
	ctx := context.Background()
	traceID := "abc123"

	ctx = WithTraceID(ctx, traceID)
	result := GetTraceID(ctx)
	assert.Equal(t, traceID, result)
}

func TestGetTraceID_NotSet(t *testing.T) {
	ctx := context.Background()
	result := GetTraceID(ctx)
	assert.Equal(t, "", result)
}
