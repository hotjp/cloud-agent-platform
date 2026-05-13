// Package worker provides domain interfaces for worker management.
// This package contains interfaces that define the contract between
// the domain layer and plugin implementations for sandbox-backed workers.
package worker

import (
	"context"
	"time"
)

// SandboxOptions contains configuration for creating a sandbox.
type SandboxOptions struct {
	// Image is the container image to use (e.g., "alpine:latest").
	Image string
	// WorkingDir is the working directory inside the sandbox.
	WorkingDir string
	// Envvars are environment variables to set in the sandbox.
	Envvars map[string]string
	// CPUPeriod limits CPU usage (in microseconds).
	CPUPeriod int64
	// CPUQuota limits CPU time (in microseconds).
	CPUQuota int64
	// MemoryLimit limits memory usage in bytes.
	MemoryLimit int64
	// NetworkDisabled disables network access when true.
	NetworkDisabled bool
	// ReadonlyRootfs makes root filesystem read-only.
	ReadonlyRootfs bool
	// Timeout is the maximum duration for sandbox creation.
	Timeout time.Duration
}

// ExecOptions contains configuration for executing a command in a sandbox.
type ExecOptions struct {
	// Cmd is the command and arguments to execute.
	Cmd []string
	// WorkingDir overrides the sandbox default working directory.
	WorkingDir string
	// Envvars are additional environment variables.
	Envvars map[string]string
	// Timeout is the maximum execution time.
	Timeout time.Duration
	// Stdin provides input to the command.
	Stdin []byte
}

// ExecResult contains the result of command execution.
type ExecResult struct {
	// ExitCode is the process exit code.
	ExitCode int
	// Stdout contains standard output.
	Stdout []byte
	// Stderr contains standard error.
	Stderr []byte
	// StartedAt is when the command started.
	StartedAt time.Time
	// FinishedAt is when the command finished.
	FinishedAt time.Time
	// OOMKilled indicates if the process was killed due to OOM.
	OOMKilled bool
}

// SandboxStatus represents the current state of a sandbox.
type SandboxStatus string

const (
	StatusCreating  SandboxStatus = "creating"
	StatusRunning  SandboxStatus = "running"
	StatusStopping SandboxStatus = "stopping"
	StatusStopped SandboxStatus = "stopped"
	StatusError   SandboxStatus = "error"
)

// HealthStatus represents the health status of a sandbox.
type HealthStatus struct {
	// IsHealthy indicates if the sandbox is healthy.
	IsHealthy bool
	// Message provides additional information about health status.
	Message string
	// LastCheck is when the health check was performed.
	LastCheck time.Time
}

// SandboxBackend defines the interface for worker sandbox backends.
// Implementations must be stateless (can be recreated after failure).
// This interface extends the basic Sandbox operations with health checking
// for use by the WorkerManager.
type SandboxBackend interface {
	// Create creates a new sandbox instance and returns its ID.
	Create(ctx context.Context, opts SandboxOptions) (string, error)

	// Exec executes a command in the specified sandbox.
	Exec(ctx context.Context, id string, opts ExecOptions) (ExecResult, error)

	// Destroy terminates and removes a sandbox.
	Destroy(ctx context.Context, id string) error

	// Status returns the current status of a sandbox.
	Status(ctx context.Context, id string) (SandboxStatus, error)

	// HealthCheck checks the health of a sandbox and returns detailed status.
	HealthCheck(ctx context.Context, id string) (HealthStatus, error)
}

// ErrSandboxNotFound is returned when a sandbox with the given ID doesn't exist.
type ErrSandboxNotFound struct {
	ID string
}

func (e *ErrSandboxNotFound) Error() string {
	return "sandbox not found: " + e.ID
}

// ErrSandboxTimeout is returned when a sandbox operation times out.
type ErrSandboxTimeout struct {
	Operation string
	Timeout   time.Duration
}

func (e *ErrSandboxTimeout) Error() string {
	return "sandbox operation " + e.Operation + " timed out after " + e.Timeout.String()
}

// ErrSandboxCreationFailed is returned when sandbox creation fails.
type ErrSandboxCreationFailed struct {
	Image string
	Cause error
}

func (e *ErrSandboxCreationFailed) Error() string {
	return "failed to create sandbox with image " + e.Image + ": " + e.Cause.Error()
}

func (e *ErrSandboxCreationFailed) Unwrap() error {
	return e.Cause
}

// ErrSandboxUnhealthy is returned when a sandbox fails health check.
type ErrSandboxUnhealthy struct {
	SandboxID string
	Message   string
}

func (e *ErrSandboxUnhealthy) Error() string {
	return "sandbox " + e.SandboxID + " is unhealthy: " + e.Message
}
