// Package tracing provides OpenTelemetry distributed tracing with OTLP export
// and probability sampling for the Cloud Agent Platform.
package tracing

// TelemetryConfig holds OpenTelemetry tracing configuration.
type TelemetryConfig struct {
	// ServiceName is the name of the service for telemetry.
	ServiceName string
	// Endpoint is the OTLP collector endpoint (e.g., "localhost:4317").
	Endpoint string
	// SampleRate is the probability sampling rate (0.0 to 1.0).
	SampleRate float64
}

// DefaultTelemetryConfig returns a default telemetry configuration.
func DefaultTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{
		ServiceName: "cloud-agent-platform",
		Endpoint:    "http://localhost:4317",
		SampleRate:  0.1, // 10% sampling by default
	}
}
