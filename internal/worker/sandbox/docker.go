// Package sandbox provides Worker sandbox implementations.
package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"go.uber.org/zap"
)

// DockerSandbox implements Sandbox interface using Docker containers.
type DockerSandbox struct {
	client    *docker.Client
	logger    *zap.Logger
	mu        sync.RWMutex
	containers map[string]dockerCreateResponse
}

type dockerCreateResponse struct {
	config     docker.Config
	hostConfig docker.HostConfig
	status     SandboxStatus
}

// NewDockerSandbox creates a new Docker-based sandbox.
func NewDockerSandbox(logger *zap.Logger) (*DockerSandbox, error) {
	cli, err := docker.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &DockerSandbox{
		client:    cli,
		logger:    logger,
		containers: make(map[string]dockerCreateResponse),
	}, nil
}

// NewDockerSandboxWithClient creates a new Docker-based sandbox with a provided client.
func NewDockerSandboxWithClient(cli *docker.Client, logger *zap.Logger) *DockerSandbox {
	return &DockerSandbox{
		client:    cli,
		logger:    logger,
		containers: make(map[string]dockerCreateResponse),
	}
}

// Create implements Sandbox.Create using Docker containers.
func (s *DockerSandbox) Create(ctx context.Context, opts SandboxOptions) (string, error) {
	if err := ValidateOptions(opts); err != nil {
		return "", err
	}

	// Create a unique ID for this sandbox
	id := generateSandboxID()

	// Build container config
	containerCfg := docker.Config{
		Image:        opts.Image,
		WorkingDir:   opts.WorkingDir,
		Env:          buildEnvVars(opts.Envvars),
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		OpenStdin:    true,
		StdinOnce:    true,
	}

	// Build host config for resource limits
	hostCfg := docker.HostConfig{
		Memory:        opts.MemoryLimit,
		CPUPeriod:     opts.CPUPeriod,
		CPUQuota:      opts.CPUQuota,
		AutoRemove:    false,
		NetworkMode:   "bridge",
		ReadonlyRootfs: opts.ReadonlyRootfs,
	}

	// Disable network if requested
	if opts.NetworkDisabled {
		hostCfg.NetworkMode = "none"
	}

	// Create the container
	resp, err := s.client.CreateContainer(docker.CreateContainerOptions{
		Name: id,
		Config: &containerCfg,
		HostConfig: &hostCfg,
		NetworkingConfig: &docker.NetworkingConfig{
			EndpointsConfig: map[string]*docker.EndpointConfig{},
		},
	})
	if err != nil {
		s.logger.Error("failed to create Docker container",
			zap.String("sandbox_id", id),
			zap.String("image", opts.Image),
			zap.Error(err),
		)
		return "", &ErrSandboxCreationFailed{Image: opts.Image, Cause: err}
	}

	// Store container info for later reference
	s.mu.Lock()
	s.containers[id] = dockerCreateResponse{
		config:     containerCfg,
		hostConfig: hostCfg,
		status:    StatusRunning,
	}
	s.mu.Unlock()

	// Start the container
	if err := s.client.StartContainer(resp.ID, nil); err != nil {
		s.logger.Error("failed to start Docker container",
			zap.String("sandbox_id", id),
			zap.String("container_id", resp.ID),
			zap.Error(err),
		)
		// Clean up on start failure
		_ = s.client.RemoveContainer(docker.RemoveContainerOptions{ID: resp.ID, Force: true})
		return "", &ErrSandboxCreationFailed{Image: opts.Image, Cause: err}
	}

	s.logger.Info("Docker sandbox created",
		zap.String("sandbox_id", id),
		zap.String("container_id", resp.ID),
		zap.String("image", opts.Image),
	)

	return id, nil
}

// Exec implements Sandbox.Exec by running a command in a Docker container.
func (s *DockerSandbox) Exec(ctx context.Context, id string, opts ExecOptions) (ExecResult, error) {
	if err := ValidateExecOptions(opts); err != nil {
		return ExecResult{}, err
	}

	// Get container info
	s.mu.RLock()
	_, ok := s.containers[id]
	s.mu.RUnlock()
	if !ok {
		return ExecResult{}, &ErrSandboxNotFound{ID: id}
	}

	// Create exec instance
	execCfg := docker.CreateExecOptions{
		Container:    id,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          opts.Cmd,
		WorkingDir:   opts.WorkingDir,
		Env:          buildEnvVars(opts.Envvars),
	}

	execID, err := s.client.CreateExec(execCfg)
	if err != nil {
		return ExecResult{}, &ErrSandboxExecutionFailed{SandboxID: id, Cmd: opts.Cmd, Cause: err}
	}

	// Set up exec start
	startCfg := docker.StartExecOptions{
		Detach: false,
	}

	// Handle stdin if provided
	if len(opts.Stdin) > 0 {
		startCfg.InputStream = bytes.NewReader(opts.Stdin)
	}

	// Set timeout
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Collect output
	var stdout, stderr bytes.Buffer
	startCfg.OutputStream = &stdout
	startCfg.ErrorStream = &stderr

	// Start exec
	err = s.client.StartExec(execID.ID, startCfg)
	if err != nil {
		return ExecResult{
			Stdout: stdout.Bytes(),
			Stderr: stderr.Bytes(),
			Error:  err,
		}, &ErrSandboxExecutionFailed{SandboxID: id, Cmd: opts.Cmd, Cause: err}
	}

	// Wait for exec to complete
	info, err := s.client.InspectExec(execID.ID)
	if err != nil {
		return ExecResult{
			Stdout: stdout.Bytes(),
			Stderr: stderr.Bytes(),
			Error:  err,
		}, err
	}

	exitCode := info.ExitCode

	s.logger.Debug("command executed in Docker sandbox",
		zap.String("sandbox_id", id),
		zap.Strings("cmd", opts.Cmd),
		zap.Int("exit_code", exitCode),
	)

	return ExecResult{
		ExitCode:   exitCode,
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
	}, nil
}

// Destroy implements Sandbox.Destroy by stopping and removing a container.
func (s *DockerSandbox) Destroy(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if container exists
	if _, ok := s.containers[id]; !ok {
		// Try to find by container ID
		containers, err := s.client.ListContainers(docker.ListContainersOptions{All: true})
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}
		found := false
		for _, c := range containers {
			if c.ID == id || strings.HasPrefix(c.ID, id) {
				id = c.ID // Use full container ID
				found = true
				break
			}
		}
		if !found {
			return &ErrSandboxNotFound{ID: id}
		}
	}

	// Update status
	if c, ok := s.containers[id]; ok {
		c.status = StatusStopping
		s.containers[id] = c
	}

	// Stop the container
	if err := s.client.StopContainer(id, 10); err != nil {
		s.logger.Warn("failed to stop container gracefully, forcing removal",
			zap.String("sandbox_id", id),
			zap.Error(err),
		)
	}

	// Remove the container
	if err := s.client.RemoveContainer(docker.RemoveContainerOptions{ID: id, Force: true}); err != nil {
		return fmt.Errorf("failed to remove container %q: %w", id, err)
	}

	// Remove from tracking
	delete(s.containers, id)

	s.logger.Info("Docker sandbox destroyed",
		zap.String("sandbox_id", id),
	)

	return nil
}

// Status implements Sandbox.Status by checking container state.
func (s *DockerSandbox) Status(ctx context.Context, id string) (SandboxStatus, error) {
	s.mu.RLock()
	c, ok := s.containers[id]
	s.mu.RUnlock()

	if !ok {
		// Try to find by container ID
		containers, err := s.client.ListContainers(docker.ListContainersOptions{All: true})
		if err != nil {
			return "", fmt.Errorf("failed to list containers: %w", err)
		}
		for _, c := range containers {
			if c.ID == id || strings.HasPrefix(c.ID, id) {
				return mapDockerState(c.State), nil
			}
		}
		return "", &ErrSandboxNotFound{ID: id}
	}

	return c.status, nil
}

// HealthCheck implements Sandbox.HealthCheck by performing a detailed health check.
func (s *DockerSandbox) HealthCheck(ctx context.Context, id string) (HealthStatus, error) {
	s.mu.RLock()
	c, ok := s.containers[id]
	s.mu.RUnlock()

	if !ok {
		// Try to find by container ID
		containers, err := s.client.ListContainers(docker.ListContainersOptions{All: true})
		if err != nil {
			return HealthStatus{}, &ErrSandboxNotFound{ID: id}
		}
		found := false
		for _, c := range containers {
			if c.ID == id || strings.HasPrefix(c.ID, id) {
				found = true
				if c.State != "running" {
					return HealthStatus{
						IsHealthy: false,
						Message:   "container is not running, state: " + c.State,
						LastCheck: time.Now(),
					}, nil
				}
				return HealthStatus{
					IsHealthy: true,
					Message:   "container is running",
					LastCheck: time.Now(),
				}, nil
			}
		}
		if !found {
			return HealthStatus{}, &ErrSandboxNotFound{ID: id}
		}
	}

	if c.status != StatusRunning {
		return HealthStatus{
			IsHealthy: false,
			Message:   "sandbox status is " + string(c.status),
			LastCheck: time.Now(),
		}, nil
	}

	// Try to execute a simple health check command
	execCfg := docker.CreateExecOptions{
		Container:    id,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"echo", "health"},
	}

	execID, err := s.client.CreateExec(execCfg)
	if err != nil {
		return HealthStatus{
			IsHealthy: false,
			Message:   "failed to create health check exec: " + err.Error(),
			LastCheck: time.Now(),
		}, nil
	}

	var stdout, stderr bytes.Buffer
	startCfg := docker.StartExecOptions{
		OutputStream: &stdout,
		ErrorStream: &stderr,
	}

	err = s.client.StartExec(execID.ID, startCfg)
	if err != nil {
		return HealthStatus{
			IsHealthy: false,
			Message:   "health check exec failed: " + err.Error(),
			LastCheck: time.Now(),
		}, nil
	}

	if strings.TrimSpace(stdout.String()) != "health" {
		return HealthStatus{
			IsHealthy: false,
			Message:   "health check returned unexpected output",
			LastCheck: time.Now(),
		}, nil
	}

	return HealthStatus{
		IsHealthy: true,
		Message:   "sandbox is healthy",
		LastCheck: time.Now(),
	}, nil
}

// mapDockerState maps Docker container states to SandboxStatus.
func mapDockerState(state string) SandboxStatus {
	switch strings.ToLower(state) {
	case "created":
		return StatusCreating
	case "running", "restarting":
		return StatusRunning
	case "paused":
		return StatusStopped
	case "exited", "dead":
		return StatusStopped
	default:
		return StatusError
	}
}

// buildEnvVars converts a map to Docker-style environment variables.
func buildEnvVars(envvars map[string]string) []string {
	if envvars == nil {
		return nil
	}
	env := make([]string, 0, len(envvars))
	for k, v := range envvars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// generateSandboxID creates a unique sandbox identifier.
func generateSandboxID() string {
	return fmt.Sprintf("sandbox-%d", time.Now().UnixNano())
}

// Close releases Docker client resources.
func (s *DockerSandbox) Close() error {
	return nil // go-dockerclient doesn't require explicit close
}

// GetContainerInfo returns container information for debugging.
func (s *DockerSandbox) GetContainerInfo(id string) (docker.Config, docker.HostConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.containers[id]
	if !ok {
		return docker.Config{}, docker.HostConfig{}, &ErrSandboxNotFound{ID: id}
	}
	return c.config, c.hostConfig, nil
}

// EnsureImagePulled pulls an image if not present.
func (s *DockerSandbox) EnsureImagePulled(ctx context.Context, image string) error {
	// Check if image exists
	_, err := s.client.InspectImage(image)
	if err == nil {
		return nil
	}

	s.logger.Info("pulling Docker image",
		zap.String("image", image),
	)

	// Pull the image
	err = s.client.PullImage(docker.PullImageOptions{
		Repository: image,
	}, docker.AuthConfiguration{})
	if err != nil {
		return fmt.Errorf("failed to pull image %q: %w", image, err)
	}

	return nil
}
