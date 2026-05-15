// Package pool provides Worker pool management with prewarming, scaling, and health checks.
package pool

import (
	"context"
	"sync"
	"time"

	"github.com/cloud-agent-platform/cap/internal/worker/sandbox"
	"go.uber.org/zap"
)

// WorkerStatus represents the current state of a worker.
type WorkerStatus string

const (
	WorkerStatusIdle    WorkerStatus = "idle"
	WorkerStatusBusy    WorkerStatus = "busy"
	WorkerStatusStarting WorkerStatus = "starting"
	WorkerStatusStopping WorkerStatus = "stopping"
	WorkerStatusUnhealthy WorkerStatus = "unhealthy"
)

// Worker wraps a Sandbox instance and tracks its state.
type Worker struct {
	id        string
	sandbox   sandbox.Sandbox
	status    WorkerStatus
	createdAt time.Time
	lastUsed  time.Time
	taskCtx   context.Context
	taskCancel context.CancelFunc
	mu        sync.RWMutex
	logger    *zap.Logger
}

// NewWorker creates a new worker with the given sandbox.
func NewWorker(id string, sb sandbox.Sandbox, logger *zap.Logger) *Worker {
	return &Worker{
		id:        id,
		sandbox:   sb,
		status:    WorkerStatusStarting,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		logger:    logger,
	}
}

// ID returns the worker's unique identifier.
func (w *Worker) ID() string {
	return w.id
}

// Status returns the current worker status.
func (w *Worker) Status() WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.status
}

// setStatus sets the worker status.
func (w *Worker) setStatus(status WorkerStatus) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status = status
}

// LastUsed returns the last time the worker was used.
func (w *Worker) LastUsed() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastUsed
}

// UpdateLastUsed updates the last used timestamp.
func (w *Worker) UpdateLastUsed() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastUsed = time.Now()
}

// CreatedAt returns when the worker was created.
func (w *Worker) CreatedAt() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.createdAt
}

// Sandbox returns the underlying sandbox instance.
func (w *Worker) Sandbox() sandbox.Sandbox {
	return w.sandbox
}

// Execute runs a task in the worker.
func (w *Worker) Execute(ctx context.Context, opts sandbox.ExecOptions) (sandbox.ExecResult, error) {
	w.mu.Lock()
	if w.status == WorkerStatusStopping || w.status == WorkerStatusUnhealthy {
		w.mu.Unlock()
		return sandbox.ExecResult{}, ErrWorkerNotAvailable
	}
	w.status = WorkerStatusBusy
	w.taskCtx, w.taskCancel = context.WithCancel(ctx)
	w.lastUsed = time.Now()
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.status = WorkerStatusIdle
		if w.taskCancel != nil {
			w.taskCancel()
		}
		w.mu.Unlock()
	}()

	result, err := w.sandbox.Exec(w.taskCtx, w.id, opts)
	if err != nil {
		w.logger.Warn("worker exec failed",
			zap.String("worker_id", w.id),
			zap.Error(err),
		)
		return result, err
	}

	return result, nil
}

// Stop signals the worker to stop accepting new tasks.
func (w *Worker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.status = WorkerStatusStopping
	if w.taskCancel != nil {
		w.taskCancel()
	}
}

// IsAvailable returns true if the worker can accept new tasks.
func (w *Worker) IsAvailable() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.status == WorkerStatusIdle
}

// IsDraining returns true if the worker is stopping.
func (w *Worker) IsDraining() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.status == WorkerStatusStopping
}

// ErrWorkerNotAvailable is returned when a worker is not available to accept tasks.
var ErrWorkerNotAvailable = &WorkerError{Message: "worker is not available"}

// WorkerError represents a worker-level error.
type WorkerError struct {
	Message string
}

func (e *WorkerError) Error() string {
	return e.Message
}
