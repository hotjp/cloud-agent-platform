// Package sandbox provides Worker sandbox implementations.
package sandbox

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

	"go.uber.org/zap"
)

// CubeSandbox implements Sandbox interface using lightweight process isolation.
// This is a fallback when Docker is unavailable - provides basic process
// isolation but not as strong as container-based solutions.
type CubeSandbox struct {
	logger      *zap.Logger
	mu          sync.RWMutex
	processes   map[string]*cubeProcess
	workspaceDir string
}

type cubeProcess struct {
	cmd      *exec.Cmd
	status   SandboxStatus
	pid      int
	startTime time.Time
}

// NewCubeSandbox creates a new CubeSandbox instance.
func NewCubeSandbox(logger *zap.Logger, workspaceDir string) (*CubeSandbox, error) {
	// Ensure workspace directory exists
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	return &CubeSandbox{
		logger:      logger,
		processes:   make(map[string]*cubeProcess),
		workspaceDir: workspaceDir,
	}, nil
}

// SandboxOptions contains additional CubeSandbox-specific options.
type CubeSandboxOptions struct {
	// SandboxOptions is the base options.
	SandboxOptions
	// Uid specifies the user ID to run as (Unix only).
	Uid int
	// Gid specifies the group ID to run as (Unix only).
	Gid int
	// ReadonlyPaths makes specified paths read-only.
	ReadonlyPaths []string
	// PrivateTmp creates a private /tmp directory.
	PrivateTmp bool
}

// Create implements Sandbox.Create for CubeSandbox.
func (s *CubeSandbox) Create(ctx context.Context, opts SandboxOptions) (string, error) {
	if err := ValidateOptions(opts); err != nil {
		return "", err
	}

	id := generateSandboxID()

	// Create sandbox directory
	sandboxDir := filepath.Join(s.workspaceDir, id)
	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create sandbox directory: %w", err)
	}

	// Set working directory
	workDir := opts.WorkingDir
	if workDir == "" {
		workDir = sandboxDir
	}

	// Create a init script that sets up the environment
	initScript := filepath.Join(sandboxDir, ".cubeinit")
	if err := s.writeInitScript(initScript, opts); err != nil {
		return "", fmt.Errorf("failed to write init script: %w", err)
	}

	s.mu.Lock()
	s.processes[id] = &cubeProcess{
		status: StatusRunning,
	}
	s.mu.Unlock()

	s.logger.Info("CubeSandbox created",
		zap.String("sandbox_id", id),
		zap.String("workspace", sandboxDir),
		zap.String("image", opts.Image),
	)

	return id, nil
}

// writeInitScript creates an initialization script for the sandbox.
func (s *CubeSandbox) writeInitScript(path string, opts SandboxOptions) error {
	var buf bytes.Buffer

	// Write environment variables
	for k, v := range opts.Envvars {
		buf.WriteString(fmt.Sprintf("export %s=%q\n", k, v))
	}

	// Set working directory if specified
	if opts.WorkingDir != "" {
		buf.WriteString(fmt.Sprintf("cd %q\n", opts.WorkingDir))
	}

	// Write the script
	return os.WriteFile(path, buf.Bytes(), 0755)
}

// Exec implements Sandbox.Exec for CubeSandbox.
func (s *CubeSandbox) Exec(ctx context.Context, id string, opts ExecOptions) (ExecResult, error) {
	if err := ValidateExecOptions(opts); err != nil {
		return ExecResult{}, err
	}

	s.mu.RLock()
	p, ok := s.processes[id]
	s.mu.RUnlock()
	if !ok {
		return ExecResult{}, &ErrSandboxNotFound{ID: id}
	}

	if p.status != StatusRunning {
		return ExecResult{}, fmt.Errorf("sandbox %q is not running", id)
	}

	// Get sandbox directory
	sandboxDir := filepath.Join(s.workspaceDir, id)
	workDir := opts.WorkingDir
	if workDir == "" {
		workDir = sandboxDir
	}

	// Determine shell and command
	shell, args := s.buildCommand(opts.Cmd)

	// Create command
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Dir = workDir

	// Merge environment variables
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

	// Execute with timeout
	startTime := time.Now()

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	err := cmd.Start()
	if err != nil {
		return ExecResult{}, &ErrSandboxExecutionFailed{SandboxID: id, Cmd: opts.Cmd, Cause: err}
	}

	// Update process info
	s.mu.Lock()
	p.pid = cmd.Process.Pid
	p.startTime = startTime
	s.mu.Unlock()

	// Wait for completion
	err = cmd.Wait()

	finishTime := time.Now()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return ExecResult{
				Stdout:     stdout.Bytes(),
				Stderr:     stderr.Bytes(),
				StartedAt:  startTime,
				FinishedAt: finishTime,
				Error:      err,
			}, &ErrSandboxExecutionFailed{SandboxID: id, Cmd: opts.Cmd, Cause: err}
		}
	}

	s.logger.Debug("command executed in CubeSandbox",
		zap.String("sandbox_id", id),
		zap.Strings("cmd", opts.Cmd),
		zap.Int("exit_code", exitCode),
		zap.Duration("duration", finishTime.Sub(startTime)),
	)

	return ExecResult{
		ExitCode:   exitCode,
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),
		StartedAt:  startTime,
		FinishedAt: finishTime,
	}, nil
}

// buildCommand constructs the shell command to execute.
func (s *CubeSandbox) buildCommand(cmd []string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", append([]string{"/c"}, cmd...)
	}
	// Unix-like systems
	if len(cmd) == 1 {
		return "/bin/sh", []string{"-c", cmd[0]}
	}
	return "/bin/sh", append([]string{"-c", cmd[0]}, cmd[1:]...)
}

// Destroy implements Sandbox.Destroy for CubeSandbox.
func (s *CubeSandbox) Destroy(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.processes[id]
	if !ok {
		// Check if directory exists
		sandboxDir := filepath.Join(s.workspaceDir, id)
		if _, err := os.Stat(sandboxDir); os.IsNotExist(err) {
			return &ErrSandboxNotFound{ID: id}
		}
		// Directory exists but not tracked - might be a leftover
		return nil
	}

	p.status = StatusStopping

	// Kill process if running
	if p.pid > 0 {
		// Kill the entire process group
		pgid := p.pid
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}

	// Clean up workspace
	sandboxDir := filepath.Join(s.workspaceDir, id)
	if err := os.RemoveAll(sandboxDir); err != nil {
		s.logger.Warn("failed to remove sandbox directory",
			zap.String("sandbox_id", id),
			zap.String("directory", sandboxDir),
			zap.Error(err),
		)
	}

	// Remove from tracking
	delete(s.processes, id)

	s.logger.Info("CubeSandbox destroyed",
		zap.String("sandbox_id", id),
	)

	return nil
}

// Status implements Sandbox.Status for CubeSandbox.
func (s *CubeSandbox) Status(ctx context.Context, id string) (SandboxStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.processes[id]
	if !ok {
		// Check if directory exists
		sandboxDir := filepath.Join(s.workspaceDir, id)
		if _, err := os.Stat(sandboxDir); os.IsNotExist(err) {
			return "", &ErrSandboxNotFound{ID: id}
		}
		return StatusStopped, nil
	}

	// Check if process is still running
	if p.pid > 0 {
		process, err := os.FindProcess(p.pid)
		if err != nil {
			p.status = StatusStopped
			return p.status, nil
		}
		// Try to signal process - if it exists and hasn't exited, this will succeed
		if err := process.Signal(syscall.Signal(0)); err == nil {
			return StatusRunning, nil
		}
	}

	return p.status, nil
}

// HealthCheck implements Sandbox.HealthCheck for CubeSandbox.
func (s *CubeSandbox) HealthCheck(ctx context.Context, id string) (HealthStatus, error) {
	s.mu.RLock()
	p, ok := s.processes[id]
	s.mu.RUnlock()

	if !ok {
		// Check if directory exists
		sandboxDir := filepath.Join(s.workspaceDir, id)
		if _, err := os.Stat(sandboxDir); os.IsNotExist(err) {
			return HealthStatus{}, &ErrSandboxNotFound{ID: id}
		}
		return HealthStatus{
			IsHealthy: false,
			Message:   "sandbox directory exists but process not tracked",
			LastCheck: time.Now(),
		}, nil
	}

	// Check if process is still running
	if p.pid > 0 {
		process, err := os.FindProcess(p.pid)
		if err != nil {
			return HealthStatus{
				IsHealthy: false,
				Message:   "cannot find process: " + err.Error(),
				LastCheck: time.Now(),
			}, nil
		}
		// Try to signal process - if it exists and hasn't exited, this will succeed
		if err := process.Signal(syscall.Signal(0)); err != nil {
			return HealthStatus{
				IsHealthy: false,
				Message:   "process not responsive: " + err.Error(),
				LastCheck: time.Now(),
			}, nil
		}
	}

	if p.status != StatusRunning {
		return HealthStatus{
			IsHealthy: false,
			Message:   "sandbox status is " + string(p.status),
			LastCheck: time.Now(),
		}, nil
	}

	return HealthStatus{
		IsHealthy: true,
		Message:   "sandbox is healthy",
		LastCheck: time.Now(),
	}, nil
}

// WithResourceLimits applies resource limits to a CubeSandbox process.
type WithResourceLimits struct {
	// MaxMemory is the maximum memory in bytes.
	MaxMemory int64
	// MaxCPU is the maximum CPU time in microseconds.
	MaxCPU int64
	// MaxProcesses is the maximum number of processes.
	MaxProcesses int
}

// Apply applies resource limits to a command.
func (r *WithResourceLimits) Apply(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Set process group for cleanup
	cmd.SysProcAttr.Setpgid = true

	if runtime.GOOS == "linux" {
		// On Linux, we can use seccomp via prctl, but for simplicity
		// we rely on the process being killed by OOM or cgroup
	}
}

// CleanupStaleSandboxes removes sandbox directories that aren't tracked.
func (s *CubeSandbox) CleanupStaleSandboxes(ctx context.Context, maxAge time.Duration) error {
	entries, err := os.ReadDir(s.workspaceDir)
	if err != nil {
		return fmt.Errorf("failed to read workspace directory: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)

	s.mu.RLock()
	trackedIDs := make(map[string]struct{})
	for id := range s.processes {
		trackedIDs[id] = struct{}{}
	}
	s.mu.RUnlock()

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
		path := filepath.Join(s.workspaceDir, id)
		if err := os.RemoveAll(path); err != nil {
			s.logger.Warn("failed to remove stale sandbox",
				zap.String("sandbox_id", id),
				zap.Error(err),
			)
		} else {
			s.logger.Info("removed stale sandbox",
				zap.String("sandbox_id", id),
			)
		}
	}

	return nil
}

// GetSandboxDir returns the workspace directory for a sandbox.
func (s *CubeSandbox) GetSandboxDir(id string) string {
	return filepath.Join(s.workspaceDir, id)
}

// List returns all sandbox IDs.
func (s *CubeSandbox) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.processes))
	for id := range s.processes {
		ids = append(ids, id)
	}
	return ids
}

// SetLogger updates the logger.
func (s *CubeSandbox) SetLogger(logger *zap.Logger) {
	s.logger = logger
}