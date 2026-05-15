// Package sandbox provides Worker sandbox implementations for executing
// agent code in isolated environments.
package sandbox

import (
	"context"
	"fmt"
	"time"
)

// SandboxStatus represents the current state of a sandbox.
type SandboxStatus string

const (
	StatusCreating  SandboxStatus = "creating"
	StatusRunning   SandboxStatus = "running"
	StatusStopping  SandboxStatus = "stopping"
	StatusStopped  SandboxStatus = "stopped"
	StatusError    SandboxStatus = "error"
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
	// GitOptions configures post-execution git commit/push.
	GitOptions *GitOptions
}

// GitOptions configures git operations after execution.
type GitOptions struct {
	DoGitCommit bool
	CommitMsg   string
	Branch      string
	Remote      string
}

// ExecResult contains the result of command execution.
type ExecResult struct {
	// ExitCode is the process exit code.
	ExitCode int
	// Stdout contains standard output.
	Stdout []byte
	// Stderr contains standard error.
	Stderr []byte
	// Error indicates any execution error.
	Error error
	// StartedAt is when the command started.
	StartedAt time.Time
	// FinishedAt is when the command finished.
	FinishedAt time.Time
	// OOMKilled indicates if the process was killed due to OOM.
	OOMKilled bool
}

// Sandbox defines the interface for Worker sandboxes.
// Implementations must be stateless (can be recreated after failure).
type Sandbox interface {
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
	return fmt.Sprintf("sandbox not found: %s", e.ID)
}

func (e *ErrSandboxNotFound) Is(target error) bool {
	_, ok := target.(*ErrSandboxNotFound)
	return ok
}

// ErrSandboxTimeout is returned when a sandbox operation times out.
type ErrSandboxTimeout struct {
	Operation string
	Timeout   time.Duration
}

func (e *ErrSandboxTimeout) Error() string {
	return fmt.Sprintf("sandbox operation %q timed out after %v", e.Operation, e.Timeout)
}

func (e *ErrSandboxTimeout) Is(target error) bool {
	_, ok := target.(*ErrSandboxTimeout)
	return ok
}

// ErrSandboxCreationFailed is returned when sandbox creation fails.
type ErrSandboxCreationFailed struct {
	Image string
	Cause error
}

func (e *ErrSandboxCreationFailed) Error() string {
	return fmt.Sprintf("failed to create sandbox with image %q: %v", e.Image, e.Cause)
}

func (e *ErrSandboxCreationFailed) Unwrap() error {
	return e.Cause
}

// ErrSandboxExecutionFailed is returned when command execution fails.
type ErrSandboxExecutionFailed struct {
	SandboxID string
	Cmd       []string
	Cause     error
}

func (e *ErrSandboxExecutionFailed) Error() string {
	return fmt.Sprintf("failed to execute %v in sandbox %q: %v", e.Cmd, e.SandboxID, e.Cause)
}

func (e *ErrSandboxExecutionFailed) Unwrap() error {
	return e.Cause
}

// ValidateOptions validates sandbox options and returns an error if invalid.
func ValidateOptions(opts SandboxOptions) error {
	if opts.Image == "" {
		return fmt.Errorf("image is required")
	}
	if opts.CPUQuota > 0 && opts.CPUPeriod == 0 {
		return fmt.Errorf("CPUPeriod must be set when CPUQuota is specified")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	return nil
}

// ValidateExecOptions validates exec options and returns an error if invalid.
func ValidateExecOptions(opts ExecOptions) error {
	if len(opts.Cmd) == 0 {
		return fmt.Errorf("command is required")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	return nil
}