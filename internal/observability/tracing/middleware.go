// Package tracing provides OpenTelemetry distributed tracing with OTLP export
// and probability sampling for the Cloud Agent Platform.
package tracing

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// MiddlewareConfig holds configuration for the tracing middleware.
type MiddlewareConfig struct {
	ServiceName string
	Logger      *zap.Logger
}

// TracingMiddleware returns an HTTP middleware that extracts or creates
// a trace context and adds trace_id to the request context.
func TracingMiddleware(cfg MiddlewareConfig) func(http.Handler) http.Handler {
	tracer := otel.Tracer("github.com/cloud-agent-platform/cap/middleware")
	propagator := otel.GetTextMapPropagator()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from incoming request headers
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Start a new span for the HTTP request
			spanName := r.Method + " " + r.URL.Path
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String(AttrHTTPMethod, r.Method),
					attribute.String(AttrHTTPPath, r.URL.Path),
					attribute.String("http.url", r.URL.String()),
					attribute.String("http.host", r.Host),
					attribute.String("http.user_agent", r.UserAgent()),
					attribute.String(AttrLayer, "L5"),
				),
			)
			defer func() {
				span.SetStatus(codes.Ok, "")
				span.End()
			}()

			// Inject trace context into response headers
			propagator.Inject(ctx, propagation.HeaderCarrier(w.Header()))

			// Add trace ID to request context for downstream use
			spanCtx := span.SpanContext()
			if spanCtx.HasTraceID() {
				ctx = WithTraceID(ctx, spanCtx.TraceID().String())
			}

			// Process request
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// traceIDKey is the context key for trace ID.
type traceIDKey struct{}

// WithTraceID returns a new context with the given trace ID.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// GetTraceID returns the trace ID from the context, or empty string if not present.
func GetTraceID(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey{}).(string); ok {
		return id
	}
	return ""
}

// HTTPTraceIDMiddleware is an alternative middleware that only adds trace ID
// to the context without creating a span (for use in tests or minimal tracing).
func HTTPTraceIDMiddleware() func(http.Handler) http.Handler {
	propagator := otel.GetTextMapPropagator()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Get trace ID if present
			spanCtx := trace.SpanContextFromContext(ctx)
			if spanCtx.HasTraceID() {
				ctx = WithTraceID(ctx, spanCtx.TraceID().String())
			}

			// Process request with trace context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// LoggerWithTrace returns a logger with trace_id field added from the context.
func LoggerWithTrace(logger *zap.Logger, ctx context.Context) *zap.Logger {
	traceID := GetTraceID(ctx)
	if traceID == "" {
		return logger
	}
	return logger.With(zap.String("trace_id", traceID))
}
