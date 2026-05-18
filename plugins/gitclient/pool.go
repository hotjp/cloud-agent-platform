// Package gitclient implements Git operations using go-git.
// Provides clone/commit/push without requiring git binary in Worker.
package gitclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// WorkerState represents the lifecycle state of a GitWorker.
type WorkerState int

const (
	// WorkerStateCreated indicates the worker has been created but not yet preheated.
	WorkerStateCreated WorkerState = iota
	// WorkerStatePreheated indicates the worker has been preheated and is ready for use.
	WorkerStatePreheated
	// WorkerStateActive indicates the worker is currently executing an operation.
	WorkerStateActive
	// WorkerStateIdle indicates the worker is idle and available for reuse.
	WorkerStateIdle
	// WorkerStateRecycled indicates the worker has been recycled and resources cleaned up.
	WorkerStateRecycled
)

// String returns a human-readable string for the worker state.
func (s WorkerState) String() string {
	switch s {
	case WorkerStateCreated:
		return "created"
	case WorkerStatePreheated:
		return "preheated"
	case WorkerStateActive:
		return "active"
	case WorkerStateIdle:
		return "idle"
	case WorkerStateRecycled:
		return "recycled"
	default:
		return "unknown"
	}
}

// ErrPoolExhausted is returned when no workers are available and pool is at capacity.
var ErrPoolExhausted = errors.New("worker pool exhausted")

// ErrWorkerNotAvailable is returned when the worker is not in a valid state for the operation.
var ErrWorkerNotAvailable = errors.New("worker not available in required state")

// ErrPoolClosed is returned when operations are attempted on a closed pool.
var ErrPoolClosed = errors.New("worker pool is closed")

// GitWorker wraps a GitClient with lifecycle management for a single worker.
// Each GitWorker manages its own git repository directory.
type GitWorker struct {
	id        string
	state     WorkerState
	client    *GitClient
	repoPath  string // Path to the working directory
	repoURL   string // URL of the cloned repository (if any)
	createdAt time.Time
	lastUsed  time.Time
	useCount  int32 // atomic
	mu        sync.RWMutex
	logger    *zap.Logger
}

// NewGitWorker creates a new GitWorker with the given ID.
func NewGitWorker(id string, logger *zap.Logger) *GitWorker {
	return &GitWorker{
		id:        id,
		state:     WorkerStateCreated,
		client:    New(logger),
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		logger:    logger,
	}
}

// ID returns the unique identifier of this worker.
func (w *GitWorker) ID() string {
	return w.id
}

// State returns the current state of the worker.
func (w *GitWorker) State() WorkerState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state
}

// UseCount returns the number of times this worker has been used.
func (w *GitWorker) UseCount() int32 {
	return atomic.LoadInt32(&w.useCount)
}

// LastUsed returns the last time this worker was used.
func (w *GitWorker) LastUsed() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastUsed
}

// RepoPath returns the path to the worker's repository directory.
func (w *GitWorker) RepoPath() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.repoPath
}

// Client returns the underlying GitClient for operations.
func (w *GitWorker) Client() *GitClient {
	return w.client
}

// transitionState transitions the worker to a new state.
// Returns an error if the transition is not valid.
func (w *GitWorker) transitionState(from, to WorkerState) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state != from {
		return fmt.Errorf("cannot transition from %s to %s: current state is %s",
			from, to, w.state)
	}

	w.logger.Debug("worker state transition",
		zap.String("worker_id", w.id),
		zap.String("from", from.String()),
		zap.String("to", to.String()),
	)

	w.state = to
	return nil
}

// MarkActive marks the worker as actively processing a request.
func (w *GitWorker) MarkActive() error {
	return w.transitionState(WorkerStatePreheated, WorkerStateActive)
}

// MarkIdle marks the worker as idle and available for reuse.
func (w *GitWorker) MarkIdle() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Can transition from Active or Preheated to Idle
	switch w.state {
	case WorkerStateActive, WorkerStatePreheated:
		w.state = WorkerStateIdle
		w.lastUsed = time.Now()
		w.logger.Debug("worker marked idle",
			zap.String("worker_id", w.id),
			zap.Duration("idle_duration", time.Since(w.lastUsed)),
		)
		return nil
	default:
		return fmt.Errorf("cannot mark idle from state %s", w.state)
	}
}

// MarkRecycled marks the worker as recycled and cleans up resources.
func (w *GitWorker) MarkRecycled() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state == WorkerStateRecycled {
		return nil
	}

	// Clean up the repository directory
	if w.repoPath != "" {
		w.logger.Debug("cleaning up worker repo directory",
			zap.String("worker_id", w.id),
			zap.String("repo_path", w.repoPath),
		)
		// Cleanup is delegated to the pool to avoid blocking
	}

	w.state = WorkerStateRecycled
	return nil
}

// Clone clones a repository into this worker's directory.
func (w *GitWorker) Clone(ctx context.Context, opts CloneOptions) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state != WorkerStateCreated && w.state != WorkerStateIdle {
		return fmt.Errorf("%w: cannot clone in state %s", ErrWorkerNotAvailable, w.state)
	}

	// Create a temporary directory for the clone
	tempDir, err := cloneToTempDir(opts.Directory)
	if err != nil {
		return err
	}

	cloneOpts := opts
	cloneOpts.Directory = tempDir

	if err := w.client.Clone(ctx, cloneOpts); err != nil {
		return err
	}

	w.repoPath = tempDir
	w.repoURL = opts.URL
	w.state = WorkerStatePreheated

	return nil
}

// Open opens an existing local repository in this worker's directory.
func (w *GitWorker) Open(ctx context.Context, dir string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state != WorkerStateCreated && w.state != WorkerStateIdle {
		return fmt.Errorf("%w: cannot open in state %s", ErrWorkerNotAvailable, w.state)
	}

	if err := w.client.Open(ctx, dir); err != nil {
		return err
	}

	w.repoPath = dir
	w.state = WorkerStatePreheated

	return nil
}

// Reset resets the worker to the preheated state, clearing the repository.
func (w *GitWorker) Reset() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.client = New(w.logger)
	w.repoPath = ""
	w.repoURL = ""
	w.state = WorkerStateCreated

	return nil
}

// cloneToTempDir creates a temporary directory for cloning.
// If baseDir is provided and is not empty, it will be used as the parent directory.
func cloneToTempDir(baseDir string) (string, error) {
	if baseDir != "" {
		return baseDir, nil
	}
	// Create temp directory for the clone
	tempDir, err := os.MkdirTemp("", "git-clone-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	return tempDir, nil
}

// mkdirTemp creates a temporary directory using os.MkdirTemp.
var mkdirTemp = func(pattern string) (string, error) {
	return os.MkdirTemp("", pattern)
}

// PoolConfig contains configuration for the worker pool.
type PoolConfig struct {
	// InitialSize is the number of workers to pre-create on pool startup.
	InitialSize int
	// MaxSize is the maximum number of workers in the pool.
	MaxSize int
	// MaxIdleTime is the maximum time a worker can be idle before being recycled.
	MaxIdleTime time.Duration
	// HealthCheckInterval is the interval between health checks.
	HealthCheckInterval time.Duration
	// PreheatTimeout is the timeout for preheating a worker.
	PreheatTimeout time.Duration
}

// DefaultPoolConfig returns the default pool configuration.
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		InitialSize:         2,
		MaxSize:             5,
		MaxIdleTime:         5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		PreheatTimeout:      30 * time.Second,
	}
}

// Validate validates the pool configuration.
func (c *PoolConfig) Validate() error {
	if c.InitialSize < 0 {
		return errors.New("initial size must be non-negative")
	}
	if c.MaxSize < 1 {
		return errors.New("max size must be at least 1")
	}
	if c.InitialSize > c.MaxSize {
		return errors.New("initial size cannot exceed max size")
	}
	if c.MaxIdleTime <= 0 {
		return errors.New("max idle time must be positive")
	}
	if c.HealthCheckInterval <= 0 {
		return errors.New("health check interval must be positive")
	}
	if c.PreheatTimeout <= 0 {
		return errors.New("preheat timeout must be positive")
	}
	return nil
}

// WorkerPool manages a pool of GitWorker instances with preheating, reuse, and recycling.
type WorkerPool struct {
	config  PoolConfig
	logger  *zap.Logger

	workers map[string]*GitWorker
	mu      sync.RWMutex
	closed  bool
	started bool

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewWorkerPool creates a new WorkerPool with the given configuration.
func NewWorkerPool(config *PoolConfig, logger *zap.Logger) (*WorkerPool, error) {
	if config == nil {
		config = DefaultPoolConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid pool config: %w", err)
	}

	pool := &WorkerPool{
		config:  *config,
		logger:  logger,
		workers: make(map[string]*GitWorker),
		stopCh:  make(chan struct{}),
	}

	return pool, nil
}

// Start starts the worker pool and preheats initial workers.
func (p *WorkerPool) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrPoolClosed
	}
	if p.started {
		p.mu.Unlock()
		return errors.New("pool already started")
	}
	p.started = true
	p.mu.Unlock()

	p.logger.Info("starting worker pool",
		zap.Int("initial_size", p.config.InitialSize),
		zap.Int("max_size", p.config.MaxSize),
	)

	// Pre-create initial workers
	for i := 0; i < p.config.InitialSize; i++ {
		worker := p.createWorker()
		if err := p.preheatWorker(ctx, worker); err != nil {
			p.logger.Warn("failed to preheat worker",
				zap.String("worker_id", worker.id),
				zap.Error(err),
			)
			// Continue with other workers
		}
	}

	// Start health check goroutine
	p.wg.Add(1)
	go p.runHealthCheck()

	return nil
}

// Stop stops the worker pool and waits for all workers to finish.
func (p *WorkerPool) Stop(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	p.logger.Info("stopping worker pool")

	// Signal health check to stop
	close(p.stopCh)

	// Wait for health check to finish
	p.wg.Wait()

	// Recycle all workers
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, worker := range p.workers {
		if err := worker.MarkRecycled(); err != nil {
			p.logger.Warn("failed to recycle worker",
				zap.String("worker_id", id),
				zap.Error(err),
			)
		}
		cleanupWorkerDir(worker)
	}

	p.workers = make(map[string]*GitWorker)
	p.logger.Info("worker pool stopped")

	return nil
}

// createWorker creates a new worker and adds it to the pool.
func (p *WorkerPool) createWorker() *GitWorker {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.createWorkerLocked()
}

// createWorkerLocked creates a new worker and adds it to the pool.
// Caller must hold p.mu.
func (p *WorkerPool) createWorkerLocked() *GitWorker {
	id := fmt.Sprintf("git-worker-%d", len(p.workers)+1)
	worker := NewGitWorker(id, p.logger)
	worker.state = WorkerStatePreheated // New workers start preheated
	p.workers[id] = worker

	p.logger.Debug("created new worker",
		zap.String("worker_id", id),
		zap.Int("total_workers", len(p.workers)),
	)

	return worker
}

// preheatWorker prepares a worker for use by running a lightweight operation.
func (p *WorkerPool) preheatWorker(ctx context.Context, worker *GitWorker) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Mark worker as preheated - actual git operations are verified on first use
	worker.mu.Lock()
	worker.state = WorkerStatePreheated
	worker.mu.Unlock()

	p.logger.Debug("worker preheated",
		zap.String("worker_id", worker.id),
	)

	return nil
}

// Acquire acquires an idle worker from the pool.
// Returns ErrPoolExhausted if no workers are available and pool is at max capacity.
// Returns ErrPoolClosed if the pool has been stopped.
func (p *WorkerPool) Acquire(ctx context.Context) (*GitWorker, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, ErrPoolClosed
	}
	if !p.started {
		return nil, ErrPoolClosed
	}

	// Find an idle or preheated worker
	var idleWorker *GitWorker
	for _, worker := range p.workers {
		state := worker.State()
		if state == WorkerStateIdle || state == WorkerStatePreheated {
			idleWorker = worker
			break
		}
	}

	// If no idle worker and we're under max size, create a new one
	if idleWorker == nil && len(p.workers) < p.config.MaxSize {
		idleWorker = p.createWorkerLocked()
	}

	if idleWorker == nil {
		return nil, ErrPoolExhausted
	}

	// Mark as active
	if err := idleWorker.transitionState(WorkerStatePreheated, WorkerStateActive); err != nil {
		// If already active, someone else acquired it
		return nil, ErrPoolExhausted
	}

	atomic.AddInt32(&idleWorker.useCount, 1)

	p.logger.Debug("acquired worker",
		zap.String("worker_id", idleWorker.id),
		zap.Int32("use_count", idleWorker.UseCount()),
	)

	return idleWorker, nil
}

// Release releases a worker back to the pool for reuse.
func (p *WorkerPool) Release(worker *GitWorker) {
	if worker == nil {
		return
	}

	if err := worker.MarkIdle(); err != nil {
		p.logger.Warn("failed to release worker to idle state",
			zap.String("worker_id", worker.id),
			zap.Error(err),
		)
		// Worker is in a bad state, remove it
		p.removeWorker(worker.ID())
	}
}

// removeWorker removes a worker from the pool.
func (p *WorkerPool) removeWorker(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if worker, ok := p.workers[id]; ok {
		cleanupWorkerDir(worker)
		delete(p.workers, id)
		p.logger.Debug("removed worker from pool",
			zap.String("worker_id", id),
			zap.Int("remaining_workers", len(p.workers)),
		)
	}
}

// RecycleIdle recycles workers that have been idle for longer than max idle time.
func (p *WorkerPool) RecycleIdle() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	recycled := 0

	for id, worker := range p.workers {
		worker.mu.RLock()
		idleDuration := now.Sub(worker.lastUsed)
		state := worker.state
		worker.mu.RUnlock()

		if state == WorkerStateIdle && idleDuration > p.config.MaxIdleTime {
			if err := worker.MarkRecycled(); err != nil {
				p.logger.Warn("failed to recycle worker",
					zap.String("worker_id", id),
					zap.Error(err),
				)
				continue
			}

			cleanupWorkerDir(worker)
			delete(p.workers, id)
			recycled++

			p.logger.Info("recycled idle worker",
				zap.String("worker_id", id),
				zap.Duration("idle_duration", idleDuration),
			)
		}
	}

	return recycled
}

// runHealthCheck periodically checks worker health and recycles stale workers.
func (p *WorkerPool) runHealthCheck() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			p.logger.Debug("health check stopped")
			return
		case <-ticker.C:
			recycled := p.RecycleIdle()
			if recycled > 0 {
				p.logger.Info("health check recycled idle workers",
					zap.Int("recycled_count", recycled),
				)
			}
		}
	}
}

// Stats returns current pool statistics.
type PoolStats struct {
	TotalWorkers  int            `json:"total_workers"`
	ActiveWorkers int            `json:"active_workers"`
	IdleWorkers   int            `json:"idle_workers"`
	PreheatedWorkers int          `json:"preheated_workers"`
	RecycledWorkers int           `json:"recycled_workers"`
	CreatedWorkers int            `json:"created_workers"`
}

// Stats returns current pool statistics.
func (p *WorkerPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PoolStats{TotalWorkers: len(p.workers)}

	for _, worker := range p.workers {
		switch worker.State() {
		case WorkerStateActive:
			stats.ActiveWorkers++
		case WorkerStateIdle:
			stats.IdleWorkers++
		case WorkerStatePreheated:
			stats.PreheatedWorkers++
		case WorkerStateRecycled:
			stats.RecycledWorkers++
		case WorkerStateCreated:
			stats.CreatedWorkers++
		}
	}

	return stats
}

// cleanupWorkerDir cleans up the worker's repository directory.
func cleanupWorkerDir(worker *GitWorker) {
	if worker == nil {
		return
	}

	worker.mu.RLock()
	repoPath := worker.repoPath
	worker.mu.RUnlock()

	if repoPath != "" {
		if err := os.RemoveAll(repoPath); err != nil {
			worker.logger.Warn("failed to cleanup worker directory",
				zap.String("worker_id", worker.id),
				zap.String("repo_path", repoPath),
				zap.Error(err),
			)
		}
	}
}
