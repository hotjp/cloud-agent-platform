// Package workermanager provides Worker lifecycle management tests.
package workermanager

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// mockDockerClient implements a minimal docker client for testing.
type mockDockerClient struct {
	containers map[string]*mockContainer
	pinged     bool
}

type mockContainer struct {
	id      string
	name    string
	image   string
	state   string
	created int64
}

func newMockDockerClient() *mockDockerClient {
	return &mockDockerClient{
		containers: make(map[string]*mockContainer),
	}
}

// testableDockerBackend wraps DockerBackend for testing with mock client.
type testableDockerBackend struct {
	*DockerBackend
	mockClient *mockDockerClient
}

func setupTestableBackend(t *testing.T) *testableDockerBackend {
	logger := zaptest.NewLogger(t)
	cfg := DefaultDockerBackendConfig()
	cfg.SeccompProfilePath = "" // Disable seccomp loading for unit tests

	backend := &DockerBackend{
		cfg:        cfg,
		logger:     logger,
		containers: make(map[string]dockerContainerInfo),
	}

	return &testableDockerBackend{
		DockerBackend: backend,
		mockClient:    newMockDockerClient(),
	}
}

func TestDockerBackendConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     DockerBackendConfig
		wantErr bool
	}{
		{
			name:    "default config is valid",
			cfg:     DefaultDockerBackendConfig(),
			wantErr: false,
		},
		{
			name: "custom limits",
			cfg: DockerBackendConfig{
				DefaultImage:      "custom-worker:latest",
				DefaultMemoryLimit: 4 * 1024 * 1024 * 1024, // 4GB
				DefaultCPUQuota:  200000,                    // 2 CPUs
				DefaultPidsLimit: 1024,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.cfg.DefaultImage)
		})
	}
}

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
		{
			name: "with equals sign in value",
			input: map[string]string{
				"ENV": "value=with=equals",
			},
			expected: []string{"ENV=value=with=equals"},
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

func TestGenerateContainerID(t *testing.T) {
	id1 := generateContainerID()
	id2 := generateContainerID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.Contains(t, id1, "cap-worker-")
	assert.Contains(t, id2, "cap-worker-")
	assert.NotEqual(t, id1, id2, "IDs should be unique")
}

func TestMapContainerState(t *testing.T) {
	tests := []struct {
		state  string
		status worker.SandboxStatus
	}{
		{"created", worker.StatusCreating},
		{"running", worker.StatusRunning},
		{"restarting", worker.StatusRunning},
		{"paused", worker.StatusStopped},
		{"exited", worker.StatusStopped},
		{"dead", worker.StatusStopped},
		{"unknown", worker.StatusError},
		{"", worker.StatusError},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			result := mapContainerState(tt.state)
			assert.Equal(t, tt.status, result)
		})
	}
}

func TestDefaultRestrictiveSeccompProfile(t *testing.T) {
	profile := defaultRestrictiveSeccompProfile()

	assert.NotNil(t, profile)
	assert.Equal(t, "cap-worker-default", profile.Name)
	assert.NotEmpty(t, profile.Syscalls)

	// Find the deny all syscall
	var hasDenySyscalls bool
	for _, sys := range profile.Syscalls {
		if sys.Action == "deny" {
			hasDenySyscalls = true
			// Verify dangerous syscalls are in deny list
			assert.Contains(t, sys.Names, "mount", "should deny mount syscall")
			assert.Contains(t, sys.Names, "ptrace", "should deny ptrace syscall")
			assert.Contains(t, sys.Names, "execve", "should deny execve syscall")
		}
	}
	assert.True(t, hasDenySyscalls, "should have at least one deny syscall")
}

func TestLoadSeccompProfile(t *testing.T) {
	// Create a temporary seccomp profile file
	tmpDir := t.TempDir()
	profilePath := tmpDir + "/test-seccomp.json"

	validProfile := `{
		"name": "test-profile",
		"syscalls": [
			{"names": ["read", "write"], "action": "allow"},
			{"names": ["mount"], "action": "deny"}
		]
	}`

	err := os.WriteFile(profilePath, []byte(validProfile), 0644)
	require.NoError(t, err)

	profile, err := loadSeccompProfile(profilePath)
	require.NoError(t, err)
	assert.Equal(t, "test-profile", profile.Name)
	assert.Len(t, profile.Syscalls, 2)
}

func TestLoadSeccompProfile_FileNotFound(t *testing.T) {
	_, err := loadSeccompProfile("/nonexistent/path/seccomp.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read seccomp profile")
}

func TestLoadSeccompProfile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	profilePath := tmpDir + "/invalid.json"

	err := os.WriteFile(profilePath, []byte("invalid json"), 0644)
	require.NoError(t, err)

	_, err = loadSeccompProfile(profilePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse seccomp profile")
}

func TestSeccompProfileToString(t *testing.T) {
	profile := &seccompProfile{
		Name: "test",
		Syscalls: []seccompSyscall{
			{Names: []string{"read", "write"}, Action: "allow"},
		},
	}

	str, err := seccompProfileToString(profile)
	require.NoError(t, err)
	assert.Contains(t, str, `"name":"test"`)
	assert.Contains(t, str, `"read"`)
}

func TestResolveSeccompProfilePath(t *testing.T) {
	// Test with empty path
	_, err := ResolveSeccompProfilePath("")
	assert.Error(t, err)

	// Test with absolute path
	tmpDir := t.TempDir()
	absPath := tmpDir + "/test.json"
	err = os.WriteFile(absPath, []byte("{}"), 0644)
	require.NoError(t, err)

	resolved, err := ResolveSeccompProfilePath(absPath)
	require.NoError(t, err)
	assert.Equal(t, absPath, resolved)
}

func TestDockerBackend_List(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultDockerBackendConfig()

	backend := &DockerBackend{
		cfg:        cfg,
		logger:     logger,
		containers: make(map[string]dockerContainerInfo),
	}

	// Initially empty
	assert.Empty(t, backend.List())

	// Add some containers
	backend.containers["test-1"] = dockerContainerInfo{id: "id-1", image: "alpine"}
	backend.containers["test-2"] = dockerContainerInfo{id: "id-2", image: "ubuntu"}

	ids := backend.List()
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, "test-1")
	assert.Contains(t, ids, "test-2")
}

func TestDockerBackend_Stats(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultDockerBackendConfig()

	backend := &DockerBackend{
		cfg:        cfg,
		logger:     logger,
		containers: make(map[string]dockerContainerInfo),
	}

	// Initially empty
	stats := backend.Stats()
	assert.Equal(t, 0, stats.TotalContainers)
	assert.Equal(t, 0, stats.RunningContainers)
	assert.Equal(t, 0, stats.StoppingContainers)

	// Add running containers
	backend.containers["running-1"] = dockerContainerInfo{status: worker.StatusRunning}
	backend.containers["running-2"] = dockerContainerInfo{status: worker.StatusRunning}
	backend.containers["stopping-1"] = dockerContainerInfo{status: worker.StatusStopping}

	stats = backend.Stats()
	assert.Equal(t, 3, stats.TotalContainers)
	assert.Equal(t, 2, stats.RunningContainers)
	assert.Equal(t, 1, stats.StoppingContainers)
}

func TestDockerBackend_GetConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultDockerBackendConfig()
	cfg.DefaultMemoryLimit = 4 * 1024 * 1024 * 1024

	backend := &DockerBackend{
		cfg:    cfg,
		logger: logger,
	}

	got := backend.GetConfig()
	assert.Equal(t, cfg.DefaultMemoryLimit, got.DefaultMemoryLimit)
	assert.Equal(t, cfg.DefaultImage, got.DefaultImage)
}

func TestDockerBackend_WorkspaceDir(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultDockerBackendConfig()

	backend := &DockerBackend{
		cfg:        cfg,
		logger:     logger,
		containers: make(map[string]dockerContainerInfo),
	}

	// Non-existent container
	dir := backend.WorkspaceDir("nonexistent")
	assert.Empty(t, dir)

	// Existing container
	backend.containers["test-1"] = dockerContainerInfo{id: "id-1"}
	dir = backend.WorkspaceDir("test-1")
	assert.Equal(t, "/workspace", dir)
}

func TestDockerBackend_GetContainerInfo(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultDockerBackendConfig()

	backend := &DockerBackend{
		cfg:        cfg,
		logger:     logger,
		containers: make(map[string]dockerContainerInfo),
	}

	// Non-existent
	_, _, err := backend.GetContainerInfo("nonexistent")
	assert.Error(t, err)
	assert.IsType(t, &worker.ErrSandboxNotFound{}, err)

	// Existing
	backend.containers["test-1"] = dockerContainerInfo{id: "id-1", image: "alpine:latest"}
	id, image, err := backend.GetContainerInfo("test-1")
	require.NoError(t, err)
	assert.Equal(t, "id-1", id)
	assert.Equal(t, "alpine:latest", image)
}

func TestDockerBackend_BuildHostConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultDockerBackendConfig()
	cfg.DefaultMemoryLimit = 2 * 1024 * 1024 * 1024 // 2GB
	cfg.DefaultCPUQuota = 100000                       // 1 CPU

	backend := &DockerBackend{
		cfg:            cfg,
		logger:         logger,
		containers:     make(map[string]dockerContainerInfo),
		seccompProfile: defaultRestrictiveSeccompProfile(),
	}

	opts := worker.SandboxOptions{
		Image:            "test:latest",
		MemoryLimit:      4 * 1024 * 1024 * 1024, // Override to 4GB
		CPUQuota:         200000,                   // Override to 2 CPUs
		NetworkDisabled:  true,
		ReadonlyRootfs:   true,
	}

	hostCfg := backend.buildHostConfig(opts)

	// Verify resource limits
	assert.Equal(t, int64(4*1024*1024*1024), hostCfg.Memory)
	assert.Equal(t, int64(200000), hostCfg.CPUQuota)
	assert.Equal(t, int64(512), *hostCfg.PidsLimit)

	// Verify network isolation
	assert.Equal(t, "none", hostCfg.NetworkMode)

	// Verify readonly rootfs
	assert.True(t, hostCfg.ReadonlyRootfs)

	// Verify security options
	assert.Contains(t, hostCfg.SecurityOpt, "no-new-privileges:true")
	// Check that at least one security opt starts with "seccomp="
	hasSeccomp := false
	for _, opt := range hostCfg.SecurityOpt {
		if strings.HasPrefix(opt, "seccomp=") {
			hasSeccomp = true
			break
		}
	}
	assert.True(t, hasSeccomp, "expected seccomp security option")

	// Verify capability dropping
	assert.Contains(t, hostCfg.CapDrop, "ALL")

	// Verify TMPFS
	assert.Contains(t, hostCfg.Tmpfs, "/tmp")

	// Verify ulimits
	assert.NotEmpty(t, hostCfg.Ulimits)
}

func TestDockerBackend_BuildHostConfig_Defaults(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultDockerBackendConfig()
	cfg.DefaultMemoryLimit = 2 * 1024 * 1024 * 1024
	cfg.DefaultCPUQuota = 100000

	backend := &DockerBackend{
		cfg:        cfg,
		logger:     logger,
		containers: make(map[string]dockerContainerInfo),
	}

	opts := worker.SandboxOptions{
		Image: "test:latest",
	}

	hostCfg := backend.buildHostConfig(opts)

	// Verify defaults are applied
	assert.Equal(t, int64(2*1024*1024*1024), hostCfg.Memory)
	assert.Equal(t, int64(100000), hostCfg.CPUQuota)
	assert.Equal(t, "bridge", hostCfg.NetworkMode)
}

func TestDockerBackend_BuildHostConfig_WithDiskQuota(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := DefaultDockerBackendConfig()

	backend := &DockerBackend{
		cfg:        cfg,
		logger:     logger,
		containers: make(map[string]dockerContainerInfo),
	}

	opts := worker.SandboxOptions{
		Image: "test:latest",
	}

	hostCfg := backend.buildHostConfig(opts)

	// Verify storage opt
	assert.Contains(t, hostCfg.StorageOpt, "size")
}

func TestErrSandboxNotFound(t *testing.T) {
	err := &worker.ErrSandboxNotFound{ID: "test-123"}
	assert.Equal(t, "sandbox not found: test-123", err.Error())

	// Test error type
	var notFoundErr *worker.ErrSandboxNotFound
	assert.ErrorAs(t, err, &notFoundErr)
	assert.Equal(t, "test-123", notFoundErr.ID)
}

func TestErrSandboxCreationFailed(t *testing.T) {
	cause := context.DeadlineExceeded
	err := &worker.ErrSandboxCreationFailed{Image: "test:latest", Cause: cause}
	assert.Contains(t, err.Error(), "test:latest")
	assert.Contains(t, err.Error(), cause.Error())
	assert.Equal(t, cause, err.Unwrap())
}

// TestDockerBackend_Integration tests that require Docker (skip if not available)
func TestDockerBackend_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := zaptest.NewLogger(t)
	cfg := DefaultDockerBackendConfig()
	cfg.DefaultImage = "alpine:latest"
	cfg.SeccompProfilePath = "" // Don't load external profile for testing

	backend, err := NewDockerBackend(cfg, logger)
	if err != nil {
		t.Skip("Failed to create DockerBackend (Docker may not be available): ", err)
	}

	// Test IsAvailable - this is a simple connectivity check
	if !backend.IsAvailable(context.Background()) {
		t.Skip("Docker daemon not available")
	}

	// Test EnsureImagePulled
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err = backend.EnsureImagePulled(ctx, "alpine:latest")
	if err != nil {
		t.Skip("Failed to pull alpine image: ", err)
	}

	// Test Create and Destroy
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	id, err := backend.Create(ctx, worker.SandboxOptions{
		Image: "alpine:latest",
	})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	// Cleanup
	err = backend.Destroy(ctx, id)
	require.NoError(t, err)
}

func TestErrSandboxTimeout(t *testing.T) {
	err := &worker.ErrSandboxTimeout{Operation: "exec", Timeout: 30 * time.Second}
	assert.Contains(t, err.Error(), "exec")
	assert.Contains(t, err.Error(), "30s")
}

// Interface compatibility check
var _ worker.SandboxBackend = (*DockerBackend)(nil)
