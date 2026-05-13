// Package workermanager implements Worker lifecycle management.
// Supports on-demand worker creation and destruction based on task load.
package workermanager

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain/worker"
	"go.uber.org/zap"
)

// Config contains configuration for the WorkerManager.
type Config struct {
	// MaxWorkers is the maximum number of concurrent workers.
	MaxWorkers int
	// IdleTimeout is how long a worker can be idle before being destroyed.
	IdleTimeout time.Duration
	// HealthCheckInterval is how often to check worker health.
	HealthCheckInterval time.Duration
	// HealthCheckTimeout is the timeout for each health check.
	HealthCheckTimeout time.Duration
	// CreationTimeout is the timeout for creating a new sandbox.
	CreationTimeout time.Duration
	// DestroyTimeout is the timeout for destroying a sandbox.
	DestroyTimeout time.Duration
	// DefaultSandboxOpts contains default sandbox options.
	DefaultSandboxOpts worker.SandboxOptions
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		MaxWorkers:           10,
		IdleTimeout:          5 * time.Minute,
		HealthCheckInterval:  30 * time.Second,
		HealthCheckTimeout:   5 * time.Second,
		CreationTimeout:      30 * time.Second,
		DestroyTimeout:       10 * time.Second,
		DefaultSandboxOpts: worker.SandboxOptions{
			Image: "alpine:latest",
			Timeout: 30 * time.Second,
		},
	}
}

// Validate validates the configuration.
func (c Config) Validate() error {
	if c.MaxWorkers <= 0 {
		return fmt.Errorf("max_workers must be positive")
	}
	if c.IdleTimeout <= 0 {
		return fmt.Errorf("idle_timeout must be positive")
	}
	if c.DefaultSandboxOpts.Image == "" {
		return fmt.Errorf("default sandbox image is required")
	}
	return nil
}

// workerState tracks a worker's current state.
type workerState struct {
	id        string
	status    workerStatus
	createdAt time.Time
	lastUsed   time.Time
}

// workerStatus represents the current state of a managed worker.
type workerStatus string

const (
	workerStatusIdle     workerStatus = "idle"
	workerStatusBusy     workerStatus = "busy"
	workerStatusStopping workerStatus = "stopping"
)

// WorkerManager manages Worker lifecycle with on-demand creation and destruction.
type WorkerManager struct {
	cfg     Config
	backend worker.SandboxBackend
	logger  *zap.Logger

	mu      sync.RWMutex
	workers map[string]*workerState

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	running atomic.Bool
}

// New creates a new WorkerManager instance.
func New(cfg Config, backend worker.SandboxBackend, logger *zap.Logger) (*WorkerManager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid worker manager config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wm := &WorkerManager{
		cfg:     cfg,
		backend: backend,
		logger:  logger,
		workers: make(map[string]*workerState),
		ctx:     ctx,
		cancel:  cancel,
	}

	return wm, nil
}

// Start starts the worker manager and its background goroutines.
func (wm *WorkerManager) Start() error {
	if wm.running.Load() {
		return fmt.Errorf("worker manager is already running")
	}

	wm.running.Store(true)

	wm.wg.Add(1)
	go wm.idleCleanupLoop()

	wm.wg.Add(1)
	go wm.healthCheckLoop()

	wm.logger.Info("worker manager started",
		zap.Int("max_workers", wm.cfg.MaxWorkers),
		zap.Duration("idle_timeout", wm.cfg.IdleTimeout),
	)

	return nil
}

// Stop gracefully shuts down the worker manager.
func (wm *WorkerManager) Stop(ctx context.Context) error {
	if !wm.running.Load() {
		return nil
	}

	wm.logger.Info("stopping worker manager...")
	wm.running.Store(false)

	wm.cancel()

	done := make(chan struct{})
	go func() {
		wm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		wm.logger.Info("worker manager stopped")
	case <-ctx.Done():
		wm.logger.Warn("timeout waiting for worker manager to stop")
	}

	// Destroy all remaining workers
	wm.mu.Lock()
	var destroyWg sync.WaitGroup
	for id := range wm.workers {
		destroyWg.Add(1)
		go func(id string) {
			defer destroyWg.Done()
			destroyCtx, cancel := context.WithTimeout(context.Background(), wm.cfg.DestroyTimeout)
			defer cancel()
			_ = wm.backend.Destroy(destroyCtx, id)
		}(id)
	}
	wm.mu.Unlock()

	destroyWg.Wait()
	wm.mu.Lock()
	wm.workers = make(map[string]*workerState)
	wm.mu.Unlock()

	return nil
}

// AcquireWorker acquires an available worker, creating one on-demand if needed.
func (wm *WorkerManager) AcquireWorker(ctx context.Context, opts worker.SandboxOptions) (string, error) {
	if !wm.running.Load() {
		return "", fmt.Errorf("worker manager is not running")
	}

	// Merge provided options with defaults
	if opts.Image == "" {
		opts.Image = wm.cfg.DefaultSandboxOpts.Image
	}
	if opts.Timeout == 0 {
		opts.Timeout = wm.cfg.DefaultSandboxOpts.Timeout
	}

	// Fast path: find an available worker
	wm.mu.RLock()
	for id, w := range wm.workers {
		if w.status == workerStatusIdle {
			w.status = workerStatusBusy
			w.lastUsed = time.Now()
			wm.mu.RUnlock()
			wm.logger.Debug("acquired idle worker",
				zap.String("worker_id", id),
			)
			return id, nil
		}
	}
	wm.mu.RUnlock()

	// Need to create a new worker
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Double-check after acquiring write lock
	for id, w := range wm.workers {
		if w.status == workerStatusIdle {
			w.status = workerStatusBusy
			w.lastUsed = time.Now()
			return id, nil
		}
	}

	// Check if we've reached the limit
	if len(wm.workers) >= wm.cfg.MaxWorkers {
		return "", fmt.Errorf("worker limit reached (%d)", wm.cfg.MaxWorkers)
	}

	// Create a new worker
	createCtx, cancel := context.WithTimeout(ctx, wm.cfg.CreationTimeout)
	defer cancel()

	id, err := wm.backend.Create(createCtx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to create worker: %w", err)
	}

	wm.workers[id] = &workerState{
		id:        id,
		status:    workerStatusBusy,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}

	wm.logger.Info("created new worker",
		zap.String("worker_id", id),
		zap.Int("total_workers", len(wm.workers)),
	)

	return id, nil
}

// ReleaseWorker releases a worker back to the idle pool.
func (wm *WorkerManager) ReleaseWorker(id string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	w, ok := wm.workers[id]
	if !ok {
		wm.logger.Warn("attempted to release unknown worker",
			zap.String("worker_id", id),
		)
		return
	}

	if w.status == workerStatusStopping {
		return
	}

	w.status = workerStatusIdle
	w.lastUsed = time.Now()

	wm.logger.Debug("released worker",
		zap.String("worker_id", id),
	)
}

// DestroyWorker immediately destroys a worker.
func (wm *WorkerManager) DestroyWorker(ctx context.Context, id string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	w, ok := wm.workers[id]
	if !ok {
		return fmt.Errorf("worker not found: %s", id)
	}

	w.status = workerStatusStopping

	if err := wm.backend.Destroy(ctx, id); err != nil {
		w.status = workerStatusIdle
		return fmt.Errorf("failed to destroy worker: %w", err)
	}

	delete(wm.workers, id)

	wm.logger.Info("destroyed worker",
		zap.String("worker_id", id),
		zap.Int("remaining_workers", len(wm.workers)),
	)

	return nil
}

// Execute executes a task in the specified worker.
func (wm *WorkerManager) Execute(ctx context.Context, workerID string, opts worker.ExecOptions) (worker.ExecResult, error) {
	wm.mu.RLock()
	w, ok := wm.workers[workerID]
	wm.mu.RUnlock()

	if !ok {
		return worker.ExecResult{}, fmt.Errorf("worker not found: %s", workerID)
	}

	if w.status == workerStatusStopping {
		return worker.ExecResult{}, fmt.Errorf("worker is stopping")
	}

	result, err := wm.backend.Exec(ctx, workerID, opts)
	if err != nil {
		// Mark worker for destruction on next cleanup
		wm.logger.Warn("worker execution failed",
			zap.String("worker_id", workerID),
			zap.Error(err),
		)
		return result, err
	}

	return result, nil
}

// GetWorkerStatus returns the status of a worker.
func (wm *WorkerManager) GetWorkerStatus(ctx context.Context, id string) (worker.SandboxStatus, error) {
	return wm.backend.Status(ctx, id)
}

// GetWorkerHealth returns the health status of a worker.
func (wm *WorkerManager) GetWorkerHealth(ctx context.Context, id string) (worker.HealthStatus, error) {
	return wm.backend.HealthCheck(ctx, id)
}

// WorkerStats contains statistics about managed workers.
type WorkerStats struct {
	TotalWorkers    int
	IdleWorkers     int
	BusyWorkers     int
	StoppingWorkers int
}

// Stats returns current worker statistics.
func (wm *WorkerManager) Stats() WorkerStats {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	stats := WorkerStats{
		TotalWorkers: len(wm.workers),
	}

	for _, w := range wm.workers {
		switch w.status {
		case workerStatusIdle:
			stats.IdleWorkers++
		case workerStatusBusy:
			stats.BusyWorkers++
		case workerStatusStopping:
			stats.StoppingWorkers++
		}
	}

	return stats
}

// idleCleanupLoop periodically cleans up idle workers beyond the limit.
func (wm *WorkerManager) idleCleanupLoop() {
	defer wm.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-wm.ctx.Done():
			return
		case <-ticker.C:
			wm.cleanupIdleWorkers()
		}
	}
}

// cleanupIdleWorkers removes workers that have been idle too long.
func (wm *WorkerManager) cleanupIdleWorkers() {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	now := time.Now()
	var toDestroy []string

	for id, w := range wm.workers {
		if w.status == workerStatusIdle && now.Sub(w.lastUsed) > wm.cfg.IdleTimeout {
			w.status = workerStatusStopping
			toDestroy = append(toDestroy, id)
		}
	}

	if len(toDestroy) > 0 {
		wm.logger.Info("cleaning up idle workers",
			zap.Int("count", len(toDestroy)),
		)
	}

	// Destroy workers outside the lock to avoid deadlock
	for _, id := range toDestroy {
		go func(workerID string) {
			destroyCtx, cancel := context.WithTimeout(context.Background(), wm.cfg.DestroyTimeout)
			defer cancel()
			if err := wm.backend.Destroy(destroyCtx, workerID); err != nil {
				wm.logger.Warn("failed to destroy idle worker",
					zap.String("worker_id", workerID),
					zap.Error(err),
				)
			}
			wm.mu.Lock()
			delete(wm.workers, workerID)
			wm.mu.Unlock()
		}(id)
	}
}

// healthCheckLoop periodically checks worker health.
func (wm *WorkerManager) healthCheckLoop() {
	defer wm.wg.Done()

	ticker := time.NewTicker(wm.cfg.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-wm.ctx.Done():
			return
		case <-ticker.C:
			wm.checkWorkerHealth()
		}
	}
}

// checkWorkerHealth checks the health of all workers.
func (wm *WorkerManager) checkWorkerHealth() {
	wm.mu.RLock()
	workerIDs := make([]string, 0, len(wm.workers))
	for id, w := range wm.workers {
		if w.status != workerStatusStopping {
			workerIDs = append(workerIDs, id)
		}
	}
	wm.mu.RUnlock()

	for _, id := range workerIDs {
		select {
		case <-wm.ctx.Done():
			return
		default:
		}

		wm.checkAndReplaceWorker(id)
	}
}

// checkAndReplaceWorker checks a worker's health and replaces it if unhealthy.
func (wm *WorkerManager) checkAndReplaceWorker(id string) {
	checkCtx, cancel := context.WithTimeout(context.Background(), wm.cfg.HealthCheckTimeout)
	defer cancel()

	health, err := wm.backend.HealthCheck(checkCtx, id)
	if err != nil {
		wm.logger.Warn("health check failed for worker",
			zap.String("worker_id", id),
			zap.Error(err),
		)
		wm.markWorkerUnhealthy(id)
		return
	}

	if !health.IsHealthy {
		wm.logger.Warn("worker is unhealthy",
			zap.String("worker_id", id),
			zap.String("message", health.Message),
		)
		wm.markWorkerUnhealthy(id)
	}
}

// markWorkerUnhealthy marks a worker as unhealthy for replacement.
func (wm *WorkerManager) markWorkerUnhealthy(id string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	w, ok := wm.workers[id]
	if !ok || w.status == workerStatusStopping {
		return
	}

	// Remove the unhealthy worker so a new one can be created on next Acquire
	w.status = workerStatusStopping

	wm.logger.Info("marked worker for replacement",
		zap.String("worker_id", id),
	)

	// Schedule for destruction
	go func(workerID string) {
		destroyCtx, cancel := context.WithTimeout(context.Background(), wm.cfg.DestroyTimeout)
		defer cancel()
		if err := wm.backend.Destroy(destroyCtx, workerID); err != nil {
			wm.logger.Warn("failed to destroy unhealthy worker",
				zap.String("worker_id", workerID),
				zap.Error(err),
			)
		}
		wm.mu.Lock()
		delete(wm.workers, workerID)
		wm.mu.Unlock()
	}(id)
}
