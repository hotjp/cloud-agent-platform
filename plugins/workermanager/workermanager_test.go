// Package workermanager provides Worker lifecycle management tests.
package workermanager

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// mockSandbox implements worker.SandboxBackend for testing.
type mockSandbox struct {
	createFn   func(ctx context.Context, opts worker.SandboxOptions) (string, error)
	execFn     func(ctx context.Context, id string, opts worker.ExecOptions) (worker.ExecResult, error)
	destroyFn  func(ctx context.Context, id string) error
	statusFn   func(ctx context.Context, id string) (worker.SandboxStatus, error)
	healthFn   func(ctx context.Context, id string) (worker.HealthStatus, error)

	mu        sync.Mutex
	sandboxes map[string]bool
}

func newMockSandbox() *mockSandbox {
	return &mockSandbox{
		sandboxes: make(map[string]bool),
	}
}

func (m *mockSandbox) Create(ctx context.Context, opts worker.SandboxOptions) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sandboxes == nil {
		m.sandboxes = make(map[string]bool)
	}

	if m.createFn != nil {
		id, err := m.createFn(ctx, opts)
		if err != nil {
			return "", err
		}
		m.sandboxes[id] = true
		return id, nil
	}

	id := "mock-sandbox-" + opts.Image
	m.sandboxes[id] = true
	return id, nil
}

func (m *mockSandbox) Exec(ctx context.Context, id string, opts worker.ExecOptions) (worker.ExecResult, error) {
	m.mu.Lock()
	_, ok := m.sandboxes[id]
	m.mu.Unlock()

	if !ok {
		return worker.ExecResult{}, &worker.ErrSandboxNotFound{ID: id}
	}

	if m.execFn != nil {
		return m.execFn(ctx, id, opts)
	}

	return worker.ExecResult{
		ExitCode:   0,
		Stdout:     []byte("success"),
		Stderr:     []byte(""),
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
	}, nil
}

func (m *mockSandbox) Destroy(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.destroyFn != nil {
		return m.destroyFn(ctx, id)
	}

	delete(m.sandboxes, id)
	return nil
}

func (m *mockSandbox) Status(ctx context.Context, id string) (worker.SandboxStatus, error) {
	m.mu.Lock()
	_, ok := m.sandboxes[id]
	m.mu.Unlock()

	if !ok {
		return "", &worker.ErrSandboxNotFound{ID: id}
	}

	if m.statusFn != nil {
		return m.statusFn(ctx, id)
	}

	return worker.StatusRunning, nil
}

func (m *mockSandbox) HealthCheck(ctx context.Context, id string) (worker.HealthStatus, error) {
	m.mu.Lock()
	_, ok := m.sandboxes[id]
	m.mu.Unlock()

	if !ok {
		return worker.HealthStatus{}, &worker.ErrSandboxNotFound{ID: id}
	}

	if m.healthFn != nil {
		return m.healthFn(ctx, id)
	}

	return worker.HealthStatus{
		IsHealthy: true,
		Message:   "ok",
		LastCheck: time.Now(),
	}, nil
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     DefaultConfig(),
			wantErr: false,
		},
		{
			name: "zero max workers",
			cfg: Config{
				MaxWorkers: 0,
				IdleTimeout: 5 * time.Minute,
				DefaultSandboxOpts: worker.SandboxOptions{
					Image: "alpine:latest",
				},
			},
			wantErr: true,
		},
		{
			name: "negative max workers",
			cfg: Config{
				MaxWorkers: -1,
				IdleTimeout: 5 * time.Minute,
				DefaultSandboxOpts: worker.SandboxOptions{
					Image: "alpine:latest",
				},
			},
			wantErr: true,
		},
		{
			name: "zero idle timeout",
			cfg: Config{
				MaxWorkers: 10,
				IdleTimeout: 0,
				DefaultSandboxOpts: worker.SandboxOptions{
					Image: "alpine:latest",
				},
			},
			wantErr: true,
		},
		{
			name: "empty image",
			cfg: Config{
				MaxWorkers: 10,
				IdleTimeout: 5 * time.Minute,
				DefaultSandboxOpts: worker.SandboxOptions{
					Image: "",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWorkerManager_StartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := newMockSandbox()

	cfg := DefaultConfig()
	wm, err := New(cfg, ms, logger)
	require.NoError(t, err)

	// Start should succeed
	err = wm.Start()
	require.NoError(t, err)

	// Starting again should fail
	err = wm.Start()
	assert.Error(t, err)

	// Stop should succeed
	err = wm.Stop(context.Background())
	require.NoError(t, err)
}

func TestWorkerManager_AcquireRelease(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := newMockSandbox()

	cfg := DefaultConfig()
	cfg.MaxWorkers = 2
	wm, err := New(cfg, ms, logger)
	require.NoError(t, err)

	err = wm.Start()
	require.NoError(t, err)
	defer wm.Stop(context.Background())

	// Acquire first worker
	id1, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, id1)

	stats := wm.Stats()
	assert.Equal(t, 1, stats.TotalWorkers)
	assert.Equal(t, 0, stats.IdleWorkers)
	assert.Equal(t, 1, stats.BusyWorkers)

	// Release the worker
	wm.ReleaseWorker(id1)

	stats = wm.Stats()
	assert.Equal(t, 1, stats.TotalWorkers)
	assert.Equal(t, 1, stats.IdleWorkers)
	assert.Equal(t, 0, stats.BusyWorkers)

	// Acquire again should get the same worker
	id2, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)
	assert.Equal(t, id1, id2)
}

func TestWorkerManager_AcquireOnDemand(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := newMockSandbox()

	cfg := DefaultConfig()
	cfg.MaxWorkers = 2
	wm, err := New(cfg, ms, logger)
	require.NoError(t, err)

	err = wm.Start()
	require.NoError(t, err)
	defer wm.Stop(context.Background())

	// Initially no workers
	stats := wm.Stats()
	assert.Equal(t, 0, stats.TotalWorkers)

	// Acquire should create a worker on demand
	id1, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, id1)

	stats = wm.Stats()
	assert.Equal(t, 1, stats.TotalWorkers)

	// Release and acquire should reuse the same worker
	wm.ReleaseWorker(id1)
	id2, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)
	assert.Equal(t, id1, id2)
}

func TestWorkerManager_MaxWorkersLimit(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := DefaultConfig()
	cfg.MaxWorkers = 1
	wm, err := New(cfg, newMockSandbox(), logger)
	require.NoError(t, err)

	err = wm.Start()
	require.NoError(t, err)
	defer wm.Stop(context.Background())

	// Acquire first worker
	id1, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)

	// Try to acquire second worker should fail
	_, err = wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker limit reached")

	// Release and acquire should work
	wm.ReleaseWorker(id1)
	id2, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)
	assert.Equal(t, id1, id2)
}

func TestWorkerManager_Execute(t *testing.T) {
	logger := zaptest.NewLogger(t)
	execCalled := atomic.Bool{}

	ms := &mockSandbox{
		execFn: func(ctx context.Context, id string, opts worker.ExecOptions) (worker.ExecResult, error) {
			execCalled.Store(true)
			return worker.ExecResult{
				ExitCode:   0,
				Stdout:     []byte("task completed"),
				Stderr:     []byte(""),
				StartedAt:  time.Now(),
				FinishedAt: time.Now(),
			}, nil
		},
	}

	cfg := DefaultConfig()
	wm, err := New(cfg, ms, logger)
	require.NoError(t, err)

	err = wm.Start()
	require.NoError(t, err)
	defer wm.Stop(context.Background())

	id, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)

	result, err := wm.ExecOnWorker(context.Background(), id, worker.ExecOptions{
		Cmd:     []string{"echo", "hello"},
		Timeout: 5 * time.Second,
	})

	require.NoError(t, err)
	assert.True(t, execCalled.Load())
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, []byte("task completed"), result.Stdout)
}

func TestWorkerManager_ExecuteError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	ms := &mockSandbox{
		execFn: func(ctx context.Context, id string, opts worker.ExecOptions) (worker.ExecResult, error) {
			return worker.ExecResult{}, errors.New("execution failed")
		},
	}

	cfg := DefaultConfig()
	wm, err := New(cfg, ms, logger)
	require.NoError(t, err)

	err = wm.Start()
	require.NoError(t, err)
	defer wm.Stop(context.Background())

	id, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)

	_, err = wm.ExecOnWorker(context.Background(), id, worker.ExecOptions{
		Cmd:     []string{"false"},
		Timeout: 5 * time.Second,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "execution failed")
}

func TestWorkerManager_DestroyWorker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	destroyed := atomic.Bool{}

	ms := &mockSandbox{
		destroyFn: func(ctx context.Context, id string) error {
			destroyed.Store(true)
			return nil
		},
	}

	cfg := DefaultConfig()
	wm, err := New(cfg, ms, logger)
	require.NoError(t, err)

	err = wm.Start()
	require.NoError(t, err)
	defer wm.Stop(context.Background())

	id, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)

	stats := wm.Stats()
	assert.Equal(t, 1, stats.TotalWorkers)

	err = wm.DestroyWorker(context.Background(), id)
	require.NoError(t, err)

	assert.True(t, destroyed.Load())

	stats = wm.Stats()
	assert.Equal(t, 0, stats.TotalWorkers)
}

func TestWorkerManager_ExecuteOnDestroyedWorker(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := DefaultConfig()
	wm, err := New(cfg, newMockSandbox(), logger)
	require.NoError(t, err)

	err = wm.Start()
	require.NoError(t, err)
	defer wm.Stop(context.Background())

	id, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)

	// Destroy the worker
	err = wm.DestroyWorker(context.Background(), id)
	require.NoError(t, err)

	// Try to execute on destroyed worker should fail
	_, err = wm.ExecOnWorker(context.Background(), id, worker.ExecOptions{
		Cmd: []string{"echo", "hello"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker not found")
}

func TestWorkerManager_GetWorkerHealth(t *testing.T) {
	logger := zaptest.NewLogger(t)

	healthCalled := atomic.Bool{}
	ms := &mockSandbox{
		healthFn: func(ctx context.Context, id string) (worker.HealthStatus, error) {
			healthCalled.Store(true)
			return worker.HealthStatus{
				IsHealthy: true,
				Message:   "all good",
				LastCheck: time.Now(),
			}, nil
		},
	}

	cfg := DefaultConfig()
	wm, err := New(cfg, ms, logger)
	require.NoError(t, err)

	err = wm.Start()
	require.NoError(t, err)
	defer wm.Stop(context.Background())

	id, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)

	health, err := wm.GetWorkerHealth(context.Background(), id)
	require.NoError(t, err)
	assert.True(t, healthCalled.Load())
	assert.True(t, health.IsHealthy)
	assert.Equal(t, "all good", health.Message)
}

func TestWorkerManager_Stats(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := DefaultConfig()
	cfg.MaxWorkers = 5
	wm, err := New(cfg, newMockSandbox(), logger)
	require.NoError(t, err)

	err = wm.Start()
	require.NoError(t, err)
	defer wm.Stop(context.Background())

	stats := wm.Stats()
	assert.Equal(t, 0, stats.TotalWorkers)
	assert.Equal(t, 0, stats.IdleWorkers)
	assert.Equal(t, 0, stats.BusyWorkers)

	// Acquire two workers
	id1, err := wm.AcquireWorker(context.Background(), worker.SandboxOptions{Image: "alpine:latest"})
	require.NoError(t, err)
	_, err = wm.AcquireWorker(context.Background(), worker.SandboxOptions{Image: "ubuntu:latest"})
	require.NoError(t, err)

	stats = wm.Stats()
	assert.Equal(t, 2, stats.TotalWorkers)
	assert.Equal(t, 0, stats.IdleWorkers)
	assert.Equal(t, 2, stats.BusyWorkers)

	// Release one
	wm.ReleaseWorker(id1)

	stats = wm.Stats()
	assert.Equal(t, 2, stats.TotalWorkers)
	assert.Equal(t, 1, stats.IdleWorkers)
	assert.Equal(t, 1, stats.BusyWorkers)
}

func TestWorkerManager_NotRunningError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := DefaultConfig()
	wm, err := New(cfg, newMockSandbox(), logger)
	require.NoError(t, err)

	// Don't start - try to acquire should fail
	_, err = wm.AcquireWorker(context.Background(), worker.SandboxOptions{
		Image: "alpine:latest",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}
