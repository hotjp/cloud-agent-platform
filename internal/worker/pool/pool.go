// Package pool provides Worker pool management with prewarming, scaling, and health checks.
package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloud-agent-platform/cap/internal/worker/sandbox"
	"go.uber.org/zap"
)

// Pool manages a pool of Workers with prewarming, scaling, and health checks.
type Pool struct {
	cfg     Config
	sb      sandbox.Sandbox
	logger  *zap.Logger
	workers map[string]*Worker
	mu      sync.RWMutex
	wg      sync.WaitGroup

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// Queue metrics
	pendingTasks int64

	// State
	running atomic.Bool
}

// NewPool creates a new Worker pool.
func NewPool(cfg Config, sb sandbox.Sandbox, logger *zap.Logger) (*Pool, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid pool config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		cfg:      cfg,
		sb:       sb,
		logger:   logger,
		workers:  make(map[string]*Worker),
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
	}

	return p, nil
}

// Start initializes the pool with prewarmed workers and starts background goroutines.
func (p *Pool) Start(ctx context.Context) error {
	if p.running.Load() {
		return ErrPoolAlreadyRunning
	}

	p.running.Store(true)

	// Prewarm: create minimum number of workers
	if err := p.prewarm(ctx); err != nil {
		p.logger.Error("failed to prewarm workers", zap.Error(err))
		// Continue anyway - workers will be created on demand
	}

	// Start background goroutines
	p.wg.Add(1)
	go p.scaleLoop()
	p.wg.Add(1)
	go p.healthLoop()
	p.wg.Add(1)
	go p.cleanupLoop()

	p.logger.Info("worker pool started",
		zap.Int("min_workers", p.cfg.MinWorkers),
		zap.Int("max_workers", p.cfg.MaxWorkers),
	)

	return nil
}

// prewarm creates the minimum number of workers.
func (p *Pool) prewarm(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	created := 0
	for i := 0; i < p.cfg.MinWorkers; i++ {
		w, err := p.createWorker(ctx)
		if err != nil {
			p.logger.Warn("failed to prewarm worker",
				zap.Int("attempt", i+1),
				zap.Error(err),
			)
			continue
		}
		p.workers[w.ID()] = w
		created++
	}

	p.logger.Info("prewarmed workers", zap.Int("count", created))
	return nil
}

// createWorker creates a new worker with a sandbox.
func (p *Pool) createWorker(ctx context.Context) (*Worker, error) {
	opts := sandbox.SandboxOptions{
		Image:            p.cfg.SandboxOpts.Image,
		WorkingDir:       p.cfg.SandboxOpts.WorkingDir,
		Envvars:          p.cfg.SandboxOpts.Envvars,
		CPUPeriod:        p.cfg.SandboxOpts.CPUPeriod,
		CPUQuota:         p.cfg.SandboxOpts.CPUQuota,
		MemoryLimit:      p.cfg.SandboxOpts.MemoryLimit,
		NetworkDisabled:  p.cfg.SandboxOpts.NetworkDisabled,
		ReadonlyRootfs:   p.cfg.SandboxOpts.ReadonlyRootfs,
		Timeout:          p.cfg.SandboxOpts.Timeout,
	}

	id, err := p.sb.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}

	w := NewWorker(id, p.sb, p.logger)
	w.setStatus(WorkerStatusIdle)

	p.logger.Debug("worker created", zap.String("worker_id", id))
	return w, nil
}

// Acquire gets an available worker, creating one if necessary and under max limit.
func (p *Pool) Acquire(ctx context.Context) (*Worker, error) {
	if !p.running.Load() {
		return nil, ErrPoolNotRunning
	}

	// Fast path: find an available worker
	p.mu.RLock()
	for _, w := range p.workers {
		if w.IsAvailable() {
			w.setStatus(WorkerStatusBusy)
			p.mu.RUnlock()
			return w, nil
		}
	}
	p.mu.RUnlock()

	// Need to create a new worker
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	for _, w := range p.workers {
		if w.IsAvailable() {
			w.setStatus(WorkerStatusBusy)
			return w, nil
		}
	}

	// Check if we can create more workers
	if len(p.workers) >= p.cfg.MaxWorkers {
		return nil, ErrPoolExhausted
	}

	// Create new worker - this is done without context timeout to allow creation
	createCtx, cancel := context.WithTimeout(context.Background(), p.cfg.SandboxOpts.Timeout)
	defer cancel()

	w, err := p.createWorker(createCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create worker: %w", err)
	}

	w.setStatus(WorkerStatusBusy)
	p.workers[w.ID()] = w
	return w, nil
}

// Release returns a worker to the pool (workers are kept in pool after use).
func (p *Pool) Release(w *Worker) {
	w.setStatus(WorkerStatusIdle)
	p.logger.Debug("worker released", zap.String("worker_id", w.ID()))
}

// DestroyWorker removes a worker from the pool and destroys its sandbox.
func (p *Pool) DestroyWorker(ctx context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	w, ok := p.workers[id]
	if !ok {
		return ErrWorkerNotFound
	}

	// Prevent new task assignment
	w.Stop()

	// Destroy sandbox
	if err := p.sb.Destroy(ctx, id); err != nil {
		p.logger.Warn("failed to destroy sandbox",
			zap.String("worker_id", id),
			zap.Error(err),
		)
	}

	delete(p.workers, id)
	p.logger.Info("worker destroyed", zap.String("worker_id", id))
	return nil
}

// SubmitTask submits a task to be executed by an available worker.
func (p *Pool) SubmitTask(ctx context.Context, opts sandbox.ExecOptions) (sandbox.ExecResult, error) {
	atomic.AddInt64(&p.pendingTasks, 1)
	defer atomic.AddInt64(&p.pendingTasks, -1)

	worker, err := p.Acquire(ctx)
	if err != nil {
		return sandbox.ExecResult{}, err
	}

	result, err := worker.Execute(ctx, opts)
	if err != nil {
		// If execution fails, destroy the worker as it may be unhealthy
		p.logger.Warn("task execution failed, removing worker",
			zap.String("worker_id", worker.ID()),
			zap.Error(err),
		)
		_ = p.DestroyWorker(ctx, worker.ID())
		return result, err
	}

	return result, nil
}

// GetWorker returns a worker by ID.
func (p *Pool) GetWorker(id string) (*Worker, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	w, ok := p.workers[id]
	return w, ok
}

// WorkerCount returns the current number of workers.
func (p *Pool) WorkerCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.workers)
}

// AvailableWorkerCount returns the number of available workers.
func (p *Pool) AvailableWorkerCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	count := 0
	for _, w := range p.workers {
		if w.IsAvailable() {
			count++
		}
	}
	return count
}

// PendingTaskCount returns the number of pending tasks.
func (p *Pool) PendingTaskCount() int {
	return int(atomic.LoadInt64(&p.pendingTasks))
}

// Stats returns current pool statistics.
type PoolStats struct {
	TotalWorkers     int
	AvailableWorkers int
	BusyWorkers     int
	PendingTasks    int
}

// Stats returns current pool statistics.
func (p *Pool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PoolStats{
		TotalWorkers:  len(p.workers),
		PendingTasks: p.PendingTaskCount(),
	}

	for _, w := range p.workers {
		if w.IsAvailable() {
			stats.AvailableWorkers++
		} else if w.Status() == WorkerStatusBusy {
			stats.BusyWorkers++
		}
	}

	return stats
}

// Stop gracefully shuts down the pool.
func (p *Pool) Stop(ctx context.Context) error {
	if !p.running.Load() {
		return nil
	}

	p.logger.Info("stopping worker pool...")
	p.running.Store(false)

	// Cancel context to stop background goroutines
	p.cancel()

	// Signal done
	close(p.done)

	// Wait for background goroutines with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.logger.Info("background goroutines stopped")
	case <-ctx.Done():
		p.logger.Warn("timeout waiting for background goroutines")
	}

	// Stop all workers gracefully
	p.mu.Lock()
	var wg sync.WaitGroup
	for id, w := range p.workers {
		wg.Add(1)
		go func(id string, w *Worker) {
			defer wg.Done()
			// Wait briefly for current task to complete
			timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// Worker will finish current task on its own, just destroy when done
			_ = p.sb.Destroy(timeout, id)
		}(id, w)
	}
	p.mu.Unlock()

	// Wait for worker destruction with shutdown timeout
	select {
	case <-done:
	case <-time.After(p.cfg.ShutdownTimeout):
		p.logger.Warn("timeout during worker cleanup, forcing shutdown")
	}

	p.logger.Info("worker pool stopped")
	return nil
}

// scaleLoop periodically checks queue length and scales the pool.
func (p *Pool) scaleLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.cfg.ScaleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.scale(p.ctx)
		}
	}
}

// scale adjusts the pool size based on pending tasks.
func (p *Pool) scale(ctx context.Context) {
	pending := atomic.LoadInt64(&p.pendingTasks)

	p.mu.RLock()
	currentCount := len(p.workers)
	p.mu.RUnlock()

	var targetCount int

	if pending >= int64(p.cfg.ScaleUpThreshold) && currentCount < p.cfg.MaxWorkers {
		// Scale up
		targetCount = currentCount + p.cfg.ScaleUpStep
		if targetCount > p.cfg.MaxWorkers {
			targetCount = p.cfg.MaxWorkers
		}
		delta := targetCount - currentCount

		p.mu.Lock()
		for i := 0; i < delta && len(p.workers) < p.cfg.MaxWorkers; i++ {
			w, err := p.createWorker(ctx)
			if err != nil {
				p.logger.Warn("failed to scale up", zap.Error(err))
				break
			}
			p.workers[w.ID()] = w
		}
		p.mu.Unlock()

		if delta > 0 {
			p.logger.Info("scaled up workers",
				zap.Int("delta", delta),
				zap.Int("new_count", targetCount),
			)
		}

	} else if pending <= int64(p.cfg.ScaleDownThreshold) && currentCount > p.cfg.MinWorkers {
		// Scale down
		targetCount = currentCount - p.cfg.ScaleDownStep
		if targetCount < p.cfg.MinWorkers {
			targetCount = p.cfg.MinWorkers
		}
		delta := currentCount - targetCount

		// Find idle workers to remove
		p.mu.Lock()
		removed := 0
		for id, w := range p.workers {
			if removed >= delta {
				break
			}
			if w.IsAvailable() {
				w.Stop()
				go func(id string) {
					timeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = p.sb.Destroy(timeout, id)
				}(id)
				delete(p.workers, id)
				removed++
			}
		}
		p.mu.Unlock()

		if removed > 0 {
			p.logger.Info("scaled down workers",
				zap.Int("delta", -removed),
				zap.Int("new_count", targetCount),
			)
		}
	}
}

// healthLoop periodically checks worker health.
func (p *Pool) healthLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.cfg.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.checkHealth(p.ctx)
		}
	}
}

// checkHealth checks the health of all workers.
func (p *Pool) checkHealth(ctx context.Context) {
	p.mu.RLock()
	workers := make([]*Worker, 0, len(p.workers))
	for _, w := range p.workers {
		workers = append(workers, w)
	}
	p.mu.RUnlock()

	for _, w := range workers {
		select {
		case <-ctx.Done():
			return
		default:
		}

		healthy, err := p.checkWorkerHealth(ctx, w)
		if !healthy || err != nil {
			p.logger.Warn("unhealthy worker detected, replacing",
				zap.String("worker_id", w.ID()),
				zap.Error(err),
			)

			// Create replacement worker
			p.mu.Lock()
			newWorker, createErr := p.createWorker(ctx)
			if createErr != nil {
				p.logger.Error("failed to create replacement worker",
					zap.String("worker_id", w.ID()),
					zap.Error(createErr),
				)
				p.mu.Unlock()
				continue
			}
			p.workers[newWorker.ID()] = newWorker
			p.mu.Unlock()

			// Destroy unhealthy worker
			go func(id string) {
				destroyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = p.DestroyWorker(destroyCtx, id)
			}(w.ID())
		}
	}
}

// checkWorkerHealth performs a health check on a worker.
func (p *Pool) checkWorkerHealth(ctx context.Context, w *Worker) (bool, error) {
	// Check if worker is responsive by querying its status
	checkCtx, cancel := context.WithTimeout(ctx, p.cfg.HealthCheckTimeout)
	defer cancel()

	status, err := w.Sandbox().Status(checkCtx, w.ID())
	if err != nil {
		return false, fmt.Errorf("status check failed: %w", err)
	}

	if status != sandbox.StatusRunning {
		return false, fmt.Errorf("sandbox status is %s", status)
	}

	// Check if worker has a stuck task
	if w.Status() == WorkerStatusBusy {
		lastUsed := w.LastUsed()
		if time.Since(lastUsed) > p.cfg.MaxTaskDuration {
			return false, fmt.Errorf("task appears stuck (running for %v)", time.Since(lastUsed))
		}
	}

	return true, nil
}

// cleanupLoop periodically removes idle workers above MinWorkers.
func (p *Pool) cleanupLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.cleanup(p.ctx)
		}
	}
}

// cleanup removes idle workers above the minimum.
func (p *Pool) cleanup(ctx context.Context) {
	p.mu.RLock()
	if len(p.workers) <= p.cfg.MinWorkers {
		p.mu.RUnlock()
		return
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.workers) <= p.cfg.MinWorkers {
		return
	}

	now := time.Now()
	for id, w := range p.workers {
		if !w.IsAvailable() {
			continue
		}
		// Only remove workers that have been idle for longer than TTL
		if now.Sub(w.LastUsed()) > p.cfg.IdleWorkerTTL {
			w.Stop()
			go func(id string) {
				timeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = p.sb.Destroy(timeout, id)
			}(id)
			delete(p.workers, id)
			p.logger.Debug("cleaned up idle worker", zap.String("worker_id", id))
		}

		// Stop if we've reached minimum
		if len(p.workers) <= p.cfg.MinWorkers {
			break
		}
	}
}

// ErrPoolAlreadyRunning is returned when Start is called on a running pool.
var ErrPoolAlreadyRunning = errors.New("pool is already running")

// ErrPoolNotRunning is returned when operating on a stopped pool.
var ErrPoolNotRunning = errors.New("pool is not running")

// ErrPoolExhausted is returned when all workers are busy and max has been reached.
var ErrPoolExhausted = errors.New("pool is exhausted (max workers busy)")

// ErrWorkerNotFound is returned when a worker is not found.
var ErrWorkerNotFound = errors.New("worker not found")
