// Package scheduler - tests for cold-start scheduler.
package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// stubBackend is an in-memory Backend for testing.
type stubBackend struct {
	created  atomic.Int32
	destroyed atomic.Int32
}

func (b *stubBackend) Create(ctx context.Context, spec ContainerSpec) (string, error) {
	id := fmt.Sprintf("container-%d", b.created.Add(1))
	return id, nil
}

func (b *stubBackend) Exec(ctx context.Context, containerID string, spec ExecSpec) (ExecResult, error) {
	return ExecResult{
		ExitCode:   0,
		Stdout:     []byte("ok"),
		Stderr:     nil,
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
	}, nil
}

func (b *stubBackend) Destroy(ctx context.Context, containerID string) error {
	b.destroyed.Add(1)
	return nil
}

func TestColdStart_AcquireAndRelease(t *testing.T) {
	backend := &stubBackend{}
	logger := zap.NewNop()

	sched, err := New(Config{
		MaxContainers:  5,
		CreateTimeout:  5 * time.Second,
		DestroyTimeout: 5 * time.Second,
		IdleTimeout:    10 * time.Minute,
		DefaultSpec:    ContainerSpec{Image: "alpine:latest"},
	}, backend, logger)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	sched.Start(ctx)
	defer sched.Stop(ctx)

	// Acquire
	c, err := sched.Acquire(ctx, ContainerSpec{Image: "alpine:latest"})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID() == "" {
		t.Fatal("expected non-empty container ID")
	}

	stats := sched.Stats(ctx)
	if stats.Total != 1 {
		t.Fatalf("expected 1 container, got %d", stats.Total)
	}

	// Release
	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}

	stats = sched.Stats(ctx)
	if stats.Total != 0 {
		t.Fatalf("expected 0 containers after release, got %d", stats.Total)
	}

	if backend.created.Load() != 1 {
		t.Fatalf("expected 1 create call, got %d", backend.created.Load())
	}
	if backend.destroyed.Load() != 1 {
		t.Fatalf("expected 1 destroy call, got %d", backend.destroyed.Load())
	}
}

func TestColdStart_Run(t *testing.T) {
	backend := &stubBackend{}
	logger := zap.NewNop()

	sched, err := New(DefaultConfig(), backend, logger)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	sched.Start(ctx)
	defer sched.Stop(ctx)

	result, err := sched.Run(ctx, ContainerSpec{Image: "alpine:latest"}, ExecSpec{
		Cmd: []string{"echo", "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if string(result.Stdout) != "ok" {
		t.Fatalf("expected stdout 'ok', got %q", string(result.Stdout))
	}

	// After Run, container should be destroyed
	stats := sched.Stats(ctx)
	if stats.Total != 0 {
		t.Fatalf("expected 0 containers after Run, got %d", stats.Total)
	}
}

func TestColdStart_MaxContainers(t *testing.T) {
	backend := &stubBackend{}
	logger := zap.NewNop()

	sched, err := New(Config{
		MaxContainers:  2,
		CreateTimeout:  5 * time.Second,
		DestroyTimeout: 5 * time.Second,
		IdleTimeout:    10 * time.Minute,
		DefaultSpec:    ContainerSpec{Image: "alpine:latest"},
	}, backend, logger)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	sched.Start(ctx)
	defer sched.Stop(ctx)

	// Fill up
	c1, _ := sched.Acquire(ctx, ContainerSpec{Image: "alpine:latest"})
	c2, _ := sched.Acquire(ctx, ContainerSpec{Image: "alpine:latest"})

	// Third should fail
	_, err = sched.Acquire(ctx, ContainerSpec{Image: "alpine:latest"})
	if err == nil {
		t.Fatal("expected error when exceeding max containers")
	}

	// Release one
	c1.Close(ctx)

	// Now should work
	c3, err := sched.Acquire(ctx, ContainerSpec{Image: "alpine:latest"})
	if err != nil {
		t.Fatal(err)
	}

	c2.Close(ctx)
	c3.Close(ctx)
}
