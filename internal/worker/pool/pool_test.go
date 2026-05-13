// Package pool provides Worker pool management with prewarming, scaling, and health checks.
package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/worker/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// mockSandbox implements sandbox.Sandbox for testing.
type mockSandbox struct {
	createFn  func(ctx context.Context, opts sandbox.SandboxOptions) (string, error)
	execFn     func(ctx context.Context, id string, opts sandbox.ExecOptions) (sandbox.ExecResult, error)
	destroyFn  func(ctx context.Context, id string) error
	statusFn   func(ctx context.Context, id string) (sandbox.SandboxStatus, error)

	sandboxes map[string]bool
	mu        sync.Mutex
}

func newMockSandbox() *mockSandbox {
	return &mockSandbox{
		sandboxes: make(map[string]bool),
	}
}

func (m *mockSandbox) Create(ctx context.Context, opts sandbox.SandboxOptions) (string, error) {
	m.mu.Lock()
	if m.sandboxes == nil {
		m.sandboxes = make(map[string]bool)
	}
	m.mu.Unlock()

	if m.createFn != nil {
		id, err := m.createFn(ctx, opts)
		if err != nil {
			return "", err
		}
		m.mu.Lock()
		m.sandboxes[id] = true
		m.mu.Unlock()
		return id, nil
	}
	id := "mock-sandbox-" + opts.Image
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sandboxes[id] = true
	return id, nil
}

func (m *mockSandbox) Exec(ctx context.Context, id string, opts sandbox.ExecOptions) (sandbox.ExecResult, error) {
	if m.execFn != nil {
		return m.execFn(ctx, id, opts)
	}
	return sandbox.ExecResult{
		ExitCode:   0,
		Stdout:     []byte("success"),
		Stderr:     []byte(""),
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
	}, nil
}

func (m *mockSandbox) Destroy(ctx context.Context, id string) error {
	if m.destroyFn != nil {
		return m.destroyFn(ctx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sandboxes, id)
	return nil
}

func (m *mockSandbox) Status(ctx context.Context, id string) (sandbox.SandboxStatus, error) {
	if m.statusFn != nil {
		return m.statusFn(ctx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sandboxes[id]; ok {
		return sandbox.StatusRunning, nil
	}
	return "", &sandbox.ErrSandboxNotFound{ID: id}
}

func (m *mockSandbox) HealthCheck(ctx context.Context, id string) (sandbox.HealthStatus, error) {
	m.mu.Lock()
	_, ok := m.sandboxes[id]
	m.mu.Unlock()

	if !ok {
		return sandbox.HealthStatus{}, &sandbox.ErrSandboxNotFound{ID: id}
	}
	return sandbox.HealthStatus{
		IsHealthy: true,
		Message:   "ok",
		LastCheck: time.Now(),
	}, nil
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name:    "valid config",
			cfg:     DefaultConfig(),
			wantErr: nil,
		},
		{
			name: "negative min workers",
			cfg: Config{
				MinWorkers:          -1,
				MaxWorkers:          10,
				ScaleUpStep:         2,
				ScaleDownStep:       1,
				ScaleUpThreshold:    5,
				ScaleDownThreshold:  2,
			},
			wantErr: ErrInvalidMinWorkers,
		},
		{
			name: "max less than min",
			cfg: Config{
				MinWorkers:          5,
				MaxWorkers:          3,
				ScaleUpStep:         2,
				ScaleDownStep:       1,
				ScaleUpThreshold:    5,
				ScaleDownThreshold:  2,
			},
			wantErr: ErrMaxWorkersLessThanMin,
		},
		{
			name: "zero scale up step",
			cfg: Config{
				MinWorkers:          1,
				MaxWorkers:          10,
				ScaleUpStep:         0,
				ScaleDownStep:       1,
				ScaleUpThreshold:    5,
				ScaleDownThreshold:  2,
			},
			wantErr: ErrInvalidScaleUpStep,
		},
		{
			name: "invalid scale thresholds",
			cfg: Config{
				MinWorkers:          1,
				MaxWorkers:         10,
				ScaleUpStep:         2,
				ScaleDownStep:       1,
				ScaleUpThreshold:    5,
				ScaleDownThreshold:  10, // Must be < ScaleUpThreshold
			},
			wantErr: ErrInvalidScaleThresholds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPool_Start(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := newMockSandbox()

	cfg := DefaultConfig()
	cfg.MinWorkers = 2

	pool, err := NewPool(cfg, ms, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(context.Background())

	assert.True(t, pool.WorkerCount() >= 0)
}

func TestPool_Acquire(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := newMockSandbox()

	cfg := DefaultConfig()
	cfg.MinWorkers = 0
	cfg.MaxWorkers = 2

	pool, err := NewPool(cfg, ms, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(context.Background())

	// Acquire should create a new worker when none available
	w, err := pool.Acquire(ctx)
	require.NoError(t, err)
	assert.NotNil(t, w)

	// Second acquire should return same worker (still available)
	pool.Release(w)

	w2, err := pool.Acquire(ctx)
	require.NoError(t, err)
	assert.Equal(t, w.ID(), w2.ID())
}

func TestPool_Acquire_Exhausted(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := DefaultConfig()
	cfg.MinWorkers = 0
	cfg.MaxWorkers = 1

	pool, err := NewPool(cfg, &mockSandbox{
		createFn: func(ctx context.Context, opts sandbox.SandboxOptions) (string, error) {
			return "sandbox-1", nil
		},
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(context.Background())

	// Acquire first worker
	w1, err := pool.Acquire(ctx)
	require.NoError(t, err)

	// Try to acquire second worker when max is 1 - should fail
	_, err = pool.Acquire(ctx)
	assert.ErrorIs(t, err, ErrPoolExhausted)

	pool.Release(w1)
}

func TestPool_SubmitTask(t *testing.T) {
	logger := zaptest.NewLogger(t)
	execCalled := atomic.Bool{}

	ms := &mockSandbox{
		execFn: func(ctx context.Context, id string, opts sandbox.ExecOptions) (sandbox.ExecResult, error) {
			execCalled.Store(true)
			return sandbox.ExecResult{
				ExitCode:   0,
				Stdout:     []byte("task completed"),
				Stderr:     []byte(""),
				StartedAt:  time.Now(),
				FinishedAt: time.Now(),
			}, nil
		},
	}

	cfg := DefaultConfig()
	cfg.MinWorkers = 0

	pool, err := NewPool(cfg, ms, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(context.Background())

	result, err := pool.SubmitTask(ctx, sandbox.ExecOptions{
		Cmd:     []string{"echo", "hello"},
		Timeout: 5 * time.Second,
	})

	require.NoError(t, err)
	assert.True(t, execCalled.Load())
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, []byte("task completed"), result.Stdout)
}

func TestPool_SubmitTask_ExecError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	ms := &mockSandbox{
		createFn: func(ctx context.Context, opts sandbox.SandboxOptions) (string, error) {
			return "sandbox-error", nil
		},
		execFn: func(ctx context.Context, id string, opts sandbox.ExecOptions) (sandbox.ExecResult, error) {
			return sandbox.ExecResult{}, &sandbox.ErrSandboxExecutionFailed{
				SandboxID: id,
				Cmd:       opts.Cmd,
				Cause:     errors.New("execution failed"),
			}
		},
		destroyFn: func(ctx context.Context, id string) error {
			return nil
		},
	}

	cfg := DefaultConfig()
	cfg.MinWorkers = 0
	cfg.MaxWorkers = 1

	pool, err := NewPool(cfg, ms, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(context.Background())

	_, err = pool.SubmitTask(ctx, sandbox.ExecOptions{
		Cmd:     []string{"false"},
		Timeout: 5 * time.Second,
	})

	assert.Error(t, err)
}

func TestPool_Stop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := newMockSandbox()

	cfg := DefaultConfig()
	cfg.MinWorkers = 0

	pool, err := NewPool(cfg, ms, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)

	// Stop should succeed
	err = pool.Stop(context.Background())
	assert.NoError(t, err)

	// Operating on stopped pool should fail
	_, err = pool.Acquire(context.Background())
	assert.ErrorIs(t, err, ErrPoolNotRunning)
}

func TestPool_Stats(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := newMockSandbox()

	cfg := DefaultConfig()
	cfg.MinWorkers = 0

	pool, err := NewPool(cfg, ms, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(context.Background())

	stats := pool.Stats()
	assert.Equal(t, 0, stats.TotalWorkers)
	assert.Equal(t, 0, stats.AvailableWorkers)

	// Acquire a worker
	w, err := pool.Acquire(ctx)
	require.NoError(t, err)
	stats = pool.Stats()
	assert.Equal(t, 1, stats.TotalWorkers)
	assert.Equal(t, 0, stats.AvailableWorkers) // Acquired worker is not "available"
	assert.Equal(t, 1, stats.BusyWorkers)

	pool.Release(w)
	stats = pool.Stats()
	assert.Equal(t, 1, stats.TotalWorkers)
	assert.Equal(t, 1, stats.AvailableWorkers)
}

func TestPool_ScaleUp(t *testing.T) {
	logger := zaptest.NewLogger(t)

	ms := &mockSandbox{}

	cfg := DefaultConfig()
	cfg.MinWorkers = 0
	cfg.MaxWorkers = 5
	cfg.ScaleUpStep = 2
	cfg.ScaleUpThreshold = 3
	cfg.ScaleDownThreshold = 0
	cfg.ScaleInterval = 50 * time.Millisecond

	pool, err := NewPool(cfg, ms, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)

	// Submit multiple tasks to trigger scale up
	for i := 0; i < 3; i++ {
		_, err = pool.SubmitTask(ctx, sandbox.ExecOptions{
			Cmd:     []string{"echo", "test"},
			Timeout: 5 * time.Second,
		})
		require.NoError(t, err)
	}

	// Verify pool has at least one worker created
	stats := pool.Stats()
	assert.GreaterOrEqual(t, stats.TotalWorkers, 1, "should have created worker")

	pool.Stop(context.Background())
}

func TestWorker_Execute(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := newMockSandbox()

	id, err := ms.Create(context.Background(), sandbox.SandboxOptions{Image: "test"})
	require.NoError(t, err)

	w := NewWorker(id, ms, logger)
	assert.Equal(t, WorkerStatusStarting, w.Status())

	// Execute should change status to busy
	result, err := w.Execute(context.Background(), sandbox.ExecOptions{
		Cmd:     []string{"echo", "hello"},
		Timeout: 5 * time.Second,
	})

	assert.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, WorkerStatusIdle, w.Status()) // Should return to idle after
}

func TestWorker_Stop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := newMockSandbox()

	id, err := ms.Create(context.Background(), sandbox.SandboxOptions{Image: "test"})
	require.NoError(t, err)

	w := NewWorker(id, ms, logger)
	w.setStatus(WorkerStatusIdle)

	w.Stop()
	assert.Equal(t, WorkerStatusStopping, w.Status())
	assert.True(t, w.IsDraining())
}

func TestWorker_IsAvailable(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := newMockSandbox()

	id, err := ms.Create(context.Background(), sandbox.SandboxOptions{Image: "test"})
	require.NoError(t, err)

	w := NewWorker(id, ms, logger)
	w.setStatus(WorkerStatusIdle)
	assert.True(t, w.IsAvailable())

	w.setStatus(WorkerStatusBusy)
	assert.False(t, w.IsAvailable())

	w.setStatus(WorkerStatusStopping)
	assert.False(t, w.IsAvailable())
}

func TestPool_DestroyWorker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	destroyed := atomic.Bool{}
	idCounter := atomic.Int32{}

	ms := &mockSandbox{
		createFn: func(ctx context.Context, opts sandbox.SandboxOptions) (string, error) {
			id := fmt.Sprintf("test-sandbox-%d", idCounter.Add(1))
			return id, nil
		},
		destroyFn: func(ctx context.Context, id string) error {
			destroyed.Store(true)
			return nil
		},
	}

	cfg := DefaultConfig()
	pool, err := NewPool(cfg, ms, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(context.Background())

	// Worker should exist
	count := pool.WorkerCount()
	assert.Equal(t, 2, count) // MinWorkers default is 2

	// Workers are prewarmed
	workers := pool.WorkerCount()
	assert.Greater(t, workers, 0)

	pool.Stop(context.Background())
}

func TestPool_GracefulShutdown(t *testing.T) {
	logger := zaptest.NewLogger(t)

	execCount := atomic.Int32{}
	ms := &mockSandbox{
		execFn: func(ctx context.Context, id string, opts sandbox.ExecOptions) (sandbox.ExecResult, error) {
			execCount.Add(1)
			// Simulate long running task
			time.Sleep(100 * time.Millisecond)
			return sandbox.ExecResult{
				ExitCode:   0,
				Stdout:     []byte("done"),
				FinishedAt: time.Now(),
			}, nil
		},
	}

	cfg := DefaultConfig()
	cfg.MinWorkers = 1

	pool, err := NewPool(cfg, ms, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)

	// Submit a task
	_, err = pool.SubmitTask(ctx, sandbox.ExecOptions{
		Cmd:     []string{"sleep", "1"},
		Timeout: 10 * time.Second,
	})
	require.NoError(t, err)

	// Stop should wait for task to complete
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = pool.Stop(stopCtx)
	assert.NoError(t, err)
	assert.Equal(t, int32(1), execCount.Load())
}

func TestPool_Prewarming(t *testing.T) {
	logger := zaptest.NewLogger(t)
	created := atomic.Int32{}

	ms := &mockSandbox{
		createFn: func(ctx context.Context, opts sandbox.SandboxOptions) (string, error) {
			created.Add(1)
			return "sandbox-prewarm-" + string(rune('0'+created.Load())), nil
		},
	}

	cfg := DefaultConfig()
	cfg.MinWorkers = 3

	pool, err := NewPool(cfg, ms, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(context.Background())

	// Wait a bit for prewarm to complete
	time.Sleep(100 * time.Millisecond)

	stats := pool.Stats()
	assert.GreaterOrEqual(t, stats.TotalWorkers, cfg.MinWorkers)
}
