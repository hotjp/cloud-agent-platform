// Package tracing provides OpenTelemetry distributed tracing with OTLP export
// and probability sampling for the Cloud Agent Platform.
//
// Span naming convention: lowercase with dots (e.g., task.submit, llm.call).
// Trace context is propagated via W3C TraceContext headers.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// InitTracer initializes the OpenTelemetry tracer provider with OTLP export
// and probability sampling. It returns a shutdown function that should be
// called on application shutdown.
//
// Configuration is loaded from the provided TelemetryConfig.
// If endpoint is empty, a no-op tracer is used.
func InitTracer(ctx context.Context, cfg TelemetryConfig, logger *zap.Logger) (func(context.Context) error, error) {
	// Use nop logger if none provided
	if logger == nil {
		logger = zap.NewNop()
	}

	// If endpoint is empty, tracing is disabled - use no-op tracer
	if cfg.Endpoint == "" {
		logger.Info("tracing disabled: no endpoint configured")
		return func(ctx context.Context) error { return nil }, nil
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion("1.0.0"),
		),
		resource.WithHost(),
		resource.WithOS(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create OTLP trace exporter
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(), // Allow insecure for local development
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// Determine sample rate
	sampleRate := cfg.SampleRate
	if sampleRate <= 0 {
		sampleRate = 0.1 // Default 10% sampling
	}
	if sampleRate > 1 {
		sampleRate = 1
	}

	// Create sampler with probability sampling
	sampler := sdktrace.TraceIDRatioBased(sampleRate)

	// Create tracer provider with batching
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exporter),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator for W3C TraceContext
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("tracing initialized",
		zap.String("service_name", cfg.ServiceName),
		zap.String("endpoint", cfg.Endpoint),
		zap.Float64("sample_rate", sampleRate),
	)

	// Return shutdown function
	return func(ctx context.Context) error {
		logger.Info("shutting down tracer provider")
		return tp.Shutdown(ctx)
	}, nil
}

// GetTracer returns the global tracer.
func GetTracer() trace.Tracer {
	return otel.Tracer("github.com/cloud-agent-platform/cap")
}
