// Package tracing provides OpenTelemetry distributed tracing with OTLP export
// and probability sampling for the Cloud Agent Platform.
package tracing

import (
	"math"

	"go.opentelemetry.io/otel/sdk/trace"
)

// SamplerConfig holds configuration for adaptive sampling.
type SamplerConfig struct {
	// InitialSampleRate is the initial probability sampling rate (0.0 to 1.0).
	InitialSampleRate float64
	// MinSampleRate is the minimum sampling rate for adaptive sampling.
	MinSampleRate float64
	// MaxSampleRate is the maximum sampling rate for adaptive sampling.
	MaxSampleRate float64
	// ErrorRateThreshold is the error rate threshold for increasing sample rate.
	ErrorRateThreshold float64
	// TargetQPS is the target queries per second for adaptive sampling.
	TargetQPS float64
}

// DefaultSamplerConfig returns a default sampler configuration.
func DefaultSamplerConfig() SamplerConfig {
	return SamplerConfig{
		InitialSampleRate:    0.1,  // 10%
		MinSampleRate:        0.01, // 1%
		MaxSampleRate:        1.0,  // 100%
		ErrorRateThreshold:   0.05, // 5%
		TargetQPS:            100,
	}
}

// ProbabilitySampler creates a sampler with fixed probability sampling.
func ProbabilitySampler(sampleRate float64) trace.Sampler {
	return trace.TraceIDRatioBased(sampleRate)
}

// AdaptiveSampler creates a sampler that adjusts based on error rates.
// Currently implements a simplified version that uses TraceIDRatioBased.
func AdaptiveSampler(cfg SamplerConfig) trace.Sampler {
	if cfg.InitialSampleRate <= 0 {
		cfg.InitialSampleRate = 0.1
	}
	if cfg.InitialSampleRate > 1 {
		cfg.InitialSampleRate = 1
	}
	if cfg.MinSampleRate <= 0 {
		cfg.MinSampleRate = 0.01
	}
	if cfg.MaxSampleRate <= 0 || cfg.MaxSampleRate > 1 {
		cfg.MaxSampleRate = 1
	}

	return &adaptiveSampler{
		cfg:          cfg,
		currentRate:  cfg.InitialSampleRate,
		errorCounts:  make(map[string]int),
		requestCounts: make(map[string]int),
	}
}

// adaptiveSampler implements adaptive sampling based on error rates.
type adaptiveSampler struct {
	cfg          SamplerConfig
	currentRate  float64
	errorCounts  map[string]int
	requestCounts map[string]int
}

// ShouldSample implements trace.Sampler.
func (s *adaptiveSampler) ShouldSample(p trace.SamplingParameters) trace.SamplingResult {
	// Use trace ID to determine if this span should be sampled
	traceID := p.TraceID

	// Convert trace ID to a value between 0 and 1
	// Use the first 64 bits of the trace ID
	value := float64(traceID[0]) / 255.0

	if value < s.currentRate {
		return trace.SamplingResult{
			Decision: trace.RecordAndSample,
		}
	}

	return trace.SamplingResult{
		Decision: trace.Drop,
	}
}

// Description implements trace.Sampler.
func (s *adaptiveSampler) Description() string {
	return "AdaptiveSampler"
}

// UpdateErrorRate updates the current sample rate based on error rate.
// This would be called by a background goroutine that monitors error rates.
func (s *adaptiveSampler) UpdateErrorRate(serviceName string, errorRate float64) {
	if errorRate > s.cfg.ErrorRateThreshold {
		// Increase sample rate to capture more diagnostics
		s.currentRate = math.Min(s.currentRate*1.5, s.cfg.MaxSampleRate)
	} else if errorRate < s.cfg.ErrorRateThreshold*0.5 {
		// Decrease sample rate to reduce overhead
		s.currentRate = math.Max(s.currentRate*0.9, s.cfg.MinSampleRate)
	}
}

// GetCurrentSampleRate returns the current sample rate.
func (s *adaptiveSampler) GetCurrentSampleRate() float64 {
	return s.currentRate
}

// NeverSample returns a sampler that never samples.
func NeverSample() trace.Sampler {
	return trace.NeverSample()
}

// AlwaysSample returns a sampler that always samples.
func AlwaysSample() trace.Sampler {
	return trace.AlwaysSample()
}
