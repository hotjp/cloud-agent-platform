package workermanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestCubeBackendConfig_Default(t *testing.T) {
	cfg := DefaultCubeBackendConfig()

	assert.NotEmpty(t, cfg.WorkspaceDir)
	assert.Equal(t, int64(512*1024*1024), cfg.DefaultMemoryLimit)
	assert.Equal(t, 64, cfg.DefaultMaxProcesses)
	assert.Equal(t, 60*time.Second, cfg.DefaultTimeout)
}

func TestNewCubeSandboxBackend(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)
	assert.NotNil(t, backend)
	assert.Equal(t, cfg, backend.GetConfig())
}

func TestNewCubeSandboxBackend_CreatesWorkspace(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := CubeBackendConfig{
		WorkspaceDir: filepath.Join(tmpDir, "subdir", "sandboxes"),
	}

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)
	assert.NotNil(t, backend)

	// Workspace should have been created
	info, err := os.Stat(cfg.WorkspaceDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestNewCubeSandboxBackend_InvalidWorkspace(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Non-existent parent directory
	cfg := CubeBackendConfig{
		WorkspaceDir: "/non/existent/path/workspace",
	}

	_, err := NewCubeSandboxBackend(cfg, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create workspace directory")
}

func TestCubeSandboxBackend_Create(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	// Verify sandbox directory was created
	sandboxDir := filepath.Join(tmpDir, id)
	_, err = os.Stat(sandboxDir)
	require.NoError(t, err)

	// Verify sandbox is tracked
	ids := backend.List()
	assert.Contains(t, ids, id)

	// Cleanup
	err = backend.Destroy(ctx, id)
	require.NoError(t, err)
}

func TestCubeSandboxBackend_Create_WithWorkingDir(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()
	workDir := filepath.Join(tmpDir, "custom-workdir")
	err = os.MkdirAll(workDir, 0755)
	require.NoError(t, err)

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image:      "test-image",
		WorkingDir: workDir,
	})
	require.NoError(t, err)

	// Verify sandbox directory was created
	sandboxDir := filepath.Join(tmpDir, id)
	_, err = os.Stat(sandboxDir)
	require.NoError(t, err)

	// Cleanup
	err = backend.Destroy(ctx, id)
	require.NoError(t, err)
}

func TestCubeSandboxBackend_Exec(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)
	defer backend.Destroy(ctx, id)

	result, err := backend.Exec(ctx, id, worker.ExecOptions{
		Cmd:    []string{"echo", "hello world"},
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Stdout), "hello world")
	assert.NotZero(t, result.StartedAt)
	assert.NotZero(t, result.FinishedAt)
	assert.False(t, result.OOMKilled)
}

func TestCubeSandboxBackend_Exec_WithEnvvars(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)
	defer backend.Destroy(ctx, id)

	result, err := backend.Exec(ctx, id, worker.ExecOptions{
		Cmd:    []string{"sh", "-c", "echo $TEST_VAR"},
		Envvars: map[string]string{"TEST_VAR": "test-value"},
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Stdout), "test-value")
}

func TestCubeSandboxBackend_Exec_WithStdin(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)
	defer backend.Destroy(ctx, id)

	result, err := backend.Exec(ctx, id, worker.ExecOptions{
		Cmd:    []string{"cat"},
		Stdin:  []byte("test input"),
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "test input", string(result.Stdout))
}

func TestCubeSandboxBackend_Exec_Timeout(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)
	defer backend.Destroy(ctx, id)

	// Execute a command that sleeps longer than timeout
	result, err := backend.Exec(ctx, id, worker.ExecOptions{
		Cmd:    []string{"sleep", "10"},
		Timeout: 100 * time.Millisecond,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Equal(t, -1, result.ExitCode)
}

func TestCubeSandboxBackend_Exec_NotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = backend.Exec(ctx, "non-existent", worker.ExecOptions{
		Cmd: []string{"echo", "hello"},
	})
	assert.IsType(t, &worker.ErrSandboxNotFound{}, err)
}

func TestCubeSandboxBackend_Exec_ExitCode(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)
	defer backend.Destroy(ctx, id)

	result, err := backend.Exec(ctx, id, worker.ExecOptions{
		Cmd:    []string{"exit", "42"},
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, 42, result.ExitCode)
}

func TestCubeSandboxBackend_Destroy(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)

	// Verify sandbox is tracked
	ids := backend.List()
	assert.Contains(t, ids, id)

	// Destroy
	err = backend.Destroy(ctx, id)
	require.NoError(t, err)

	// Verify sandbox is no longer tracked
	ids = backend.List()
	assert.NotContains(t, ids, id)

	// Verify directory is removed
	sandboxDir := filepath.Join(tmpDir, id)
	_, err = os.Stat(sandboxDir)
	assert.True(t, os.IsNotExist(err))
}

func TestCubeSandboxBackend_Destroy_NotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	err = backend.Destroy(ctx, "non-existent")
	assert.IsType(t, &worker.ErrSandboxNotFound{}, err)
}

func TestCubeSandboxBackend_Status(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)
	defer backend.Destroy(ctx, id)

	status, err := backend.Status(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, worker.StatusRunning, status)
}

func TestCubeSandboxBackend_Status_NotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = backend.Status(ctx, "non-existent")
	assert.IsType(t, &worker.ErrSandboxNotFound{}, err)
}

func TestCubeSandboxBackend_HealthCheck(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)
	defer backend.Destroy(ctx, id)

	health, err := backend.HealthCheck(ctx, id)
	require.NoError(t, err)
	assert.True(t, health.IsHealthy)
	assert.Equal(t, "sandbox is healthy", health.Message)
	assert.NotZero(t, health.LastCheck)
}

func TestCubeSandboxBackend_HealthCheck_NotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = backend.HealthCheck(ctx, "non-existent")
	assert.IsType(t, &worker.ErrSandboxNotFound{}, err)
}

func TestCubeSandboxBackend_IsAvailable(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// CubeSandboxBackend is always available
	assert.True(t, backend.IsAvailable(ctx))
}

func TestCubeSandboxBackend_List(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Initially empty
	assert.Empty(t, backend.List())

	// Create some sandboxes
	id1, err := backend.Create(ctx, worker.SandboxOptions{Image: "test"})
	require.NoError(t, err)
	id2, err := backend.Create(ctx, worker.SandboxOptions{Image: "test"})
	require.NoError(t, err)

	ids := backend.List()
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, id1)
	assert.Contains(t, ids, id2)

	// Clean up
	backend.Destroy(ctx, id1)
	backend.Destroy(ctx, id2)
}

func TestCubeSandboxBackend_Stats(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	// Initially empty
	stats := backend.Stats()
	assert.Equal(t, 0, stats.TotalContainers)
	assert.Equal(t, 0, stats.RunningContainers)
	assert.Equal(t, 0, stats.StoppingContainers)

	ctx := context.Background()

	// Create some sandboxes
	id1, err := backend.Create(ctx, worker.SandboxOptions{Image: "test"})
	require.NoError(t, err)
	id2, err := backend.Create(ctx, worker.SandboxOptions{Image: "test"})
	require.NoError(t, err)

	stats = backend.Stats()
	assert.Equal(t, 2, stats.TotalContainers)
	assert.Equal(t, 2, stats.RunningContainers)

	// Destroy one
	backend.Destroy(ctx, id1)

	stats = backend.Stats()
	assert.Equal(t, 1, stats.TotalContainers)
	assert.Equal(t, 1, stats.RunningContainers)

	// Clean up
	backend.Destroy(ctx, id2)
}

func TestCubeSandboxBackend_Cleanup(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create some sandboxes
	id1, err := backend.Create(ctx, worker.SandboxOptions{Image: "test"})
	require.NoError(t, err)
	id2, err := backend.Create(ctx, worker.SandboxOptions{Image: "test"})
	require.NoError(t, err)

	ids := backend.List()
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, id1)
	assert.Contains(t, ids, id2)

	// Cleanup all
	err = backend.Cleanup(ctx)
	require.NoError(t, err)

	assert.Empty(t, backend.List())
}

func TestCubeSandboxBackend_WorkspaceDir(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Non-existent
	dir := backend.WorkspaceDir("non-existent")
	assert.Empty(t, dir)

	// Create sandbox
	id, err := backend.Create(ctx, worker.SandboxOptions{Image: "test"})
	require.NoError(t, err)

	dir = backend.WorkspaceDir(id)
	assert.Equal(t, filepath.Join(tmpDir, id), dir)

	backend.Destroy(ctx, id)
}

func TestCubeSandboxBackend_CleanupStaleSandboxes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a tracked sandbox
	id, err := backend.Create(ctx, worker.SandboxOptions{Image: "test"})
	require.NoError(t, err)

	// Create a stale directory manually (not tracked)
	staleDir := filepath.Join(tmpDir, "stale-sandbox")
	err = os.MkdirAll(staleDir, 0755)
	require.NoError(t, err)

	// Set modification time to 2 hours ago
	oldTime := time.Now().Add(-2 * time.Hour)
	os.Chtimes(staleDir, oldTime, oldTime)

	// Cleanup with maxAge of 1 hour
	err = backend.CleanupStaleSandboxes(ctx, 1*time.Hour)
	require.NoError(t, err)

	// Tracked sandbox should still exist
	ids := backend.List()
	assert.Contains(t, ids, id)

	// Stale directory should be removed
	_, err = os.Stat(staleDir)
	assert.True(t, os.IsNotExist(err))

	backend.Destroy(ctx, id)
}

func TestGenerateCubeSandboxID(t *testing.T) {
	id1 := generateCubeSandboxID()
	id2 := generateCubeSandboxID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.Contains(t, id1, "cube-")
	assert.Contains(t, id2, "cube-")
	assert.NotEqual(t, id1, id2, "IDs should be unique")
}

func TestBuildCmdString(t *testing.T) {
	backend := &CubeSandboxBackend{}

	// Single command
	cmdStr := backend.buildCmdString([]string{"ls"})
	assert.Equal(t, "ls", cmdStr)

	// Command with args
	cmdStr = backend.buildCmdString([]string{"ls", "-la"})
	assert.Equal(t, "ls -la", cmdStr)

	// Complex shell command
	cmdStr = backend.buildCmdString([]string{"echo $HOME"})
	assert.Equal(t, "echo $HOME", cmdStr)
}

func TestCubeSandboxBackend_Exec_NoCommand(t *testing.T) {
	logger := zaptest.NewLogger(t)
	tmpDir := t.TempDir()

	cfg := DefaultCubeBackendConfig()
	cfg.WorkspaceDir = tmpDir

	backend, err := NewCubeSandboxBackend(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := backend.Create(ctx, worker.SandboxOptions{Image: "test"})
	require.NoError(t, err)
	defer backend.Destroy(ctx, id)

	_, err = backend.Exec(ctx, id, worker.ExecOptions{
		Cmd: []string{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command is required")
}

// Interface compliance check
var _ worker.SandboxBackend = (*CubeSandboxBackend)(nil)
