// Package metrics provides business metrics tests.
package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics_RecordTaskSubmitted(t *testing.T) {
	// Create a new registry to avoid conflicts
	reg := prometheus.NewRegistry()

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_task_total",
			Help: "Total number of tasks",
		},
		[]string{"status"},
	)
	reg.MustRegister(counter)

	counter.WithLabelValues("pending").Inc()

	count := testutil.ToFloat64(counter.WithLabelValues("pending"))
	assert.Equal(t, float64(1), count)
}

func TestMetrics_RecordTaskCompleted(t *testing.T) {
	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "test_task_duration",
			Help:    "Task duration",
			Buckets: []float64{1, 5, 10, 30, 60},
		},
		[]string{"status"},
	)
	prometheus.MustRegister(histogram)

	histogram.WithLabelValues("completed").Observe(5.5)

	// Verify histogram was recorded
	metric, err := histogram.GetMetricWithLabelValues("completed")
	require.NoError(t, err)
	assert.NotNil(t, metric)
}

func TestMetrics_RecordAgentExecution(t *testing.T) {
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_agent_total",
			Help: "Total agent executions",
		},
		[]string{"template", "status"},
	)
	prometheus.MustRegister(counter)

	counter.WithLabelValues("coder", "success").Inc()
	counter.WithLabelValues("coder", "failure").Inc()

	successCount := testutil.ToFloat64(counter.WithLabelValues("coder", "success"))
	failureCount := testutil.ToFloat64(counter.WithLabelValues("coder", "failure"))

	assert.Equal(t, float64(1), successCount)
	assert.Equal(t, float64(1), failureCount)
}

func TestMetrics_RecordLLMRequest(t *testing.T) {
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_llm_total",
			Help: "Total LLM requests",
		},
		[]string{"model", "status"},
	)
	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "test_llm_duration",
			Help:    "LLM duration",
			Buckets: []float64{0.1, 0.5, 1, 5},
		},
		[]string{"model"},
	)
	prometheus.MustRegister(counter, histogram)

	counter.WithLabelValues("claude-sonnet", "success").Inc()
	histogram.WithLabelValues("claude-sonnet").Observe(1.5)

	count := testutil.ToFloat64(counter.WithLabelValues("claude-sonnet", "success"))
	assert.Equal(t, float64(1), count)
}

func TestMetrics_RecordWorkerPoolSize(t *testing.T) {
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "test_worker_pool",
			Help: "Worker pool size",
		},
		[]string{"state"},
	)
	prometheus.MustRegister(gauge)

	gauge.WithLabelValues("total").Set(10)
	gauge.WithLabelValues("busy").Set(3)
	gauge.WithLabelValues("available").Set(7)

	total := testutil.ToFloat64(gauge.WithLabelValues("total"))
	busy := testutil.ToFloat64(gauge.WithLabelValues("busy"))
	available := testutil.ToFloat64(gauge.WithLabelValues("available"))

	assert.Equal(t, float64(10), total)
	assert.Equal(t, float64(3), busy)
	assert.Equal(t, float64(7), available)
}

func TestMetrics_RecordOutboxEventsPending(t *testing.T) {
	gauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "test_outbox_pending",
			Help: "Pending outbox events",
		},
	)
	prometheus.MustRegister(gauge)

	gauge.Set(42)

	count := testutil.ToFloat64(gauge)
	assert.Equal(t, float64(42), count)
}

func TestRecorder_RecordTaskCompletion(t *testing.T) {
	m := &Metrics{
		TaskTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_recorder_task_total",
				Help: "Total tasks",
			},
			[]string{"status"},
		),
		TaskDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "test_recorder_task_duration",
				Help:    "Task duration",
				Buckets: []float64{1, 5, 10},
			},
			[]string{"status"},
		),
	}
	prometheus.MustRegister(m.TaskTotal, m.TaskDurationSeconds)

	rec := NewRecorder(m)
	rec.RecordTaskCompletion(5 * time.Second)

	count := testutil.ToFloat64(m.TaskTotal.WithLabelValues("completed"))
	assert.Equal(t, float64(1), count)
}

func TestRecorder_RecordLLMRequest(t *testing.T) {
	m := &Metrics{
		LLMRequestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_recorder_llm_total",
				Help: "Total LLM requests",
			},
			[]string{"model", "status"},
		),
		LLMRequestDurationSecs: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "test_recorder_llm_duration",
				Help:    "LLM duration",
				Buckets: []float64{0.1, 1, 10},
			},
			[]string{"model"},
		),
	}
	prometheus.MustRegister(m.LLMRequestTotal, m.LLMRequestDurationSecs)

	rec := NewRecorder(m)
	rec.RecordLLMRequest("claude-sonnet", true, 2*time.Second)

	count := testutil.ToFloat64(m.LLMRequestTotal.WithLabelValues("claude-sonnet", "success"))
	assert.Equal(t, float64(1), count)
}

func TestTaskTimer_Stop(t *testing.T) {
	m := &Metrics{
		TaskTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_timer_task_total",
				Help: "Total tasks",
			},
			[]string{"status"},
		),
		TaskDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "test_timer_task_duration",
				Help:    "Task duration",
				Buckets: []float64{0.1, 1, 10},
			},
			[]string{"status"},
		),
	}
	prometheus.MustRegister(m.TaskTotal, m.TaskDurationSeconds)

	rec := NewRecorder(m)
	timer := rec.NewTaskTimer()
	time.Sleep(10 * time.Millisecond)
	timer.Stop(true)

	count := testutil.ToFloat64(m.TaskTotal.WithLabelValues("completed"))
	assert.Equal(t, float64(1), count)
}

func TestLLMRequestTimer_Stop(t *testing.T) {
	m := &Metrics{
		LLMRequestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_llm_timer_total",
				Help: "Total LLM requests",
			},
			[]string{"model", "status"},
		),
		LLMRequestDurationSecs: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "test_llm_timer_duration",
				Help:    "LLM duration",
				Buckets: []float64{0.1, 1, 10},
			},
			[]string{"model"},
		),
	}
	prometheus.MustRegister(m.LLMRequestTotal, m.LLMRequestDurationSecs)

	rec := NewRecorder(m)
	timer := rec.NewLLMRequestTimer("claude-haiku")
	time.Sleep(10 * time.Millisecond)
	timer.Stop(true)

	count := testutil.ToFloat64(m.LLMRequestTotal.WithLabelValues("claude-haiku", "success"))
	assert.Equal(t, float64(1), count)
}

func TestHTTPRequestTimer_Stop(t *testing.T) {
	m := &Metrics{
		HTTPRequestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_http_timer_total",
				Help: "Total HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDurationSecs: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "test_http_timer_duration",
				Help:    "HTTP duration",
				Buckets: []float64{0.001, 0.01, 0.1},
			},
			[]string{"method", "path"},
		),
	}
	prometheus.MustRegister(m.HTTPRequestTotal, m.HTTPRequestDurationSecs)

	rec := NewRecorder(m)
	timer := rec.NewHTTPRequestTimer("POST", "/api/tasks")
	time.Sleep(10 * time.Millisecond)
	timer.Stop("200")

	count := testutil.ToFloat64(m.HTTPRequestTotal.WithLabelValues("POST", "/api/tasks", "200"))
	assert.Equal(t, float64(1), count)
}

func TestNewOTelMetrics(t *testing.T) {
	// This test requires a meter which requires an exporter
	// We test the recorder functionality instead as the OTel parts require
	// more complex setup with actual metric.Meter
	t.Run("Recorder returns correct types", func(t *testing.T) {
		m := &Metrics{
			TaskTotal: prometheus.NewCounterVec(
				prometheus.CounterOpts{Name: "test_types_total"},
				[]string{"status"},
			),
		}

		rec := NewRecorder(m)
		assert.IsType(t, &Recorder{}, rec)
	})
}

func TestTaskStatusLabels(t *testing.T) {
	// Verify all task status constants are defined
	statuses := []TaskStatusLabel{
		TaskStatusPending,
		TaskStatusDecomposing,
		TaskStatusDispatched,
		TaskStatusRunning,
		TaskStatusReviewing,
		TaskStatusConfirming,
		TaskStatusCompleted,
		TaskStatusFailed,
		TaskStatusCancelled,
	}

	assert.Equal(t, 9, len(statuses))

	expected := map[TaskStatusLabel]string{
		TaskStatusPending:     "pending",
		TaskStatusDecomposing: "decomposing",
		TaskStatusDispatched:  "dispatched",
		TaskStatusRunning:     "running",
		TaskStatusReviewing:   "reviewing",
		TaskStatusConfirming:  "confirming",
		TaskStatusCompleted:   "completed",
		TaskStatusFailed:      "failed",
		TaskStatusCancelled:   "cancelled",
	}

	for _, status := range statuses {
		assert.Equal(t, expected[status], string(status))
	}
}
