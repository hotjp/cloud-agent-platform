// Package scheduler - Docker backend adapter.
package scheduler

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

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

	// If VolumeHostPath is set, we need to create the container with a bind mount.
	// The existing DockerBackend.Create doesn't support this, so we create directly.
	if spec.VolumeHostPath != "" {
		return a.createWithVolume(ctx, spec)
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

// createWithVolume creates a Docker container with a bind-mounted host directory.
func (a *DockerBackendAdapter) createWithVolume(ctx context.Context, spec ContainerSpec) (string, error) {
	args := []string{"run", "-d"}

	// Bind mount host directory → container WorkingDir
	if spec.WorkingDir == "" {
		spec.WorkingDir = "/workspace"
	}
	args = append(args, "-v", fmt.Sprintf("%s:%s", spec.VolumeHostPath, spec.WorkingDir))

	// Environment variables
	for k, v := range spec.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Resource limits
	if spec.MemoryLimit > 0 {
		args = append(args, "--memory", fmt.Sprintf("%d", spec.MemoryLimit))
	}
	if spec.CPUQuota > 0 {
		args = append(args, "--cpu-quota", fmt.Sprintf("%d", spec.CPUQuota))
	}

	if spec.NetworkDisabled {
		args = append(args, "--network", "none")
	}

	args = append(args, spec.Image)

	a.logger.Debug("creating container with volume mount",
		zap.String("image", spec.Image),
		zap.String("volume", spec.VolumeHostPath),
		zap.String("mount", spec.WorkingDir),
	)

	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run with volume: %w\n%s", err, string(out))
	}

	return strings.TrimSpace(string(out)), nil
}
