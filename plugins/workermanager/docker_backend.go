// Package workermanager provides Worker lifecycle management with Docker sandbox backend.
package workermanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain/worker"
	"github.com/fsouza/go-dockerclient"
	"go.uber.org/zap"
)

// DockerBackendConfig contains configuration for the DockerBackend.
type DockerBackendConfig struct {
	// DefaultImage is the default container image to use.
	DefaultImage string
	// SeccompProfilePath is the path to the seccomp profile file.
	SeccompProfilePath string
	// DefaultMemoryLimit is the default memory limit in bytes.
	DefaultMemoryLimit int64
	// DefaultCPUQuota is the default CPU quota in microseconds.
	DefaultCPUQuota int64
	// DefaultPidsLimit is the default process limit.
	DefaultPidsLimit int64
	// NetworkMode sets the default network mode.
	NetworkMode string
	// AutoRemove enables automatic container removal on stop.
	AutoRemove bool
}

// DefaultDockerBackendConfig returns the default Docker backend configuration.
func DefaultDockerBackendConfig() DockerBackendConfig {
	return DockerBackendConfig{
		DefaultImage:      "cap-worker:latest",
		SeccompProfilePath: "", // disabled for dev; set seccomp-worker.json for production
		DefaultMemoryLimit: 2 * 1024 * 1024 * 1024, // 2GB
		DefaultCPUQuota:   100000,                   // 1 CPU
		DefaultPidsLimit:  512,
		NetworkMode:      "bridge",
		AutoRemove:       false,
	}
}

// seccompProfile represents a seccomp filter profile.
type seccompProfile struct {
	Name        string            `json:"name"`
	Namespaces  []string          `json:"namespaces,omitempty"`
	Caps        []string          `json:"caps,omitempty"`
	AltNames    []string          `json:"altNames,omitempty"`
	Syscalls    []seccompSyscall `json:"syscalls"`
}

// seccompSyscall represents a syscall rule in seccomp profile.
type seccompSyscall struct {
	Names  []string          `json:"names"`
	Action string            `json:"action"`
	Args   []seccompArg     `json:"args,omitempty"`
}

// seccompArg represents an argument matcher in seccomp rule.
type seccompArg struct {
	Index    uint   `json:"index"`
	Value    uint64 `json:"value"`
	ValueTwo uint64 `json:"valueTwo,omitempty"`
	Op       string `json:"op"`
}

// DockerBackend implements worker.SandboxBackend using Docker containers.
// It provides secure container isolation with seccomp, capability dropping,
// resource limits, and network isolation.
type DockerBackend struct {
	client *docker.Client
	cfg    DockerBackendConfig
	logger *zap.Logger

	mu        sync.RWMutex
	containers map[string]dockerContainerInfo

	// seccompProfile holds the loaded seccomp profile
	seccompProfile *seccompProfile
}

// dockerContainerInfo holds information about a managed container.
type dockerContainerInfo struct {
	id        string
	image     string
	status    worker.SandboxStatus
	createdAt time.Time
	lastUsed  time.Time
}

// NewDockerBackend creates a new DockerBackend instance.
func NewDockerBackend(cfg DockerBackendConfig, logger *zap.Logger) (*DockerBackend, error) {
	cli, err := docker.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	backend := &DockerBackend{
		client:     cli,
		cfg:        cfg,
		logger:     logger,
		containers: make(map[string]dockerContainerInfo),
	}

	// Load seccomp profile if configured
	if cfg.SeccompProfilePath != "" {
		profile, err := loadSeccompProfile(cfg.SeccompProfilePath)
		if err != nil {
			logger.Warn("failed to load seccomp profile, using default restrictive profile",
				zap.String("path", cfg.SeccompProfilePath),
				zap.Error(err),
			)
			// Use a built-in restrictive profile
			backend.seccompProfile = defaultRestrictiveSeccompProfile()
		} else {
			backend.seccompProfile = profile
		}
	}

	return backend, nil
}

// NewDockerBackendWithClient creates a new DockerBackend with a provided client.
func NewDockerBackendWithClient(cfg DockerBackendConfig, cli *docker.Client, logger *zap.Logger) *DockerBackend {
	backend := &DockerBackend{
		client:     cli,
		cfg:        cfg,
		logger:     logger,
		containers: make(map[string]dockerContainerInfo),
	}

	if cfg.SeccompProfilePath != "" {
		profile, err := loadSeccompProfile(cfg.SeccompProfilePath)
		if err != nil {
			logger.Warn("failed to load seccomp profile",
				zap.String("path", cfg.SeccompProfilePath),
				zap.Error(err),
			)
			backend.seccompProfile = defaultRestrictiveSeccompProfile()
		} else {
			backend.seccompProfile = profile
		}
	} else {
		backend.seccompProfile = defaultRestrictiveSeccompProfile()
	}

	return backend
}

// Create creates a new sandbox container with security hardening.
func (b *DockerBackend) Create(ctx context.Context, opts worker.SandboxOptions) (string, error) {
	image := opts.Image
	if image == "" {
		image = b.cfg.DefaultImage
	}

	id := generateContainerID()

	// Build environment variables
	env := buildEnvVars(opts.Envvars)


	// Container config
	// Don't set Cmd — let the image's ENTRYPOINT run (e.g., sleep infinity for cap-worker)
	containerCfg := &docker.Config{
		Image:        image,
		WorkingDir:   opts.WorkingDir,
		Env:          env,
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		OpenStdin:    false,
		StdinOnce:    false,
	}

	// Build host config with security and resource limits
	hostCfg := b.buildHostConfig(opts)

	// Create container
	resp, err := b.client.CreateContainer(docker.CreateContainerOptions{
		Name:             id,
		Config:           containerCfg,
		HostConfig:       &hostCfg,
		NetworkingConfig: nil,
	})
	if err != nil {
		b.logger.Error("failed to create container",
			zap.String("sandbox_id", id),
			zap.String("image", image),
			zap.Error(err),
		)
		return "", &worker.ErrSandboxCreationFailed{Image: image, Cause: err}
	}

	// Start the container
	if err := b.client.StartContainer(resp.ID, nil); err != nil {
		b.logger.Error("failed to start container",
			zap.String("sandbox_id", id),
			zap.String("container_id", resp.ID),
			zap.Error(err),
		)
		// Clean up on start failure
		_ = b.client.RemoveContainer(docker.RemoveContainerOptions{ID: resp.ID, Force: true})
		return "", &worker.ErrSandboxCreationFailed{Image: image, Cause: err}
	}

	// Store container info
	b.mu.Lock()
	b.containers[id] = dockerContainerInfo{
		id:        resp.ID,
		image:     image,
		status:    worker.StatusRunning,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}
	b.mu.Unlock()

	b.logger.Info("Docker sandbox created",
		zap.String("sandbox_id", id),
		zap.String("container_id", resp.ID),
		zap.String("image", image),
	)

	return id, nil
}

// buildHostConfig builds the Docker host config with security and resource limits.
func (b *DockerBackend) buildHostConfig(opts worker.SandboxOptions) docker.HostConfig {
	// Determine network mode
	networkMode := b.cfg.NetworkMode
	if opts.NetworkDisabled {
		networkMode = "none"
	}

	// Determine memory limit
	memoryLimit := b.cfg.DefaultMemoryLimit
	if opts.MemoryLimit > 0 {
		memoryLimit = opts.MemoryLimit
	}

	// Determine CPU quota
	cpuQuota := b.cfg.DefaultCPUQuota
	if opts.CPUQuota > 0 {
		cpuQuota = opts.CPUQuota
	}

	// CPU period
	cpuPeriod := opts.CPUPeriod
	if cpuPeriod == 0 {
		cpuPeriod = 100000 // 100ms default
	}

	hostCfg := docker.HostConfig{
		Memory:        memoryLimit,
		CPUPeriod:     cpuPeriod,
		CPUQuota:      cpuQuota,
		PidsLimit:     &b.cfg.DefaultPidsLimit,
		NetworkMode:   networkMode,
		ReadonlyRootfs: opts.ReadonlyRootfs,
		AutoRemove:    b.cfg.AutoRemove,
		// Security options
		SecurityOpt: []string{
			"no-new-privileges:true",
		},
		// Drop all capabilities
		CapDrop: []string{"ALL"},
		// Additional read-only paths
		ReadonlyPaths: []string{
			"/proc/sys",
			"/proc/sysrq-trigger",
			"/proc/irq",
			"/proc/bus",
		},
		// TMPFS mounts for writable temporary space
		Tmpfs: map[string]string{
			"/tmp": "noexec,nosuid,size=512M",
		},
		// Ulimits for resource constraints
		Ulimits: []docker.ULimit{
			{Name: "nproc", Soft: 512, Hard: 1024},
			{Name: "nofile", Soft: 1024, Hard: 2048},
		},
	}

	// Disk quota (if available via device mapper)
	// Note: storage-opt requires overlay over xfs with pquota, not available on macOS Docker Desktop.
	// Enable this only on Linux production hosts.
	if runtime.GOOS != "darwin" {
		hostCfg.StorageOpt = map[string]string{
			"size": "10G",
		}
	}

	// Add seccomp profile if available
	if b.seccompProfile != nil {
		seccompStr, err := seccompProfileToString(b.seccompProfile)
		if err == nil {
			hostCfg.SecurityOpt = append(hostCfg.SecurityOpt, "seccomp="+seccompStr)
		}
	}

	return hostCfg
}

// Exec executes a command in the sandbox container.
func (b *DockerBackend) Exec(ctx context.Context, id string, opts worker.ExecOptions) (worker.ExecResult, error) {
	if len(opts.Cmd) == 0 {
		return worker.ExecResult{}, fmt.Errorf("command is required")
	}

	// Check container exists
	b.mu.RLock()
	info, ok := b.containers[id]
	b.mu.RUnlock()
	if !ok {
		return worker.ExecResult{}, &worker.ErrSandboxNotFound{ID: id}
	}

	execEnv := buildEnvVars(opts.Envvars)

	// Create exec instance
	execCfg := docker.CreateExecOptions{
		Container:    info.id,
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          opts.Cmd,
		WorkingDir:   opts.WorkingDir,
		Env:          execEnv,
	}

	execID, err := b.client.CreateExec(execCfg)
	if err != nil {
		return worker.ExecResult{}, fmt.Errorf("failed to create exec: %w", err)
	}

	// Set up start options
	startCfg := docker.StartExecOptions{
		Detach: false,
	}

	// Collect output
	var stdout, stderr bytes.Buffer
	startCfg.OutputStream = &stdout
	startCfg.ErrorStream = &stderr

	// Set timeout for exec
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	startTime := time.Now()

	// Start exec
	err = b.client.StartExec(execID.ID, startCfg)
	if err != nil {
		return worker.ExecResult{
			Stdout:     stdout.Bytes(),
			Stderr:     stderr.Bytes(),
			StartedAt:  startTime,
			FinishedAt: time.Now(),
		}, fmt.Errorf("failed to start exec: %w", err)
	}

	// Wait for exec to complete
	infoResp, err := b.client.InspectExec(execID.ID)
	if err != nil {
		return worker.ExecResult{
			Stdout:     stdout.Bytes(),
			Stderr:     stderr.Bytes(),
			StartedAt:  startTime,
			FinishedAt: time.Now(),
		}, fmt.Errorf("failed to inspect exec: %w", err)
	}

	finishTime := time.Now()

	// Update last used
	b.mu.Lock()
	if c, exists := b.containers[id]; exists {
		c.lastUsed = finishTime
		b.containers[id] = c
	}
	b.mu.Unlock()

	b.logger.Debug("command executed in Docker sandbox",
		zap.String("sandbox_id", id),
		zap.Strings("cmd", opts.Cmd),
		zap.Int("exit_code", infoResp.ExitCode),
		zap.Duration("duration", finishTime.Sub(startTime)),
	)

	return worker.ExecResult{
		ExitCode:   infoResp.ExitCode,
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),
		StartedAt:  startTime,
		FinishedAt: finishTime,
	}, nil
}

// Destroy stops and removes a container.
func (b *DockerBackend) Destroy(ctx context.Context, id string) error {
	b.mu.Lock()
	info, ok := b.containers[id]
	if !ok {
		b.mu.Unlock()
		return &worker.ErrSandboxNotFound{ID: id}
	}

	// Mark as stopping
	info.status = worker.StatusStopping
	b.containers[id] = info
	b.mu.Unlock()

	// Stop the container gracefully first (with 10 second timeout)
	if err := b.client.StopContainer(info.id, 10); err != nil {
		b.logger.Warn("failed to stop container gracefully, forcing removal",
			zap.String("sandbox_id", id),
			zap.String("container_id", info.id),
			zap.Error(err),
		)
	}

	// Remove the container
	if err := b.client.RemoveContainer(docker.RemoveContainerOptions{
		ID:    info.id,
		Force: true,
	}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	// Remove from tracking
	b.mu.Lock()
	delete(b.containers, id)
	b.mu.Unlock()

	b.logger.Info("Docker sandbox destroyed",
		zap.String("sandbox_id", id),
		zap.String("container_id", info.id),
	)

	return nil
}

// Status returns the current status of a sandbox.
func (b *DockerBackend) Status(ctx context.Context, id string) (worker.SandboxStatus, error) {
	b.mu.RLock()
	info, ok := b.containers[id]
	b.mu.RUnlock()

	if !ok {
		return "", &worker.ErrSandboxNotFound{ID: id}
	}

	// Query Docker for current container state
	containers, err := b.client.ListContainers(docker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"id": {info.id},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		return "", &worker.ErrSandboxNotFound{ID: id}
	}

	return mapContainerState(containers[0].State), nil
}

// HealthCheck checks the health of a sandbox.
func (b *DockerBackend) HealthCheck(ctx context.Context, id string) (worker.HealthStatus, error) {
	b.mu.RLock()
	info, ok := b.containers[id]
	b.mu.RUnlock()

	if !ok {
		return worker.HealthStatus{}, &worker.ErrSandboxNotFound{ID: id}
	}

	// Query Docker for current container state
	containers, err := b.client.ListContainers(docker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"id": {info.id},
		},
	})
	if err != nil {
		return worker.HealthStatus{}, &worker.ErrSandboxNotFound{ID: id}
	}

	if len(containers) == 0 {
		return worker.HealthStatus{}, &worker.ErrSandboxNotFound{ID: id}
	}

	container := containers[0]

	// Check if container is running
	if container.State != "running" {
		return worker.HealthStatus{
			IsHealthy: false,
			Message:   fmt.Sprintf("container is not running, state: %s", container.State),
			LastCheck: time.Now(),
		}, nil
	}

	// Try to execute a simple health check command
	execCfg := docker.CreateExecOptions{
		Container:    info.id,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"echo", "health"},
	}

	execID, err := b.client.CreateExec(execCfg)
	if err != nil {
		return worker.HealthStatus{
			IsHealthy: false,
			Message:   fmt.Sprintf("failed to create health check exec: %v", err),
			LastCheck: time.Now(),
		}, nil
	}

	var stdout, stderr bytes.Buffer
	startCfg := docker.StartExecOptions{
		OutputStream: &stdout,
		ErrorStream: &stderr,
	}

	err = b.client.StartExec(execID.ID, startCfg)
	if err != nil {
		return worker.HealthStatus{
			IsHealthy: false,
			Message:   fmt.Sprintf("health check exec failed: %v", err),
			LastCheck: time.Now(),
		}, nil
	}

	output := strings.TrimSpace(stdout.String())
	if output != "health" {
		return worker.HealthStatus{
			IsHealthy: false,
			Message:   fmt.Sprintf("health check returned unexpected output: %s", output),
			LastCheck: time.Now(),
		}, nil
	}

	return worker.HealthStatus{
		IsHealthy: true,
		Message:   "sandbox is healthy",
		LastCheck: time.Now(),
	}, nil
}

// IsAvailable checks if the Docker backend is available.
func (b *DockerBackend) IsAvailable(ctx context.Context) bool {
	err := b.client.Ping()
	if err != nil {
		b.logger.Warn("Docker backend not available",
			zap.Error(err),
		)
		return false
	}
	return true
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

// generateContainerID creates a unique container identifier.
func generateContainerID() string {
	return fmt.Sprintf("cap-worker-%d", time.Now().UnixNano())
}

// mapContainerState maps Docker container states to worker.SandboxStatus.
func mapContainerState(state string) worker.SandboxStatus {
	switch strings.ToLower(state) {
	case "created":
		return worker.StatusCreating
	case "running", "restarting":
		return worker.StatusRunning
	case "paused":
		return worker.StatusStopped
	case "exited", "dead":
		return worker.StatusStopped
	default:
		return worker.StatusError
	}
}

// loadSeccompProfile loads a seccomp profile from a JSON file.
func loadSeccompProfile(path string) (*seccompProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read seccomp profile: %w", err)
	}

	var profile seccompProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse seccomp profile: %w", err)
	}

	return &profile, nil
}

// seccompProfileToString converts a seccomp profile to JSON string.
func seccompProfileToString(profile *seccompProfile) (string, error) {
	data, err := json.Marshal(profile)
	if err != nil {
		return "", fmt.Errorf("failed to marshal seccomp profile: %w", err)
	}
	return string(data), nil
}

// defaultRestrictiveSeccompProfile returns a built-in restrictive seccomp profile.
func defaultRestrictiveSeccompProfile() *seccompProfile {
	return &seccompProfile{
		Name: "cap-worker-default",
		Syscalls: []seccompSyscall{
			{
				Names: []string{
					"read", "write", "close", "epoll_wait", "epoll_ctl",
					"wait4", "fcntl", "futex", "getpid", "gettid",
					"madvise", "mlock", "munlock", "nanosleep", "pipe2",
					"poll", "ppoll", "prctl", "readlink", "recvfrom",
					"recvmsg", "restorer", "rt_sigaction", "rt_sigreturn",
					"select", "sendmsg", "sendto", "set_robust_list",
					"set_tid_address", "sigaltstack", "writev",
				},
				Action: "allow",
			},
			{
				Names: []string{
					"mount", "umount2", "pivot_root", "open_by_handle_at",
					"ptrace", "process_vm_writev", "lookup_dcookie",
					"perf_event_open", "fanotify_init", "fanotify_mark",
					"add_key", "request_key", "keyctl", "copy_file_range",
					"execve", "init_module", "finit_module", "delete_module",
					"syslog", "iopl", "ioperm", "reboot", "settimeofday",
					"acct", "setpcap", "sys_module", "perf_event_attr",
				},
				Action: "deny",
			},
		},
	}
}

// GetContainerInfo returns container information for debugging.
func (b *DockerBackend) GetContainerInfo(id string) (string, string, error) {
	b.mu.RLock()
	info, ok := b.containers[id]
	b.mu.RUnlock()

	if !ok {
		return "", "", &worker.ErrSandboxNotFound{ID: id}
	}

	return info.id, info.image, nil
}

// List returns all sandbox IDs managed by this backend.
func (b *DockerBackend) List() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	ids := make([]string, 0, len(b.containers))
	for id := range b.containers {
		ids = append(ids, id)
	}
	return ids
}

// Cleanup removes all containers managed by this backend.
func (b *DockerBackend) Cleanup(ctx context.Context) error {
	b.mu.Lock()
	ids := make([]string, 0, len(b.containers))
	for id := range b.containers {
		ids = append(ids, id)
	}
	b.mu.Unlock()

	var lastErr error
	for _, id := range ids {
		if err := b.Destroy(ctx, id); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// WorkspaceDir returns the workspace directory for a sandbox.
// For Docker containers, this returns the container ID.
func (b *DockerBackend) WorkspaceDir(id string) string {
	b.mu.RLock()
	_, ok := b.containers[id]
	b.mu.RUnlock()

	if !ok {
		return ""
	}

	// For Docker containers, workspace is /workspace inside the container
	return "/workspace"
}

// EnsureImagePulled pulls an image if not present.
func (b *DockerBackend) EnsureImagePulled(ctx context.Context, image string) error {
	// Check if image exists
	_, err := b.client.InspectImage(image)
	if err == nil {
		return nil
	}

	b.logger.Info("pulling Docker image",
		zap.String("image", image),
	)

	// Pull the image
	err = b.client.PullImage(docker.PullImageOptions{
		Context:    ctx,
		Repository: image,
	}, docker.AuthConfiguration{})
	if err != nil {
		return fmt.Errorf("failed to pull image %q: %w", image, err)
	}

	return nil
}

// GetConfig returns the backend configuration.
func (b *DockerBackend) GetConfig() DockerBackendConfig {
	return b.cfg
}

// Stats returns statistics about managed containers.
func (b *DockerBackend) Stats() BackendStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := BackendStats{
		TotalContainers: len(b.containers),
	}

	for _, info := range b.containers {
		if info.status == worker.StatusRunning {
			stats.RunningContainers++
		} else if info.status == worker.StatusStopping {
			stats.StoppingContainers++
		}
	}

	return stats
}

// BackendStats contains statistics about the backend.
type BackendStats struct {
	TotalContainers    int
	RunningContainers  int
	StoppingContainers int
}

// ResolveSeccompProfilePath resolves the seccomp profile path relative to the module root.
func ResolveSeccompProfilePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("seccomp profile path is empty")
	}

	// If absolute path, use as-is
	if filepath.IsAbs(path) {
		return path, nil
	}

	// Try relative to current working directory
	if _, err := os.Stat(path); err == nil {
		return filepath.Abs(path)
	}

	// Try relative to module root (assuming standard Go project structure)
	moduleRoot, err := findModuleRoot()
	if err != nil {
		return "", fmt.Errorf("failed to find module root: %w", err)
	}

	fullPath := filepath.Join(moduleRoot, path)
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath, nil
	}

	// Return original path if not found
	return path, nil
}

// findModuleRoot finds the module root directory.
func findModuleRoot() (string, error) {
	// Start from current working directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up looking for go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("go.mod not found")
}
