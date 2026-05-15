// Package scheduler - Docker-based cold start implementation.
package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"go.uber.org/zap"
)

// coldStartScheduler is the Phase 1 implementation.
// Every Acquire creates a new container, every Release destroys it.
type coldStartScheduler struct {
	cfg    Config
	backend Backend
	logger  *zap.Logger

	mu         sync.Mutex
	containers map[string]containerHandle // id → handle

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	running bool
}

// Config holds scheduler configuration.
type Config struct {
	// MaxContainers caps the total number of concurrent containers.
	MaxContainers int
	// CreateTimeout for container creation.
	CreateTimeout time.Duration
	// DestroyTimeout for container destruction.
	DestroyTimeout time.Duration
	// IdleTimeout before destroying idle containers (Phase 2 hint).
	IdleTimeout time.Duration
	// HealthCheckInterval for background health checks.
	HealthCheckInterval time.Duration
	// DefaultSpec is the default ContainerSpec if caller provides empty fields.
	DefaultSpec ContainerSpec
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxContainers:       10,
		CreateTimeout:       30 * time.Second,
		DestroyTimeout:      10 * time.Second,
		IdleTimeout:         10 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		DefaultSpec: ContainerSpec{
			Image:           "alpine:latest",
			WorkingDir:      "/workspace",
			MemoryLimit:     2 * 1024 * 1024 * 1024, // 2GB
			CPUQuota:        100000,                  // 1 CPU
			NetworkDisabled: false,
			Timeout:         30 * time.Minute,
		},
	}
}

// Validate checks config values.
func (c Config) Validate() error {
	if c.MaxContainers <= 0 {
		return fmt.Errorf("scheduler: MaxContainers must be > 0")
	}
	return nil
}

// containerHandle tracks a managed container.
type containerHandle struct {
	id        string
	spec      ContainerSpec
	createdAt time.Time
	lastUsed  time.Time
	closed    bool
}

// New creates a new cold-start scheduler.
func New(cfg Config, backend Backend, logger *zap.Logger) (Scheduler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if backend == nil {
		return nil, fmt.Errorf("scheduler: backend is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &coldStartScheduler{
		cfg:        cfg,
		backend:    backend,
		logger:     logger,
		containers: make(map[string]containerHandle),
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Start begins background maintenance goroutines.
func (s *coldStartScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.idleCleanupLoop()

	s.logger.Info("scheduler started",
		zap.Int("max_containers", s.cfg.MaxContainers),
	)
	return nil
}

// Stop destroys all managed containers and stops background goroutines.
func (s *coldStartScheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	s.cancel()
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}

	// Destroy all remaining containers
	s.mu.Lock()
	ids := make([]string, 0, len(s.containers))
	for id := range s.containers {
		ids = append(ids, id)
	}
	s.mu.Unlock()

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(containerID string) {
			defer wg.Done()
			destroyCtx, cancel := context.WithTimeout(context.Background(), s.cfg.DestroyTimeout)
			defer cancel()
			if err := s.backend.Destroy(destroyCtx, containerID); err != nil {
				s.logger.Warn("failed to destroy container during shutdown",
					zap.String("id", containerID),
					zap.Error(err),
				)
			}
		}(id)
	}
	wg.Wait()

	s.mu.Lock()
	s.containers = make(map[string]containerHandle)
	s.mu.Unlock()

	s.logger.Info("scheduler stopped")
	return nil
}

// Acquire creates a new container matching the spec.
func (s *coldStartScheduler) Acquire(ctx context.Context, spec ContainerSpec) (Container, error) {
	spec = s.mergeDefaults(spec)

	s.mu.Lock()
	if len(s.containers) >= s.cfg.MaxContainers {
		s.mu.Unlock()
		return nil, fmt.Errorf("scheduler: container limit reached (%d)", s.cfg.MaxContainers)
	}
	s.mu.Unlock()

	createCtx, cancel := context.WithTimeout(ctx, s.cfg.CreateTimeout)
	defer cancel()

	id, err := s.backend.Create(createCtx, spec)
	if err != nil {
		return nil, fmt.Errorf("scheduler: create failed: %w", err)
	}

	handle := containerHandle{
		id:        id,
		spec:      spec,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}

	s.mu.Lock()
	s.containers[id] = handle
	s.mu.Unlock()

	s.logger.Info("container acquired",
		zap.String("id", id),
		zap.String("image", spec.Image),
		zap.Int("total", len(s.containers)),
	)

	return &managedContainer{
		id:      id,
		sched:   s,
		spec:    spec,
		backend: s.backend,
		logger:  s.logger,
	}, nil
}

// Release destroys a container.
func (s *coldStartScheduler) Release(ctx context.Context, id string) error {
	s.mu.Lock()
	handle, ok := s.containers[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("scheduler: container %s not found", id)
	}
	if handle.closed {
		s.mu.Unlock()
		return nil
	}
	handle.closed = true
	s.containers[id] = handle
	s.mu.Unlock()

	destroyCtx, cancel := context.WithTimeout(ctx, s.cfg.DestroyTimeout)
	defer cancel()

	if err := s.backend.Destroy(destroyCtx, id); err != nil {
		return fmt.Errorf("scheduler: destroy failed: %w", err)
	}

	s.mu.Lock()
	delete(s.containers, id)
	s.mu.Unlock()

	s.logger.Info("container released",
		zap.String("id", id),
		zap.Int("remaining", len(s.containers)),
	)
	return nil
}

// Run is the one-shot convenience: Acquire → Exec → Release.
func (s *coldStartScheduler) Run(ctx context.Context, spec ContainerSpec, cmd ExecSpec) (ExecResult, error) {
	container, err := s.Acquire(ctx, spec)
	if err != nil {
		return ExecResult{}, err
	}
	defer container.Close(ctx)

	return container.Exec(ctx, cmd)
}

// Stats returns current pool statistics.
func (s *coldStartScheduler) Stats(ctx context.Context) PoolStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return PoolStats{Total: len(s.containers)}
}

// mergeDefaults fills in zero-valued fields from DefaultSpec.
func (s *coldStartScheduler) mergeDefaults(spec ContainerSpec) ContainerSpec {
	if spec.Image == "" {
		spec.Image = s.cfg.DefaultSpec.Image
	}
	if spec.WorkingDir == "" {
		spec.WorkingDir = s.cfg.DefaultSpec.WorkingDir
	}
	if spec.MemoryLimit == 0 {
		spec.MemoryLimit = s.cfg.DefaultSpec.MemoryLimit
	}
	if spec.CPUQuota == 0 {
		spec.CPUQuota = s.cfg.DefaultSpec.CPUQuota
	}
	if spec.Timeout == 0 {
		spec.Timeout = s.cfg.DefaultSpec.Timeout
	}
	return spec
}

// idleCleanupLoop destroys containers that have been idle too long.
func (s *coldStartScheduler) idleCleanupLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupIdle()
		}
	}
}

func (s *coldStartScheduler) cleanupIdle() {
	s.mu.Lock()
	var toClean []string
	now := time.Now()
	for id, h := range s.containers {
		if now.Sub(h.lastUsed) > s.cfg.IdleTimeout {
			toClean = append(toClean, id)
		}
	}
	s.mu.Unlock()

	for _, id := range toClean {
		ctx, cancel := context.WithTimeout(context.Background(), s.cfg.DestroyTimeout)
		if err := s.Release(ctx, id); err != nil {
			s.logger.Warn("idle cleanup failed", zap.String("id", id), zap.Error(err))
		}
		cancel()
	}
}

// ─── Managed Container ───────────────────────────────────────────────────────

type managedContainer struct {
	id      string
	sched   *coldStartScheduler
	spec    ContainerSpec
	backend Backend
	logger  *zap.Logger
	closed  bool
}

func (c *managedContainer) ID() string { return c.id }

func (c *managedContainer) Exec(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	if c.closed {
		return ExecResult{}, fmt.Errorf("scheduler: container %s is closed", c.id)
	}

	// If this container was created with a volume mount, use docker exec directly
	// (the original backend doesn't know about this container)
	if c.spec.VolumeHostPath != "" {
		return c.dockerExec(ctx, spec)
	}

	result, err := c.backend.Exec(ctx, c.id, spec)
	if err != nil {
		return result, err
	}
	c.updateLastUsed()
	return result, nil
}

func (c *managedContainer) Close(ctx context.Context) error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.sched.Release(ctx, c.id)
}

// updateLastUsed refreshes the lastUsed timestamp.
func (c *managedContainer) updateLastUsed() {
	c.sched.mu.Lock()
	if h, ok := c.sched.containers[c.id]; ok {
		h.lastUsed = time.Now()
		c.sched.containers[c.id] = h
	}
	c.sched.mu.Unlock()
}

// dockerExec runs a command via `docker exec` for volume-mounted containers
// that the original backend doesn't know about.
func (c *managedContainer) dockerExec(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	start := time.Now()

	args := []string{"exec"}
	for k, v := range spec.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	if spec.WorkingDir != "" {
		args = append(args, "-w", spec.WorkingDir)
	}
	args = append(args, c.id)
	args = append(args, spec.Cmd...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if len(spec.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(spec.Stdin)
	}

	err := cmd.Run()
	end := time.Now()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return ExecResult{}, fmt.Errorf("docker exec: %w", err)
		}
	}

	c.updateLastUsed()

	return ExecResult{
		ExitCode:   exitCode,
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),
		StartedAt:  start,
		FinishedAt: end,
	}, nil
}
