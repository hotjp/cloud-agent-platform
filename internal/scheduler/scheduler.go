// Package scheduler provides container scheduling capabilities as an independent module.
//
// Design goals:
//   - Clean package boundary, no dependency on orchestrator/service/gateway
//   - Interface-driven: Scheduler interface is stable, implementations swappable
//   - Phase 1: cold-start (create → execute → destroy)
//   - Phase 2: warm/hot pool with pause/resume (interface reserved)
//
// This package can be extracted into its own repository in the future.
// Do not import types from orchestrator, service, or gateway packages.
package scheduler

import (
	"context"
	"time"
)

// ─── Core Types ──────────────────────────────────────────────────────────────

// ContainerSpec describes what kind of container to create.
type ContainerSpec struct {
	// Image is the container image (e.g., "cap-worker:latest").
	Image string
	// WorkingDir inside the container.
	WorkingDir string
	// Env is environment variables injected into the container.
	Env map[string]string
	// MemoryLimit in bytes (0 = use default).
	MemoryLimit int64
	// CPUQuota in microseconds (0 = use default).
	CPUQuota int64
	// NetworkDisabled isolates the container from network.
	NetworkDisabled bool
	// Timeout for the entire container lifecycle (create + exec + cleanup).
	Timeout time.Duration
	// VolumeHostPath mounts this host directory into WorkingDir.
	// Empty = no mount. When set, the directory is bind-mounted as WorkingDir.
	VolumeHostPath string
}

// ExecSpec describes a command to execute inside a container.
type ExecSpec struct {
	// Cmd is the command and arguments.
	Cmd []string
	// WorkingDir overrides the container's default.
	WorkingDir string
	// Env is additional environment variables for this execution.
	Env map[string]string
	// Timeout for this command execution.
	Timeout time.Duration
	// Stdin is input piped to the command.
	Stdin []byte
}

// ExecResult holds the outcome of a command execution.
type ExecResult struct {
	ExitCode   int
	Stdout     []byte
	Stderr     []byte
	StartedAt  time.Time
	FinishedAt time.Time
	OOMKilled  bool
}

// Container represents a running or paused container managed by the scheduler.
type Container interface {
	// ID returns the unique container identifier.
	ID() string
	// Exec runs a command inside this container.
	Exec(ctx context.Context, spec ExecSpec) (ExecResult, error)
	// Close releases the container back to the scheduler (may destroy or pool).
	Close(ctx context.Context) error
}

// PoolStats describes the state of the container pool.
type PoolStats struct {
	Total    int
	HotPool  int // paused, content injected, ready to resume
	WarmPool int // running, environment ready, no content
	ColdPool int // not yet created (always 0 for Phase 1)
}

// ─── Scheduler Interface ─────────────────────────────────────────────────────

// Scheduler manages container lifecycle and scheduling.
//
// Phase 1: Acquire creates a fresh container, Release destroys it.
// Phase 2: Acquire tries hot pool → warm pool → cold start, Release returns to pool.
type Scheduler interface {
	// Acquire gets a container matching the spec.
	// Phase 1: always creates a new one.
	// Phase 2: tries pool first, falls back to creation.
	Acquire(ctx context.Context, spec ContainerSpec) (Container, error)

	// Release returns a container. Phase 1: destroys. Phase 2: returns to pool.
	//
	// Deprecated: use Container.Close() instead. This method remains for
	// backward compatibility and batch operations.
	Release(ctx context.Context, id string) error

	// Run is a convenience method: Acquire → Exec → Release.
	// Use this for one-shot executions (Phase 1 default).
	Run(ctx context.Context, spec ContainerSpec, cmd ExecSpec) (ExecResult, error)

	// Stats returns current pool statistics.
	// Phase 1: returns Total only. Phase 2: returns full breakdown.
	Stats(ctx context.Context) PoolStats

	// Start begins background goroutines (health check, idle cleanup, prewarming).
	Start(ctx context.Context) error

	// Stop gracefully shuts down all managed containers.
	Stop(ctx context.Context) error
}

// ─── Phase 2 Extensions (reserved, not implemented yet) ─────────────────────

// WarmUp creates containers in advance and puts them in the warm pool.
// Phase 2 only. Phase 1 implementations should return nil.
type Warmer interface {
	WarmUp(ctx context.Context, spec ContainerSpec, count int) error
}

// SchedulePolicy controls how containers are selected from pools.
type SchedulePolicy struct {
	// PreferHot tries hot pool first (default true).
	PreferHot bool
	// MaxIdleTime before destroying a warm container.
	MaxIdleTime time.Duration
	// MaxPoolSize caps the total number of pooled containers.
	MaxPoolSize int
}
