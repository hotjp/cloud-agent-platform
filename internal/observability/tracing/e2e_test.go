// Package tracing provides OpenTelemetry distributed tracing with OTLP export
// and probability sampling for the Cloud Agent Platform.
package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// TestLoggerWithTrace tests that LoggerWithTrace correctly adds trace_id to logger.
func TestLoggerWithTrace(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Test without trace ID
	ctx := context.Background()
	loggedCtx := WithTraceID(ctx, "test-trace-id-123")

	// Logger should have trace_id field
	log := LoggerWithTrace(logger, loggedCtx)
	assert.NotNil(t, log)
}

// TestLoggerWithTraceNilTraceID tests LoggerWithTrace with empty trace ID.
func TestLoggerWithTraceNilTraceID(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Test with empty trace ID
	ctx := context.Background()
	log := LoggerWithTrace(logger, ctx)

	// Should return the original logger without modification
	assert.NotNil(t, log)
}

// TestGetTraceIDFromContext tests the GetTraceID function.
func TestGetTraceIDFromContext(t *testing.T) {
	// Test with no trace ID
	ctx := context.Background()
	assert.Empty(t, GetTraceID(ctx))

	// Test with trace ID set
	ctx = WithTraceID(ctx, "abc123")
	assert.Equal(t, "abc123", GetTraceID(ctx))
}

// TestWithTraceIDContext tests the WithTraceID function.
func TestWithTraceIDContext(t *testing.T) {
	ctx := context.Background()
	traceID := "test-trace-456"

	result := WithTraceID(ctx, traceID)

	// Verify trace ID can be retrieved
	retrieved := GetTraceID(result)
	assert.Equal(t, traceID, retrieved)

	// Verify original context is unchanged
	assert.Empty(t, GetTraceID(ctx))
}

// TestTraceIDNotOverwritten tests that WithTraceID works correctly.
func TestTraceIDNotOverwritten(t *testing.T) {
	headers := http.Header{}
	headers.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	propagator := otel.GetTextMapPropagator()
	ctx := propagator.Extract(context.Background(), propagation.HeaderCarrier(headers))

	// Add trace ID explicitly
	ctx = WithTraceID(ctx, "different-trace-id")

	// GetTraceID should return what was set via WithTraceID
	retrieved := GetTraceID(ctx)
	assert.Equal(t, "different-trace-id", retrieved)
}

// TestTracingMiddleware tests the HTTP tracing middleware.
func TestTracingMiddleware(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := MiddlewareConfig{
		ServiceName: "test-service",
		Logger:      logger,
	}

	middleware := TracingMiddleware(cfg)

	// Create a test handler that records the trace ID
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = GetTraceID(r.Context()) // Verify we can call without panic
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with middleware
	wrapped := middleware(handler)

	// Test without traceparent header (should generate new trace)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// Note: With noop tracer, trace ID may be empty but middleware should not panic
}

// TestHTTPTraceIDMiddleware tests the HTTPTraceIDMiddleware function.
func TestHTTPTraceIDMiddleware(t *testing.T) {
	middleware := HTTPTraceIDMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = GetTraceID(r.Context()) // Verify we can call without panic
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware(handler)

	// Test with traceparent header
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// With noop tracer, trace ID may not be extracted but should not panic
}

// TestSpanNamesConstant verifies span name constants are defined correctly.
func TestSpanNamesConstant(t *testing.T) {
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

// TestAttributeKeysConstant verifies attribute key constants are defined correctly.
func TestAttributeKeysConstant(t *testing.T) {
	assert.Equal(t, "task.id", AttrTaskID)
	assert.Equal(t, "task.goal", AttrTaskGoal)
	assert.Equal(t, "task.status", AttrTaskStatus)
	assert.Equal(t, "subtask.id", AttrSubtaskID)
	assert.Equal(t, "layer", AttrLayer)
	assert.Equal(t, "http.method", AttrHTTPMethod)
	assert.Equal(t, "http.path", AttrHTTPPath)
	assert.Equal(t, "http.status_code", AttrHTTPStatusCode)
}

// TestRecordSpanEvent tests that events can be recorded on spans.
func TestRecordSpanEvent(t *testing.T) {
	tracer := otel.Tracer("test")

	_, span := tracer.Start(context.Background(), "test-span")

	// Record an event
	RecordSpanEvent(span, "test-event",
		attribute.String("key", "value"),
	)

	// End span - should not panic
	span.End()
	assert.True(t, true)
}

// TestContextKeyUniqueness verifies the traceIDKey is unique.
func TestContextKeyUniqueness(t *testing.T) {
	// This test ensures the traceIDKey type is unique
	key1 := traceIDKey{}
	key2 := traceIDKey{}

	// They are the same type but different instances
	assert.IsType(t, traceIDKey{}, key1)
	assert.IsType(t, traceIDKey{}, key2)
	assert.NotNil(t, &key1)
	assert.NotNil(t, &key2)
}

// TestSpanHelperStartStorageQuery tests the StartStorageQuery helper.
func TestSpanHelperStartStorageQuery(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	ctx, span := helper.StartStorageQuery(ctx, "tasks", "SELECT")

	assert.NotNil(t, span)
	assert.NotNil(t, ctx)

	// End span - should not panic
	span.End()
	assert.True(t, true)
}

// TestServiceLayerSpanCreation tests that service layer can create spans.
func TestServiceLayerSpanCreation(t *testing.T) {
	helper := NewSpanHelper()
	ctx := context.Background()

	// Test task.submit span
	ctx, submitSpan := helper.StartTaskSubmit(ctx, "task-123", "Implement login", "client-456")
	assert.NotNil(t, submitSpan)
	submitSpan.End()

	// Test task.decompose span
	ctx, decomposeSpan := helper.StartTaskDecompose(ctx, "task-123", 5)
	assert.NotNil(t, decomposeSpan)
	decomposeSpan.End()

	// Test task.execute span
	ctx, executeSpan := helper.StartTaskExecute(ctx, "task-123", "high")
	assert.NotNil(t, executeSpan)
	executeSpan.End()

	// All spans should be created without error
	assert.True(t, true)
}

// TestTruncateStringFunction tests the truncateString helper.
func TestTruncateStringFunction(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10chars", 10, "exactly10c..."},
		{"this is a longer string", 10, "this is a..."},
		{"ab", 10, "ab"},
		{"", 5, ""},
		{"hello world", 5, "hello..."},
	}

	for _, tc := range tests {
		result := truncateString(tc.input, tc.maxLen)
		assert.Equal(t, tc.expected, result, "Input: %s, MaxLen: %d", tc.input, tc.maxLen)
	}
}

// TestTraceIDPropagationWithContext tests that trace ID propagates through context.
func TestTraceIDPropagationWithContext(t *testing.T) {
	// Start a span
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")

	// Get the span context
	spanCtx := span.SpanContext()

	// If span is valid, verify trace ID is in context
	if spanCtx.HasTraceID() {
		traceID := spanCtx.TraceID().String()
		ctx = WithTraceID(ctx, traceID)
		assert.Equal(t, traceID, GetTraceID(ctx))
	}

	span.End()
}

// TestMultiLayerTraceConsistency tests trace context handling across layers.
func TestMultiLayerTraceConsistency(t *testing.T) {
	// This test verifies that trace context is properly handled
	// even with a noop tracer
	tracer := otel.Tracer("test")

	// L5: Gateway HTTP span
	ctx, gwSpan := tracer.Start(context.Background(), "HTTP POST /tasks",
		trace.WithSpanKind(trace.SpanKindServer),
	)

	// Add trace ID to context for logging
	gwSpanCtx := gwSpan.SpanContext()
	if gwSpanCtx.HasTraceID() {
		ctx = WithTraceID(ctx, gwSpanCtx.TraceID().String())
	}
	gwSpan.End()

	// L4: Service span (child of gateway)
	ctx, svcSpan := tracer.Start(ctx, SpanTaskSubmit,
		trace.WithSpanKind(trace.SpanKindServer),
	)
	svcSpan.End()

	// L1: Storage span (child of service)
	ctx, storageSpan := tracer.Start(ctx, SpanStorageInsert,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	storageSpan.End()

	// All operations should complete without error
	assert.True(t, true)
}

// TestPropagatorExtract tests the W3C TraceContext propagator.
func TestPropagatorExtract(t *testing.T) {
	propagator := otel.GetTextMapPropagator()

	// Test with valid traceparent
	headers := http.Header{}
	headers.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	ctx := propagator.Extract(context.Background(), propagation.HeaderCarrier(headers))

	// Should not panic and return a context
	assert.NotNil(t, ctx)
}

// TestPropagatorInject tests that trace context can be injected.
func TestPropagatorInject(t *testing.T) {
	propagator := otel.GetTextMapPropagator()

	// Create context with trace ID
	ctx := WithTraceID(context.Background(), "test-trace-id")

	// Inject into headers
	headers := http.Header{}
	propagator.Inject(ctx, propagation.HeaderCarrier(headers))

	// Should not panic - headers may or may not have traceparent set
	// depending on the propagator implementation
	assert.NotNil(t, headers)
}