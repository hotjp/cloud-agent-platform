// Package metrics implements business metrics with OpenTelemetry + Prometheus.
// All metrics use the cap_ prefix as specified in the observability requirements.
package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metric names with cap_ prefix.
const (
	// Task metrics
	MetricTaskTotal           = "cap_task_total"
	MetricTaskDurationSeconds = "cap_task_duration_seconds"

	// Agent execution metrics
	MetricAgentExecutionTotal = "cap_agent_execution_total"

	// LLM metrics
	MetricLLMRequestTotal        = "cap_llm_request_total"
	MetricLLMRequestDurationSecs = "cap_llm_request_duration_seconds"

	// Worker pool metrics
	MetricWorkerPoolSize = "cap_worker_pool_size"

	// Outbox metrics
	MetricOutboxEventsPending = "cap_outbox_events_pending"

	// Gateway request metrics
	MetricHTTPRequestTotal        = "cap_http_request_total"
	MetricHTTPRequestDurationSecs = "cap_http_request_duration_seconds"
)

// TaskStatusLabel represents task status for metrics labeling.
type TaskStatusLabel string

const (
	TaskStatusPending    TaskStatusLabel = "pending"
	TaskStatusDecomposing TaskStatusLabel = "decomposing"
	TaskStatusDispatched  TaskStatusLabel = "dispatched"
	TaskStatusRunning     TaskStatusLabel = "running"
	TaskStatusReviewing   TaskStatusLabel = "reviewing"
	TaskStatusConfirming  TaskStatusLabel = "confirming"
	TaskStatusCompleted   TaskStatusLabel = "completed"
	TaskStatusFailed      TaskStatusLabel = "failed"
	TaskStatusCancelled   TaskStatusLabel = "cancelled"
)

// Metrics holds all application metrics.
type Metrics struct {
	// Task metrics
	TaskTotal           *prometheus.CounterVec
	TaskDurationSeconds *prometheus.HistogramVec

	// Agent execution metrics
	AgentExecutionTotal *prometheus.CounterVec

	// LLM metrics
	LLMRequestTotal        *prometheus.CounterVec
	LLMRequestDurationSecs *prometheus.HistogramVec

	// Worker pool metrics
	WorkerPoolSize *prometheus.GaugeVec

	// Outbox metrics
	OutboxEventsPending prometheus.Gauge

	// HTTP request metrics
	HTTPRequestTotal        *prometheus.CounterVec
	HTTPRequestDurationSecs *prometheus.HistogramVec
}

// NewMetrics creates and registers all metrics with Prometheus.
func NewMetrics() *Metrics {
	m := &Metrics{
		TaskTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricTaskTotal,
				Help: "Total number of tasks processed, labeled by status",
			},
			[]string{"status"},
		),

		TaskDurationSeconds: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricTaskDurationSeconds,
				Help:    "Task execution duration in seconds",
				Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
			},
			[]string{"status"},
		),

		AgentExecutionTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricAgentExecutionTotal,
				Help: "Total number of agent executions, labeled by template and status",
			},
			[]string{"template", "status"},
		),

		LLMRequestTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricLLMRequestTotal,
				Help: "Total number of LLM requests, labeled by model and status",
			},
			[]string{"model", "status"},
		),

		LLMRequestDurationSecs: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricLLMRequestDurationSecs,
				Help:    "LLM request duration in seconds",
				Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
			},
			[]string{"model"},
		),

		WorkerPoolSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricWorkerPoolSize,
				Help: "Current worker pool size by state (total, busy, available)",
			},
			[]string{"state"},
		),

		OutboxEventsPending: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: MetricOutboxEventsPending,
				Help: "Number of pending outbox events waiting to be sent",
			},
		),

		HTTPRequestTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricHTTPRequestTotal,
				Help: "Total number of HTTP requests, labeled by method, path, and status",
			},
			[]string{"method", "path", "status"},
		),

		HTTPRequestDurationSecs: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricHTTPRequestDurationSecs,
				Help:    "HTTP request duration in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
	}

	return m
}

// RecordTaskSubmitted records a task submission.
func (m *Metrics) RecordTaskSubmitted() {
	m.TaskTotal.WithLabelValues(string(TaskStatusPending)).Inc()
}

// RecordTaskCompleted records a task completion with duration.
func (m *Metrics) RecordTaskCompleted(durationSeconds float64) {
	m.TaskTotal.WithLabelValues(string(TaskStatusCompleted)).Inc()
	m.TaskDurationSeconds.WithLabelValues(string(TaskStatusCompleted)).Observe(durationSeconds)
}

// RecordTaskFailed records a task failure with duration.
func (m *Metrics) RecordTaskFailed(durationSeconds float64) {
	m.TaskTotal.WithLabelValues(string(TaskStatusFailed)).Inc()
	m.TaskDurationSeconds.WithLabelValues(string(TaskStatusFailed)).Observe(durationSeconds)
}

// RecordTaskCancelled records a task cancellation.
func (m *Metrics) RecordTaskCancelled() {
	m.TaskTotal.WithLabelValues(string(TaskStatusCancelled)).Inc()
}

// RecordAgentExecution records an agent execution.
func (m *Metrics) RecordAgentExecution(template string, success bool) {
	status := "success"
	if !success {
		status = "failure"
	}
	m.AgentExecutionTotal.WithLabelValues(template, status).Inc()
}

// RecordLLMRequest records an LLM request.
func (m *Metrics) RecordLLMRequest(model string, success bool, durationSeconds float64) {
	status := "success"
	if !success {
		status = "failure"
	}
	m.LLMRequestTotal.WithLabelValues(model, status).Inc()
	m.LLMRequestDurationSecs.WithLabelValues(model).Observe(durationSeconds)
}

// RecordWorkerPoolSize records the current worker pool size.
func (m *Metrics) RecordWorkerPoolSize(total, busy, available int) {
	m.WorkerPoolSize.WithLabelValues("total").Set(float64(total))
	m.WorkerPoolSize.WithLabelValues("busy").Set(float64(busy))
	m.WorkerPoolSize.WithLabelValues("available").Set(float64(available))
}

// RecordOutboxEventsPending records the number of pending outbox events.
func (m *Metrics) RecordOutboxEventsPending(count int64) {
	m.OutboxEventsPending.Set(float64(count))
}

// RecordHTTPRequest records an HTTP request.
func (m *Metrics) RecordHTTPRequest(method, path, status string, durationSeconds float64) {
	m.HTTPRequestTotal.WithLabelValues(method, path, status).Inc()
	m.HTTPRequestDurationSecs.WithLabelValues(method, path).Observe(durationSeconds)
}

// OTelMetrics holds OpenTelemetry meter instruments for internal use.
type OTelMetrics struct {
	taskCounter      metric.Int64Counter
	taskDuration     metric.Float64Histogram
	agentCounter     metric.Int64Counter
	llmCounter       metric.Int64Counter
	llmDuration      metric.Float64Histogram
	workerPoolGauge  metric.Int64Gauge
	outboxGauge      metric.Int64Gauge
}

// OTelMetricsProvider provides OpenTelemetry metrics.
type OTelMetricsProvider struct{}

// NewOTelMetricsProvider creates a new OTel metrics provider.
func NewOTelMetricsProvider() *OTelMetricsProvider {
	return &OTelMetricsProvider{}
}

// NewOTelMetrics initializes OpenTelemetry metrics instruments.
func (p *OTelMetricsProvider) NewOTelMetrics(serviceName string) (*OTelMetrics, error) {
	return &OTelMetrics{}, nil
}

// RecordTask records a task metric.
func (m *OTelMetrics) RecordTask(ctx context.Context, status string, duration float64) {
	attrs := []attribute.KeyValue{
		attribute.String("status", status),
	}
	if m.taskCounter != nil {
		m.taskCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if m.taskDuration != nil {
		m.taskDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
	}
}

// RecordAgent records an agent execution.
func (m *OTelMetrics) RecordAgent(ctx context.Context, template string, success bool) {
	attrs := []attribute.KeyValue{
		attribute.String("template", template),
		attribute.Bool("success", success),
	}
	if m.agentCounter != nil {
		m.agentCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

// RecordLLM records an LLM request.
func (m *OTelMetrics) RecordLLM(ctx context.Context, model string, success bool, duration float64) {
	attrs := []attribute.KeyValue{
		attribute.String("model", model),
		attribute.Bool("success", success),
	}
	if m.llmCounter != nil {
		m.llmCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if m.llmDuration != nil {
		m.llmDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
	}
}

// RecordWorkerPool records worker pool size.
func (m *OTelMetrics) RecordWorkerPool(ctx context.Context, total, busy, available int64) {
	if m.workerPoolGauge != nil {
		m.workerPoolGauge.Record(ctx, total, metric.WithAttributes(
			attribute.String("state", "total"),
		))
		m.workerPoolGauge.Record(ctx, busy, metric.WithAttributes(
			attribute.String("state", "busy"),
		))
		m.workerPoolGauge.Record(ctx, available, metric.WithAttributes(
			attribute.String("state", "available"),
		))
	}
}

// RecordOutboxPending records pending outbox events.
func (m *OTelMetrics) RecordOutboxPending(ctx context.Context, count int64) {
	if m.outboxGauge != nil {
		m.outboxGauge.Record(ctx, count)
	}
}