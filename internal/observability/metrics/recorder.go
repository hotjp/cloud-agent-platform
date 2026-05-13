// Package metrics provides metrics recording helpers.
package metrics

import (
	"time"
)

// Recorder provides a convenient interface for recording metrics throughout the application.
type Recorder struct {
	metrics *Metrics
}

// NewRecorder creates a new metrics recorder.
func NewRecorder(m *Metrics) *Recorder {
	return &Recorder{metrics: m}
}

// RecordTaskSubmission records a task submission event.
func (r *Recorder) RecordTaskSubmission() {
	r.metrics.RecordTaskSubmitted()
}

// RecordTaskCompletion records a task completion event.
func (r *Recorder) RecordTaskCompletion(duration time.Duration) {
	r.metrics.RecordTaskCompleted(duration.Seconds())
}

// RecordTaskFailure records a task failure event.
func (r *Recorder) RecordTaskFailure(duration time.Duration) {
	r.metrics.RecordTaskFailed(duration.Seconds())
}

// RecordTaskCancellation records a task cancellation event.
func (r *Recorder) RecordTaskCancellation() {
	r.metrics.RecordTaskCancelled()
}

// RecordAgentExecution records an agent execution event.
func (r *Recorder) RecordAgentExecution(template string, success bool) {
	r.metrics.RecordAgentExecution(template, success)
}

// RecordLLMRequest records an LLM request event.
func (r *Recorder) RecordLLMRequest(model string, success bool, duration time.Duration) {
	r.metrics.RecordLLMRequest(model, success, duration.Seconds())
}

// RecordWorkerPool records worker pool size.
func (r *Recorder) RecordWorkerPool(total, busy, available int) {
	r.metrics.RecordWorkerPoolSize(total, busy, available)
}

// RecordOutboxPending records pending outbox events.
func (r *Recorder) RecordOutboxPending(count int64) {
	r.metrics.RecordOutboxEventsPending(count)
}

// RecordHTTPRequest records an HTTP request.
func (r *Recorder) RecordHTTPRequest(method, path, status string, duration time.Duration) {
	r.metrics.RecordHTTPRequest(method, path, status, duration.Seconds())
}

// TaskTimer is a helper for timing task execution.
type TaskTimer struct {
	recorder *Recorder
	start    time.Time
}

// NewTaskTimer starts a new task timer.
func (r *Recorder) NewTaskTimer() *TaskTimer {
	return &TaskTimer{
		recorder: r,
		start:    time.Now(),
	}
}

// Stop stops the timer and records task completion.
func (t *TaskTimer) Stop(success bool) {
	duration := time.Since(t.start)
	if success {
		t.recorder.RecordTaskCompletion(duration)
	} else {
		t.recorder.RecordTaskFailure(duration)
	}
}

// StopWithStatus stops the timer and records task with specific status.
func (t *TaskTimer) StopWithStatus(status TaskStatusLabel) {
	duration := time.Since(t.start)
	switch status {
	case TaskStatusCompleted:
		t.recorder.metrics.TaskTotal.WithLabelValues(string(status)).Inc()
		t.recorder.metrics.TaskDurationSeconds.WithLabelValues(string(status)).Observe(duration.Seconds())
	case TaskStatusFailed:
		t.recorder.metrics.TaskTotal.WithLabelValues(string(status)).Inc()
		t.recorder.metrics.TaskDurationSeconds.WithLabelValues(string(status)).Observe(duration.Seconds())
	case TaskStatusCancelled:
		t.recorder.metrics.TaskTotal.WithLabelValues(string(status)).Inc()
	default:
		t.recorder.metrics.TaskTotal.WithLabelValues(string(status)).Inc()
		t.recorder.metrics.TaskDurationSeconds.WithLabelValues(string(status)).Observe(duration.Seconds())
	}
}

// LLMRequestTimer is a helper for timing LLM requests.
type LLMRequestTimer struct {
	recorder *Recorder
	model    string
	start    time.Time
}

// NewLLMRequestTimer starts a new LLM request timer.
func (r *Recorder) NewLLMRequestTimer(model string) *LLMRequestTimer {
	return &LLMRequestTimer{
		recorder: r,
		model:    model,
		start:    time.Now(),
	}
}

// Stop stops the timer and records LLM request.
func (t *LLMRequestTimer) Stop(success bool) {
	duration := time.Since(t.start)
	t.recorder.RecordLLMRequest(t.model, success, duration)
}

// HTTPRequestTimer is a helper for timing HTTP requests.
type HTTPRequestTimer struct {
	recorder *Recorder
	method   string
	path     string
	start    time.Time
}

// NewHTTPRequestTimer starts a new HTTP request timer.
func (r *Recorder) NewHTTPRequestTimer(method, path string) *HTTPRequestTimer {
	return &HTTPRequestTimer{
		recorder: r,
		method:   method,
		path:     path,
		start:    time.Now(),
	}
}

// Stop stops the timer and records HTTP request.
func (t *HTTPRequestTimer) Stop(status string) {
	duration := time.Since(t.start)
	t.recorder.RecordHTTPRequest(t.method, t.path, status, duration)
}