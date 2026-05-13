// Package metrics provides metrics initialization and Prometheus endpoint handling.
package metrics

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Provider handles metrics initialization and lifecycle.
type Provider struct {
	meterProvider *sdkmetric.MeterProvider
	metrics       *Metrics
}

// Config holds metrics provider configuration.
type Config struct {
	ServiceName string
}

// NewProvider creates a new metrics provider with Prometheus exporter.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	// Create Prometheus exporter
	exporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	// Create meter provider
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
	)

	// Set as global provider
	otel.SetMeterProvider(meterProvider)

	// Create Prometheus metrics
	promMetrics := NewMetrics()

	return &Provider{
		meterProvider: meterProvider,
		metrics:       promMetrics,
	}, nil
}

// Metrics returns the Prometheus metrics instance.
func (p *Provider) Metrics() *Metrics {
	return p.metrics
}

// Shutdown gracefully shuts down the metrics provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.meterProvider != nil {
		return p.meterProvider.Shutdown(ctx)
	}
	return nil
}

// Handler returns an HTTP handler for the /metrics endpoint.
func Handler() http.Handler {
	return promhttp.Handler()
}

// Server starts an HTTP server for the metrics endpoint.
func Server(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		promhttp.Handler().ServeHTTP(w, r)
	})

	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}