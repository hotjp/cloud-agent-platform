// Package scheduler - Docker backend adapter.
package scheduler

import (
	"context"
	"fmt"

	"github.com/cloud-agent-platform/cap/internal/domain/worker"
	"go.uber.org/zap"
)

// DockerBackendAdapter adapts the existing DockerBackend (plugins/workermanager)
// to the scheduler.Backend interface. This keeps scheduler independent of Docker details.
type DockerBackendAdapter struct {
	backend worker.SandboxBackend
	logger  *zap.Logger
}

// NewDockerBackend creates a scheduler Backend backed by Docker.
func NewDockerBackend(sandboxBackend worker.SandboxBackend, logger *zap.Logger) Backend {
	return &DockerBackendAdapter{
		backend: sandboxBackend,
		logger:  logger,
	}
}

func (a *DockerBackendAdapter) Create(ctx context.Context, spec ContainerSpec) (string, error) {
	sandboxOpts := worker.SandboxOptions{
		Image:           spec.Image,
		WorkingDir:      spec.WorkingDir,
		Envvars:         spec.Env,
		MemoryLimit:     spec.MemoryLimit,
		CPUQuota:        spec.CPUQuota,
		NetworkDisabled: spec.NetworkDisabled,
		Timeout:         spec.Timeout,
	}

	id, err := a.backend.Create(ctx, sandboxOpts)
	if err != nil {
		return "", fmt.Errorf("docker backend create: %w", err)
	}
	return id, nil
}

func (a *DockerBackendAdapter) Exec(ctx context.Context, containerID string, spec ExecSpec) (ExecResult, error) {
	execOpts := worker.ExecOptions{
		Cmd:        spec.Cmd,
		WorkingDir: spec.WorkingDir,
		Envvars:    spec.Env,
		Timeout:    spec.Timeout,
		Stdin:      spec.Stdin,
	}

	result, err := a.backend.Exec(ctx, containerID, execOpts)
	if err != nil {
		return ExecResult{}, fmt.Errorf("docker backend exec: %w", err)
	}

	return ExecResult{
		ExitCode:   result.ExitCode,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		StartedAt:  result.StartedAt,
		FinishedAt: result.FinishedAt,
		OOMKilled:  result.OOMKilled,
	}, nil
}

func (a *DockerBackendAdapter) Destroy(ctx context.Context, containerID string) error {
	if err := a.backend.Destroy(ctx, containerID); err != nil {
		return fmt.Errorf("docker backend destroy: %w", err)
	}
	return nil
}
