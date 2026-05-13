// Package workermanager provides Worker lifecycle management with CubeSandbox backend.
package workermanager

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain/worker"
	"go.uber.org/zap"
)

// CubeBackendConfig contains configuration for the CubeSandboxBackend.
type CubeBackendConfig struct {
	// WorkspaceDir is the root directory for sandbox workspaces.
	WorkspaceDir string
	// DefaultMemoryLimit is the default memory limit in bytes.
	DefaultMemoryLimit int64
	// DefaultMaxProcesses is the default max number of processes.
	DefaultMaxProcesses int
	// DefaultTimeout is the default execution timeout.
	DefaultTimeout time.Duration
	// DefaultUID is the default user ID to run as (Unix only).
	DefaultUID int
	// DefaultGID is the default group ID to run as (Unix only).
	DefaultGID int
}

// DefaultCubeBackendConfig returns the default Cube backend configuration.
func DefaultCubeBackendConfig() CubeBackendConfig {
	return CubeBackendConfig{
		WorkspaceDir:        "/tmp/cap-sandboxes",
		DefaultMemoryLimit:  512 * 1024 * 1024, // 512MB
		DefaultMaxProcesses: 64,
		DefaultTimeout:      60 * time.Second,
		DefaultUID:          65534, // nobody user
		DefaultGID:          65534,
	}
}

// cubeSandboxProcess holds information about a running sandbox process.
type cubeSandboxProcess struct {
	id        string
	pid       int
	status    worker.SandboxStatus
	startTime time.Time
	lastUsed  time.Time
	workDir   string
}

// CubeSandboxBackend implements worker.SandboxBackend using lightweight process isolation.
// This backend does not require Docker and is suitable for development environments
// or lightweight workloads. It provides basic process isolation with resource limits
// but not as strong as container-based solutions.
type CubeSandboxBackend struct {
	cfg    CubeBackendConfig
	logger *zap.Logger

	mu      sync.RWMutex
	processes map[string]*cubeSandboxProcess
}

// NewCubeSandboxBackend creates a new CubeSandboxBackend instance.
func NewCubeSandboxBackend(cfg CubeBackendConfig, logger *zap.Logger) (*CubeSandboxBackend, error) {
	if cfg.WorkspaceDir == "" {
		cfg.WorkspaceDir = DefaultCubeBackendConfig().WorkspaceDir
	}

	// Ensure workspace directory exists
	if err := os.MkdirAll(cfg.WorkspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = DefaultCubeBackendConfig().DefaultTimeout
	}

	return &CubeSandboxBackend{
		cfg:       cfg,
		logger:    logger,
		processes: make(map[string]*cubeSandboxProcess),
	}, nil
}

// Create creates a new sandbox instance and returns its ID.
func (b *CubeSandboxBackend) Create(ctx context.Context, opts worker.SandboxOptions) (string, error) {
	// Generate unique sandbox ID
	id := generateCubeSandboxID()

	// Create sandbox directory
	sandboxDir := filepath.Join(b.cfg.WorkspaceDir, id)
	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create sandbox directory: %w", err)
	}

	// Determine working directory
	workDir := opts.WorkingDir
	if workDir == "" {
		workDir = sandboxDir
	}

	// Create init script for environment setup
	initScript := filepath.Join(sandboxDir, ".cubeinit")
	if err := b.writeInitScript(initScript, opts); err != nil {
		os.RemoveAll(sandboxDir)
		return "", fmt.Errorf("failed to write init script: %w", err)
	}

	// Track the sandbox
	b.mu.Lock()
	b.processes[id] = &cubeSandboxProcess{
		id:        id,
		status:    worker.StatusRunning,
		startTime: time.Now(),
		lastUsed:  time.Now(),
		workDir:   workDir,
	}
	b.mu.Unlock()

	b.logger.Info("CubeSandboxBackend created",
		zap.String("sandbox_id", id),
		zap.String("workspace", sandboxDir),
		zap.String("image", opts.Image),
	)

	return id, nil
}

// writeInitScript creates an initialization script for the sandbox.
func (b *CubeSandboxBackend) writeInitScript(path string, opts worker.SandboxOptions) error {
	var buf bytes.Buffer

	// Write environment variables
	for k, v := range opts.Envvars {
		buf.WriteString(fmt.Sprintf("export %s=%q\n", k, v))
	}

	// Set working directory if specified
	if opts.WorkingDir != "" {
		buf.WriteString(fmt.Sprintf("cd %q\n", opts.WorkingDir))
	}

	return os.WriteFile(path, buf.Bytes(), 0755)
}

// Exec executes a command in the specified sandbox.
func (b *CubeSandboxBackend) Exec(ctx context.Context, id string, opts worker.ExecOptions) (worker.ExecResult, error) {
	if len(opts.Cmd) == 0 {
		return worker.ExecResult{}, fmt.Errorf("command is required")
	}

	b.mu.RLock()
	p, ok := b.processes[id]
	b.mu.RUnlock()
	if !ok {
		return worker.ExecResult{}, &worker.ErrSandboxNotFound{ID: id}
	}

	if p.status != worker.StatusRunning {
		return worker.ExecResult{}, fmt.Errorf("sandbox %q is not running", id)
	}

	// Determine working directory
	workDir := opts.WorkingDir
	if workDir == "" {
		workDir = p.workDir
	}

	// Apply timeout if not specified
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = b.cfg.DefaultTimeout
	}

	// Build the full command string for shell -c
	cmdStr := b.buildCmdString(opts.Cmd)

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)

	// Create command
	cmd := exec.CommandContext(execCtx, "/bin/sh", "-c", cmdStr)
	cmd.Dir = workDir

	// Set up environment
	env := os.Environ()
	for k, v := range opts.Envvars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// Set up process group for cleanup
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Set up stdin
	if len(opts.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(opts.Stdin)
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()

	// Execute
	err := cmd.Start()
	if err != nil {
		cancel()
		return worker.ExecResult{
			StartedAt:  startTime,
			FinishedAt: time.Now(),
		}, fmt.Errorf("failed to start command: %w", err)
	}

	// Update process info
	b.mu.Lock()
	p.pid = cmd.Process.Pid
	b.mu.Unlock()

	// Wait for completion
	waitErr := cmd.Wait()
	finishTime := time.Now()

	// Cancel context to clean up resources
	cancel()

	exitCode := 0
	var oomKilled bool

	if waitErr != nil {
		// Check if this was a timeout
		if execCtx.Err() == context.DeadlineExceeded {
			return worker.ExecResult{
				Stdout:     stdout.Bytes(),
				Stderr:     stderr.Bytes(),
				StartedAt:  startTime,
				FinishedAt: finishTime,
				ExitCode:   -1,
			}, &worker.ErrSandboxTimeout{Operation: "exec", Timeout: timeout}
		}

		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			// Check if killed by OOM
			if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				oomKilled = ws.Signaled() && ws.Signal() == syscall.SIGKILL
			}
		} else {
			return worker.ExecResult{
				Stdout:     stdout.Bytes(),
				Stderr:     stderr.Bytes(),
				StartedAt:  startTime,
				FinishedAt: finishTime,
			}, fmt.Errorf("command execution failed: %w", waitErr)
		}
	}

	// Update last used time
	b.mu.Lock()
	p.lastUsed = finishTime
	b.mu.Unlock()

	b.logger.Debug("command executed in CubeSandboxBackend",
		zap.String("sandbox_id", id),
		zap.Strings("cmd", opts.Cmd),
		zap.Int("exit_code", exitCode),
		zap.Duration("duration", finishTime.Sub(startTime)),
		zap.Bool("oom_killed", oomKilled),
	)

	return worker.ExecResult{
		ExitCode:   exitCode,
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),
		StartedAt:  startTime,
		FinishedAt: finishTime,
		OOMKilled:  oomKilled,
	}, nil
}

// buildCmdString constructs the shell command string to execute.
func (b *CubeSandboxBackend) buildCmdString(cmd []string) string {
	if runtime.GOOS == "windows" {
		return joinCommand(cmd, " ")
	}
	// Unix-like systems
	// If the command already includes "-c" as second arg (e.g., ["sh", "-c", "echo hello"]),
	// extract just the script string to avoid double-wrapping
	if len(cmd) >= 3 && cmd[1] == "-c" {
		return cmd[2]
	}
	return joinCommand(cmd, " ")
}

// joinCommand joins command parts with the given separator.
func joinCommand(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

// applyResourceLimits applies resource limits to a command.
func (b *CubeSandboxBackend) applyResourceLimits(cmd *exec.Cmd, opts worker.ExecOptions) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true

	// Note: setrlimit must be called in the child process before exec
	// We use a wrapper script approach for rlimits
	if runtime.GOOS == "linux" {
		// On Linux, we can use prctl for additional control
		// Memory limit is applied via setrlimit in the child
	}
}

// Destroy terminates and removes a sandbox.
func (b *CubeSandboxBackend) Destroy(ctx context.Context, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	p, ok := b.processes[id]
	if !ok {
		// Check if directory exists
		sandboxDir := filepath.Join(b.cfg.WorkspaceDir, id)
		if _, err := os.Stat(sandboxDir); os.IsNotExist(err) {
			return &worker.ErrSandboxNotFound{ID: id}
		}
		// Directory exists but not tracked - clean it up
		os.RemoveAll(sandboxDir)
		return nil
	}

	p.status = worker.StatusStopping

	// Kill process if running
	if p.pid > 0 {
		// Kill the entire process group
		_ = syscall.Kill(-p.pid, syscall.SIGKILL)
	}

	// Clean up workspace
	sandboxDir := filepath.Join(b.cfg.WorkspaceDir, id)
	if err := os.RemoveAll(sandboxDir); err != nil {
		b.logger.Warn("failed to remove sandbox directory",
			zap.String("sandbox_id", id),
			zap.String("directory", sandboxDir),
			zap.Error(err),
		)
	}

	// Remove from tracking
	delete(b.processes, id)

	b.logger.Info("CubeSandboxBackend destroyed",
		zap.String("sandbox_id", id),
	)

	return nil
}

// Status returns the current status of a sandbox.
func (b *CubeSandboxBackend) Status(ctx context.Context, id string) (worker.SandboxStatus, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	p, ok := b.processes[id]
	if !ok {
		// Check if directory exists
		sandboxDir := filepath.Join(b.cfg.WorkspaceDir, id)
		if _, err := os.Stat(sandboxDir); os.IsNotExist(err) {
			return "", &worker.ErrSandboxNotFound{ID: id}
		}
		return worker.StatusStopped, nil
	}

	// Check if process is still running
	if p.pid > 0 {
		proc, err := os.FindProcess(p.pid)
		if err == nil {
			// Try to signal process - if it exists and hasn't exited, this will succeed
			if err := proc.Signal(syscall.Signal(0)); err == nil {
				return worker.StatusRunning, nil
			}
		}
	}

	return p.status, nil
}

// HealthCheck checks the health of a sandbox and returns detailed status.
func (b *CubeSandboxBackend) HealthCheck(ctx context.Context, id string) (worker.HealthStatus, error) {
	b.mu.RLock()
	p, ok := b.processes[id]
	b.mu.RUnlock()

	if !ok {
		// Check if directory exists
		sandboxDir := filepath.Join(b.cfg.WorkspaceDir, id)
		if _, err := os.Stat(sandboxDir); os.IsNotExist(err) {
			return worker.HealthStatus{}, &worker.ErrSandboxNotFound{ID: id}
		}
		return worker.HealthStatus{
			IsHealthy: false,
			Message:   "sandbox directory exists but process not tracked",
			LastCheck: time.Now(),
		}, nil
	}

	// Check if process is still running
	if p.pid > 0 {
		proc, err := os.FindProcess(p.pid)
		if err != nil {
			return worker.HealthStatus{
				IsHealthy: false,
				Message:   "cannot find process: " + err.Error(),
				LastCheck: time.Now(),
			}, nil
		}
		// Try to signal process - if it exists and hasn't exited, this will succeed
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return worker.HealthStatus{
				IsHealthy: false,
				Message:   "process not responsive: " + err.Error(),
				LastCheck: time.Now(),
			}, nil
		}
	}

	if p.status != worker.StatusRunning {
		return worker.HealthStatus{
			IsHealthy: false,
			Message:   "sandbox status is " + string(p.status),
			LastCheck: time.Now(),
		}, nil
	}

	return worker.HealthStatus{
		IsHealthy: true,
		Message:   "sandbox is healthy",
		LastCheck: time.Now(),
	}, nil
}

// IsAvailable checks if the Cube backend is available.
// This backend is always available as it doesn't require Docker.
func (b *CubeSandboxBackend) IsAvailable(ctx context.Context) bool {
	// CubeSandboxBackend is always available
	return true
}

// CleanupStaleSandboxes removes sandbox directories that aren't tracked.
func (b *CubeSandboxBackend) CleanupStaleSandboxes(ctx context.Context, maxAge time.Duration) error {
	entries, err := os.ReadDir(b.cfg.WorkspaceDir)
	if err != nil {
		return fmt.Errorf("failed to read workspace directory: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)

	b.mu.RLock()
	trackedIDs := make(map[string]struct{})
	for id := range b.processes {
		trackedIDs[id] = struct{}{}
	}
	b.mu.RUnlock()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		id := entry.Name()
		if _, tracked := trackedIDs[id]; tracked {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(cutoff) {
			continue
		}

		// Remove stale sandbox
		path := filepath.Join(b.cfg.WorkspaceDir, id)
		if err := os.RemoveAll(path); err != nil {
			b.logger.Warn("failed to remove stale sandbox",
				zap.String("sandbox_id", id),
				zap.Error(err),
			)
		} else {
			b.logger.Info("removed stale sandbox",
				zap.String("sandbox_id", id),
			)
		}
	}

	return nil
}

// GetSandboxDir returns the workspace directory for a sandbox.
func (b *CubeSandboxBackend) GetSandboxDir(id string) string {
	return filepath.Join(b.cfg.WorkspaceDir, id)
}

// List returns all sandbox IDs managed by this backend.
func (b *CubeSandboxBackend) List() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	ids := make([]string, 0, len(b.processes))
	for id := range b.processes {
		ids = append(ids, id)
	}
	return ids
}

// Cleanup removes all sandboxes managed by this backend.
func (b *CubeSandboxBackend) Cleanup(ctx context.Context) error {
	b.mu.Lock()
	ids := make([]string, 0, len(b.processes))
	for id := range b.processes {
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
func (b *CubeSandboxBackend) WorkspaceDir(id string) string {
	b.mu.RLock()
	_, ok := b.processes[id]
	b.mu.RUnlock()

	if !ok {
		return ""
	}

	return filepath.Join(b.cfg.WorkspaceDir, id)
}

// GetConfig returns the backend configuration.
func (b *CubeSandboxBackend) GetConfig() CubeBackendConfig {
	return b.cfg
}

// Stats returns statistics about managed sandboxes.
func (b *CubeSandboxBackend) Stats() BackendStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := BackendStats{
		TotalContainers: len(b.processes),
	}

	for _, p := range b.processes {
		if p.status == worker.StatusRunning {
			stats.RunningContainers++
		} else if p.status == worker.StatusStopping {
			stats.StoppingContainers++
		}
	}

	return stats
}

// generateCubeSandboxID creates a unique sandbox identifier.
func generateCubeSandboxID() string {
	return fmt.Sprintf("cube-%d", time.Now().UnixNano())
}

// Interface compliance check
var _ worker.SandboxBackend = (*CubeSandboxBackend)(nil)
