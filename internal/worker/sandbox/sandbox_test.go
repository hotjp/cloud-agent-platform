package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestValidateOptions tests sandbox option validation.
func TestValidateOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    SandboxOptions
		wantErr bool
	}{
		{
			name:    "valid options with image",
			opts:    SandboxOptions{Image: "alpine:latest"},
			wantErr: false,
		},
		{
			name:    "missing image",
			opts:    SandboxOptions{},
			wantErr: true,
		},
		{
			name: "CPU quota without period",
			opts: SandboxOptions{
				Image:     "alpine:latest",
				CPUQuota:  1000,
			},
			wantErr: true,
		},
		{
			name: "valid with CPU limits",
			opts: SandboxOptions{
				Image:     "alpine:latest",
				CPUPeriod: 100000,
				CPUQuota:  50000,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOptions(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateExecOptions tests exec option validation.
func TestValidateExecOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    ExecOptions
		wantErr bool
	}{
		{
			name:    "valid with command",
			opts:    ExecOptions{Cmd: []string{"echo", "hello"}},
			wantErr: false,
		},
		{
			name:    "missing command",
			opts:    ExecOptions{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExecOptions(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestBuildEnvVars tests environment variable conversion.
func TestBuildEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected []string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: []string{},
		},
		{
			name: "single variable",
			input: map[string]string{
				"PATH": "/usr/bin",
			},
			expected: []string{"PATH=/usr/bin"},
		},
		{
			name: "multiple variables",
			input: map[string]string{
				"HOME": "/root",
				"PATH": "/usr/bin",
			},
			expected: []string{"HOME=/root", "PATH=/usr/bin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildEnvVars(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else if len(tt.expected) == 0 {
				assert.Len(t, result, 0)
			} else {
				assert.ElementsMatch(t, tt.expected, result)
			}
		})
	}
}

// TestGenerateSandboxID tests sandbox ID generation.
func TestGenerateSandboxID(t *testing.T) {
	id1 := generateSandboxID()
	id2 := generateSandboxID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.Contains(t, id1, "sandbox-")
	assert.Contains(t, id2, "sandbox-")
	// IDs should be unique (though in rare cases they might collide)
}

// TestErrSandboxNotFound tests the error type.
func TestErrSandboxNotFound(t *testing.T) {
	err := &ErrSandboxNotFound{ID: "test-123"}
	assert.Equal(t, "sandbox not found: test-123", err.Error())
	assert.True(t, err.Is(&ErrSandboxNotFound{ID: "any"}))
}

// TestErrSandboxTimeout tests the error type.
func TestErrSandboxTimeout(t *testing.T) {
	err := &ErrSandboxTimeout{Operation: "exec", Timeout: 30 * time.Second}
	assert.Contains(t, err.Error(), "exec")
	assert.Contains(t, err.Error(), "30s")
}

// TestErrSandboxCreationFailed tests the error type.
func TestErrSandboxCreationFailed(t *testing.T) {
	cause := os.ErrNotExist
	err := &ErrSandboxCreationFailed{Image: "test:latest", Cause: cause}
	assert.Contains(t, err.Error(), "test:latest")
	assert.Contains(t, err.Error(), cause.Error())
	assert.Equal(t, cause, err.Unwrap())
}

// TestErrSandboxExecutionFailed tests the error type.
func TestErrSandboxExecutionFailed(t *testing.T) {
	cause := os.ErrPermission
	err := &ErrSandboxExecutionFailed{
		SandboxID: "sandbox-1",
		Cmd:       []string{"ls", "-la"},
		Cause:     cause,
	}
	assert.Contains(t, err.Error(), "sandbox-1")
	assert.Contains(t, err.Error(), "[ls -la]")
	assert.Equal(t, cause, err.Unwrap())
}

// TestMapContainerState tests Docker state mapping.
func TestMapContainerState(t *testing.T) {
	tests := []struct {
		state   string
		status  SandboxStatus
	}{
		{"created", StatusCreating},
		{"running", StatusRunning},
		{"restarting", StatusRunning},
		{"paused", StatusStopped},
		{"exited", StatusStopped},
		{"dead", StatusStopped},
		{"unknown", StatusError},
		{"", StatusError},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			result := mapDockerState(tt.state)
			assert.Equal(t, tt.status, result)
		})
	}
}

// TestCubeSandbox_Exec tests CubeSandbox command execution.
func TestCubeSandbox_Exec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CubeSandbox test in short mode")
	}

	logger := zap.NewNop()
	workspace, err := os.MkdirTemp("", "cube-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(workspace)

	cube, err := NewCubeSandbox(logger, workspace)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a sandbox
	id, err := cube.Create(ctx, SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)
	defer cube.Destroy(ctx, id)

	// Execute a simple command
	result, err := cube.Exec(ctx, id, ExecOptions{
		Cmd:    []string{"echo", "hello world"},
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Stdout), "hello world")
	assert.NotZero(t, result.StartedAt)
	assert.NotZero(t, result.FinishedAt)
}

// TestCubeSandbox_Destroy tests CubeSandbox destruction.
func TestCubeSandbox_Destroy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CubeSandbox test in short mode")
	}

	logger := zap.NewNop()
	workspace, err := os.MkdirTemp("", "cube-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(workspace)

	cube, err := NewCubeSandbox(logger, workspace)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a sandbox
	id, err := cube.Create(ctx, SandboxOptions{
		Image: "test-image",
	})
	require.NoError(t, err)

	// Destroy it
	err = cube.Destroy(ctx, id)
	require.NoError(t, err)

	// Status should return not found
	_, err = cube.Status(ctx, id)
	assert.Error(t, err)
	assert.IsType(t, &ErrSandboxNotFound{}, err)
}

// TestCubeSandbox_List tests listing sandbox IDs.
func TestCubeSandbox_List(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CubeSandbox test in short mode")
	}

	logger := zap.NewNop()
	workspace, err := os.MkdirTemp("", "cube-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(workspace)

	cube, err := NewCubeSandbox(logger, workspace)
	require.NoError(t, err)

	ctx := context.Background()

	// Initially empty
	assert.Empty(t, cube.List())

	// Create some sandboxes
	id1, err := cube.Create(ctx, SandboxOptions{Image: "test"})
	require.NoError(t, err)
	id2, err := cube.Create(ctx, SandboxOptions{Image: "test"})
	require.NoError(t, err)

	ids := cube.List()
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, id1)
	assert.Contains(t, ids, id2)

	// Clean up
	cube.Destroy(ctx, id1)
	cube.Destroy(ctx, id2)
}

// TestCubeSandbox_GetSandboxDir tests getting sandbox directory.
func TestCubeSandbox_GetSandboxDir(t *testing.T) {
	logger := zap.NewNop()
	workspace, err := os.MkdirTemp("", "cube-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(workspace)

	cube, err := NewCubeSandbox(logger, workspace)
	require.NoError(t, err)

	dir := cube.GetSandboxDir("test-sandbox")
	expected := filepath.Join(workspace, "test-sandbox")
	assert.Equal(t, expected, dir)
}

// TestCubeSandbox_Status tests getting sandbox status.
func TestCubeSandbox_Status(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CubeSandbox test in short mode")
	}

	logger := zap.NewNop()
	workspace, err := os.MkdirTemp("", "cube-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(workspace)

	cube, err := NewCubeSandbox(logger, workspace)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a sandbox
	id, err := cube.Create(ctx, SandboxOptions{Image: "test"})
	require.NoError(t, err)

	status, err := cube.Status(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, status)

	// Destroy and check status
	err = cube.Destroy(ctx, id)
	require.NoError(t, err)

	_, err = cube.Status(ctx, id)
	assert.Error(t, err)
	assert.IsType(t, &ErrSandboxNotFound{}, err)
}

// TestCubeSandbox_ExecWithStdin tests executing with stdin input.
func TestCubeSandbox_ExecWithStdin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CubeSandbox test in short mode")
	}

	logger := zap.NewNop()
	workspace, err := os.MkdirTemp("", "cube-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(workspace)

	cube, err := NewCubeSandbox(logger, workspace)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := cube.Create(ctx, SandboxOptions{Image: "test"})
	require.NoError(t, err)
	defer cube.Destroy(ctx, id)

	// Execute cat with stdin
	result, err := cube.Exec(ctx, id, ExecOptions{
		Cmd:    []string{"cat"},
		Stdin:  []byte("test input"),
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "test input", string(result.Stdout))
}

// TestCubeSandbox_ExecTimeout tests command timeout.
func TestCubeSandbox_ExecTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CubeSandbox test in short mode")
	}

	logger := zap.NewNop()
	workspace, err := os.MkdirTemp("", "cube-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(workspace)

	cube, err := NewCubeSandbox(logger, workspace)
	require.NoError(t, err)

	ctx := context.Background()

	id, err := cube.Create(ctx, SandboxOptions{Image: "test"})
	require.NoError(t, err)
	defer cube.Destroy(ctx, id)

	// Execute a command that sleeps longer than timeout
	_, err = cube.Exec(ctx, id, ExecOptions{
		Cmd:    []string{"sleep", "10"},
		Timeout: 100 * time.Millisecond,
	})
	assert.Error(t, err)
}

// TestCubeSandbox_NotFound tests operations on non-existent sandbox.
func TestCubeSandbox_NotFound(t *testing.T) {
	logger := zap.NewNop()
	workspace, err := os.MkdirTemp("", "cube-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(workspace)

	cube, err := NewCubeSandbox(logger, workspace)
	require.NoError(t, err)

	ctx := context.Background()

	// Try to execute on non-existent sandbox
	_, err = cube.Exec(ctx, "non-existent", ExecOptions{
		Cmd: []string{"echo"},
	})
	assert.ErrorIs(t, err, &ErrSandboxNotFound{ID: "non-existent"})

	// Try to destroy non-existent sandbox
	err = cube.Destroy(ctx, "non-existent")
	assert.ErrorIs(t, err, &ErrSandboxNotFound{ID: "non-existent"})

	// Try to get status of non-existent sandbox
	_, err = cube.Status(ctx, "non-existent")
	assert.ErrorIs(t, err, &ErrSandboxNotFound{ID: "non-existent"})
}

// TestNewCubeSandbox_WithInvalidWorkspace tests creating sandbox with invalid workspace.
func TestNewCubeSandbox_WithInvalidWorkspace(t *testing.T) {
	logger := zap.NewNop()

	// Try to create with non-existent parent directory
	_, err := NewCubeSandbox(logger, "/non/existent/path")
	assert.Error(t, err)
}

// TestBuildCommand tests command building for different platforms.
func TestBuildCommand(t *testing.T) {
	cube := &CubeSandbox{}

	// Single command on Unix
	shell, args := cube.buildCommand([]string{"ls"})
	assert.Equal(t, "/bin/sh", shell)
	assert.Equal(t, []string{"-c", "ls"}, args)

	// Command with args on Unix
	shell, args = cube.buildCommand([]string{"ls", "-la"})
	assert.Equal(t, "/bin/sh", shell)
	assert.Equal(t, []string{"-c", "ls -la"}, args)

	// Complex shell command
	shell, args = cube.buildCommand([]string{"echo $HOME"})
	assert.Equal(t, "/bin/sh", shell)
	assert.Equal(t, []string{"-c", "echo $HOME"}, args)
}