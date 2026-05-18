package gitclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func testPoolLogger(t *testing.T) *zap.Logger {
	return zaptest.NewLogger(t)
}

func TestWorkerState_String(t *testing.T) {
	tests := []struct {
		state    WorkerState
		expected string
	}{
		{WorkerStateCreated, "created"},
		{WorkerStatePreheated, "preheated"},
		{WorkerStateActive, "active"},
		{WorkerStateIdle, "idle"},
		{WorkerStateRecycled, "recycled"},
		{WorkerState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestNewGitWorker(t *testing.T) {
	logger := testPoolLogger(t)
	worker := NewGitWorker("test-worker-1", logger)

	assert.NotNil(t, worker)
	assert.Equal(t, "test-worker-1", worker.ID())
	assert.Equal(t, WorkerStateCreated, worker.State())
	assert.NotNil(t, worker.Client())
	assert.Equal(t, int32(0), worker.UseCount())
}

func TestGitWorker_TransitionState(t *testing.T) {
	logger := testPoolLogger(t)
	worker := NewGitWorker("test-worker", logger)

	// Created -> Preheated should work
	err := worker.transitionState(WorkerStateCreated, WorkerStatePreheated)
	assert.NoError(t, err)
	assert.Equal(t, WorkerStatePreheated, worker.State())

	// Preheated -> Active should work
	err = worker.transitionState(WorkerStatePreheated, WorkerStateActive)
	assert.NoError(t, err)
	assert.Equal(t, WorkerStateActive, worker.State())

	// Active -> Idle should work
	err = worker.MarkIdle()
	assert.NoError(t, err)
	assert.Equal(t, WorkerStateIdle, worker.State())

	// Idle -> Active is valid (worker reuse)
	err = worker.transitionState(WorkerStateIdle, WorkerStateActive)
	assert.NoError(t, err)
	assert.Equal(t, WorkerStateActive, worker.State())

	// Invalid transition: Created -> Active should fail (must go through Preheated first)
	err = worker.MarkIdle()
	require.NoError(t, err)
	err = worker.transitionState(WorkerStateCreated, WorkerStateActive)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot transition")
}

func TestGitWorker_MarkActive(t *testing.T) {
	logger := testPoolLogger(t)
	worker := NewGitWorker("test-worker", logger)

	// MarkActive from Created should fail (must be Preheated first)
	err := worker.MarkActive()
	assert.Error(t, err)

	// Preheated -> Active should work
	err = worker.transitionState(WorkerStateCreated, WorkerStatePreheated)
	require.NoError(t, err)

	err = worker.MarkActive()
	assert.NoError(t, err)
	assert.Equal(t, WorkerStateActive, worker.State())
}

func TestGitWorker_MarkIdle(t *testing.T) {
	logger := testPoolLogger(t)
	worker := NewGitWorker("test-worker", logger)

	// MarkIdle from Created should fail
	err := worker.MarkIdle()
	assert.Error(t, err)

	// Preheated -> Idle should work
	err = worker.transitionState(WorkerStateCreated, WorkerStatePreheated)
	require.NoError(t, err)

	err = worker.MarkIdle()
	assert.NoError(t, err)
	assert.Equal(t, WorkerStateIdle, worker.State())
}

func TestGitWorker_MarkRecycled(t *testing.T) {
	logger := testPoolLogger(t)
	worker := NewGitWorker("test-worker", logger)

	// MarkRecycled from Created should work
	err := worker.MarkRecycled()
	assert.NoError(t, err)
	assert.Equal(t, WorkerStateRecycled, worker.State())

	// Idempotent - calling again should not error
	err = worker.MarkRecycled()
	assert.NoError(t, err)
}

func TestGitWorker_UseCount(t *testing.T) {
	logger := testPoolLogger(t)
	worker := NewGitWorker("test-worker", logger)

	assert.Equal(t, int32(0), worker.UseCount())
}

func TestPoolConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  PoolConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: PoolConfig{
				InitialSize:         2,
				MaxSize:            5,
				MaxIdleTime:        5 * time.Minute,
				HealthCheckInterval: 30 * time.Second,
				PreheatTimeout:      30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "negative initial size",
			config: PoolConfig{
				InitialSize: -1,
				MaxSize:    5,
			},
			wantErr: true,
		},
		{
			name: "zero max size",
			config: PoolConfig{
				InitialSize: 2,
				MaxSize:    0,
			},
			wantErr: true,
		},
		{
			name: "initial greater than max",
			config: PoolConfig{
				InitialSize: 10,
				MaxSize:    5,
			},
			wantErr: true,
		},
		{
			name: "zero max idle time",
			config: PoolConfig{
				InitialSize: 2,
				MaxSize:    5,
				MaxIdleTime: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultPoolConfig(t *testing.T) {
	config := DefaultPoolConfig()
	assert.NotNil(t, config)
	assert.Equal(t, 2, config.InitialSize)
	assert.Equal(t, 5, config.MaxSize)
	assert.Equal(t, 5*time.Minute, config.MaxIdleTime)
	assert.Equal(t, 30*time.Second, config.HealthCheckInterval)
	assert.Equal(t, 30*time.Second, config.PreheatTimeout)
}

func TestNewWorkerPool(t *testing.T) {
	logger := testPoolLogger(t)

	// Valid config
	pool, err := NewWorkerPool(DefaultPoolConfig(), logger)
	assert.NoError(t, err)
	assert.NotNil(t, pool)
}

func TestNewWorkerPool_InvalidConfig(t *testing.T) {
	logger := testPoolLogger(t)

	// Invalid config - initial > max
	config := &PoolConfig{
		InitialSize: 10,
		MaxSize:     5,
	}
	pool, err := NewWorkerPool(config, logger)
	assert.Error(t, err)
	assert.Nil(t, pool)
}

func TestNewWorkerPool_NilConfig(t *testing.T) {
	logger := testPoolLogger(t)

	// Nil config should use defaults
	pool, err := NewWorkerPool(nil, logger)
	assert.NoError(t, err)
	assert.NotNil(t, pool)
}

func TestWorkerPool_StartAndStop(t *testing.T) {
	logger := testPoolLogger(t)

	config := &PoolConfig{
		InitialSize:         2,
		MaxSize:            5,
		MaxIdleTime:        5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		PreheatTimeout:      30 * time.Second,
	}

	pool, err := NewWorkerPool(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Start the pool
	err = pool.Start(ctx)
	assert.NoError(t, err)

	// Verify initial workers are preheated
	stats := pool.Stats()
	assert.Equal(t, 2, stats.TotalWorkers)
	assert.Equal(t, 2, stats.PreheatedWorkers)

	// Stop the pool
	err = pool.Stop(ctx)
	assert.NoError(t, err)

	// Verify all workers are removed
	stats = pool.Stats()
	assert.Equal(t, 0, stats.TotalWorkers)
}

func TestWorkerPool_Start_AlreadyClosed(t *testing.T) {
	logger := testPoolLogger(t)

	pool, err := NewWorkerPool(DefaultPoolConfig(), logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Start
	err = pool.Start(ctx)
	assert.NoError(t, err)

	// Stop
	err = pool.Stop(ctx)
	assert.NoError(t, err)

	// Start again should fail
	err = pool.Start(ctx)
	assert.Error(t, err)
	assert.Equal(t, ErrPoolClosed, err)
}

func TestWorkerPool_Acquire(t *testing.T) {
	logger := testPoolLogger(t)

	config := &PoolConfig{
		InitialSize:         1,
		MaxSize:            2,
		MaxIdleTime:        5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		PreheatTimeout:      30 * time.Second,
	}

	pool, err := NewWorkerPool(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(ctx)

	// Acquire a worker
	worker1, err := pool.Acquire(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, worker1)
	assert.Equal(t, WorkerStateActive, worker1.State())
	assert.Equal(t, int32(1), worker1.UseCount())

	// Stats should show 1 active
	stats := pool.Stats()
	assert.Equal(t, 1, stats.ActiveWorkers)

	// Release the worker
	pool.Release(worker1)

	// Worker should now be idle
	stats = pool.Stats()
	assert.Equal(t, 1, stats.IdleWorkers)
}

func TestWorkerPool_Acquire_PoolExhausted(t *testing.T) {
	logger := testPoolLogger(t)

	config := &PoolConfig{
		InitialSize:         1,
		MaxSize:            1,
		MaxIdleTime:        5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		PreheatTimeout:      30 * time.Second,
	}

	pool, err := NewWorkerPool(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(ctx)

	// Acquire the only worker
	worker, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer pool.Release(worker)

	// Try to acquire another - should fail since pool is exhausted
	_, err = pool.Acquire(ctx)
	assert.Error(t, err)
	assert.Equal(t, ErrPoolExhausted, err)
}

func TestWorkerPool_Acquire_NewWorker(t *testing.T) {
	logger := testPoolLogger(t)

	config := &PoolConfig{
		InitialSize:         0, // Start with no workers
		MaxSize:            2,
		MaxIdleTime:        5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		PreheatTimeout:      30 * time.Second,
	}

	pool, err := NewWorkerPool(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(ctx)

	// Acquire should create a new worker since pool is empty
	worker, err := pool.Acquire(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, worker)

	stats := pool.Stats()
	assert.Equal(t, 1, stats.TotalWorkers)
	assert.Equal(t, 1, stats.ActiveWorkers)
}

func TestWorkerPool_Release_NilWorker(t *testing.T) {
	logger := testPoolLogger(t)

	pool, err := NewWorkerPool(DefaultPoolConfig(), logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(ctx)

	// Releasing nil should not panic
	pool.Release(nil)
}

func TestWorkerPool_Stats(t *testing.T) {
	logger := testPoolLogger(t)

	config := &PoolConfig{
		InitialSize:         2,
		MaxSize:            5,
		MaxIdleTime:        5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		PreheatTimeout:      30 * time.Second,
	}

	pool, err := NewWorkerPool(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(ctx)

	stats := pool.Stats()

	assert.Equal(t, 2, stats.TotalWorkers)
	assert.Equal(t, 0, stats.ActiveWorkers)
	assert.Equal(t, 2, stats.PreheatedWorkers)
	assert.Equal(t, 0, stats.IdleWorkers)

	// Acquire one worker
	worker, err := pool.Acquire(ctx)
	require.NoError(t, err)

	stats = pool.Stats()
	assert.Equal(t, 2, stats.TotalWorkers)
	assert.Equal(t, 1, stats.ActiveWorkers)
	assert.Equal(t, 1, stats.PreheatedWorkers) // One still preheated (not yet idle)

	// Release it
	pool.Release(worker)

	stats = pool.Stats()
	assert.Equal(t, 1, stats.IdleWorkers)
	assert.Equal(t, 1, stats.PreheatedWorkers)
}

func TestWorkerPool_RecycleIdle(t *testing.T) {
	logger := testPoolLogger(t)

	config := &PoolConfig{
		InitialSize:         2,
		MaxSize:            5,
		MaxIdleTime:        50 * time.Millisecond, // Very short for testing
		HealthCheckInterval: 30 * time.Second,
		PreheatTimeout:      30 * time.Second,
	}

	pool, err := NewWorkerPool(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(ctx)

	// Acquire and release a worker to make it idle
	worker, err := pool.Acquire(ctx)
	require.NoError(t, err)
	pool.Release(worker)

	// Wait for idle timeout
	time.Sleep(100 * time.Millisecond)

	// Recycle should find and recycle the idle worker
	recycled := pool.RecycleIdle()
	assert.Equal(t, 1, recycled)

	stats := pool.Stats()
	assert.Equal(t, 1, stats.TotalWorkers) // One was recycled, one remains
}

func TestWorkerPool_RecycleIdle_NoMatch(t *testing.T) {
	logger := testPoolLogger(t)

	config := &PoolConfig{
		InitialSize:         2,
		MaxSize:            5,
		MaxIdleTime:        5 * time.Minute, // Long timeout
		HealthCheckInterval: 30 * time.Second,
		PreheatTimeout:      30 * time.Second,
	}

	pool, err := NewWorkerPool(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(ctx)

	// Recycle should not find any workers to recycle
	recycled := pool.RecycleIdle()
	assert.Equal(t, 0, recycled)
}

func TestWorkerPool_Release_BadState(t *testing.T) {
	logger := testPoolLogger(t)

	config := &PoolConfig{
		InitialSize:         1,
		MaxSize:            1,
		MaxIdleTime:        5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		PreheatTimeout:      30 * time.Second,
	}

	pool, err := NewWorkerPool(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = pool.Start(ctx)
	require.NoError(t, err)
	defer pool.Stop(ctx)

	// Get a worker and manually set it to Recycled state
	worker, err := pool.Acquire(ctx)
	require.NoError(t, err)

	worker.mu.Lock()
	worker.state = WorkerStateRecycled
	worker.mu.Unlock()

	// Release should remove the worker since it's in a bad state
	pool.Release(worker)

	stats := pool.Stats()
	assert.Equal(t, 0, stats.TotalWorkers) // Worker was removed
}

func TestGitWorker_Clone(t *testing.T) {
	logger := testPoolLogger(t)

	worker := NewGitWorker("test-worker", logger)

	ctx := context.Background()

	// Clone in wrong state should fail
	err := worker.Clone(ctx, CloneOptions{URL: "file:///fake", Directory: "/tmp/fake"})
	assert.Error(t, err)
}

func TestGitWorker_Open(t *testing.T) {
	logger := testPoolLogger(t)

	worker := NewGitWorker("test-worker", logger)

	ctx := context.Background()

	// Open in wrong state should fail
	err := worker.Open(ctx, "/tmp/fake")
	assert.Error(t, err)
}

func TestGitWorker_Reset(t *testing.T) {
	logger := testPoolLogger(t)

	worker := NewGitWorker("test-worker", logger)

	// Set to a different state
	worker.mu.Lock()
	worker.state = WorkerStatePreheated
	worker.repoPath = "/some/path"
	worker.repoURL = "https://example.com/repo"
	worker.mu.Unlock()

	// Reset should clear everything
	err := worker.Reset()
	assert.NoError(t, err)

	assert.Equal(t, WorkerStateCreated, worker.State())

	worker.mu.RLock()
	assert.Empty(t, worker.repoPath)
	assert.Empty(t, worker.repoURL)
	worker.mu.RUnlock()
}

func TestWorkerPool_Stop_AlreadyStopped(t *testing.T) {
	logger := testPoolLogger(t)

	pool, err := NewWorkerPool(DefaultPoolConfig(), logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Stop without starting should be fine
	err = pool.Stop(ctx)
	assert.NoError(t, err)

	// Stop again should also be fine
	err = pool.Stop(ctx)
	assert.NoError(t, err)
}

func TestWorkerPool_Acquire_Closed(t *testing.T) {
	logger := testPoolLogger(t)

	pool, err := NewWorkerPool(DefaultPoolConfig(), logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Don't start - try to acquire directly
	_, err = pool.Acquire(ctx)
	assert.Error(t, err)
	assert.Equal(t, ErrPoolClosed, err)
}
