// Package gitcontainer manages per-project Git containers.
// Each project (repo URL or empty project) gets its own container running cap-git.
// Workers share the container's volume to read/write files.
// Git operations (commit, push, diff) are delegated to the container's HTTP API.
package gitcontainer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ProjectID generates a stable ID from a repo URL (or "empty" for new projects).
func ProjectID(repoURL string) string {
	if repoURL == "" {
		return "empty"
	}
	h := sha256.Sum256([]byte(repoURL))
	return fmt.Sprintf("%x", h[:12])
}

// GitContainer represents a running cap-git container.
type GitContainer struct {
	ID        string // Docker container ID
	ProjectID string // Stable project identifier
	RepoURL   string // Original repo URL (may be empty)
	APIPort   int    // Host port mapped to container's 9090
	VolumeDir string // Host path for the shared volume
}

// Manager manages the lifecycle of Git containers.
type Manager struct {
	mu         sync.Mutex
	containers map[string]*GitContainer // projectID → container
	gitImage   string
	network    string // Docker network name (empty = default)
}

// Config for the Git container manager.
type Config struct {
	GitImage string // cap-git image name (default: cap-git:latest)
	Network  string // Docker network (empty = default)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		GitImage: "cap-git:latest",
		Network:  "",
	}
}

// NewManager creates a new Git container manager.
func NewManager(cfg Config) *Manager {
	return &Manager{
		containers: make(map[string]*GitContainer),
		gitImage:   cfg.GitImage,
		network:    cfg.Network,
	}
}

// Ensure returns (or creates) a Git container for the given project.
// If a container already exists for this project, it's returned directly.
func (m *Manager) Ensure(ctx context.Context, repoURL, branch string) (*GitContainer, error) {
	pid := ProjectID(repoURL)

	// Retry loop for concurrent access safety
	for i := 0; i < 60; i++ {
		m.mu.Lock()
		gc, exists := m.containers[pid]
		if exists && gc != nil {
			m.mu.Unlock()
			if m.isRunning(gc.ID) {
				return gc, nil
			}
			// Dead, clean up
			m.mu.Lock()
			delete(m.containers, pid)
			m.mu.Unlock()
			continue
		}
		if exists && gc == nil {
			// Someone else is creating, wait
			m.mu.Unlock()
			time.Sleep(300 * time.Millisecond)
			continue
		}

		// Reserve slot with nil placeholder
		m.containers[pid] = nil
		m.mu.Unlock()

		newGC, err := m.create(ctx, pid, repoURL, branch)
		if err != nil {
			m.mu.Lock()
			delete(m.containers, pid)
			m.mu.Unlock()
			return nil, err
		}
		return newGC, nil
	}
	return nil, fmt.Errorf("gitcontainer: timeout waiting for container (project %s)", pid)
}

func (m *Manager) create(ctx context.Context, pid, repoURL, branch string) (*GitContainer, error) {
	// Create a host directory for the shared volume
	volumeDir := fmt.Sprintf("/tmp/cap-git-volumes/%s", pid)
	if err := os.MkdirAll(volumeDir, 0755); err != nil {
		return nil, fmt.Errorf("gitcontainer: mkdir %s: %w", volumeDir, err)
	}

	// Find a free host port for the API
	apiPort, err := getFreePort()
	if err != nil {
		return nil, fmt.Errorf("gitcontainer: no free port: %w", err)
	}

	// Build docker run args
	args := []string{
		"run", "-d",
		"--name", fmt.Sprintf("cap-git-%s", pid),
		"-p", fmt.Sprintf("%d:9090", apiPort),
		"-v", fmt.Sprintf("%s:/workspace", volumeDir),
		"-e", "GIT_API_PORT=9090",
	}

	// Proxy env (for git clone)
	for _, envVar := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "no_proxy", "NO_PROXY"} {
		if val := os.Getenv(envVar); val != "" {
			args = append(args, "-e", fmt.Sprintf("%s=%s", envVar, val))
		}
	}

	// Network
	if m.network != "" {
		args = append(args, "--network", m.network)
	}

	args = append(args, m.gitImage)

	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gitcontainer: docker run: %w\n%s", err, string(out))
	}

	containerID := strings.TrimSpace(string(out))

	gc := &GitContainer{
		ID:        containerID,
		ProjectID: pid,
		RepoURL:   repoURL,
		APIPort:   apiPort,
		VolumeDir: volumeDir,
	}

	// Wait for API to be ready
	if err := m.waitForAPI(gc, 15*time.Second); err != nil {
		// Cleanup
		exec.Command("docker", "rm", "-f", containerID).Run()
		return nil, fmt.Errorf("gitcontainer: API not ready: %w", err)
	}

	// Initialize: clone repo or create empty git
	if repoURL != "" {
		initBody := map[string]string{"repo_url": repoURL, "branch": branch}
		if _, err := apiCall(gc.APIPort, "POST", "/init", initBody); err != nil {
			// Clone failure is non-fatal — empty repo is usable
			fmt.Printf("[gitcontainer] init warning: %v\n", err)
		}
	} else {
		apiCall(gc.APIPort, "POST", "/init", map[string]string{})
	}

	m.mu.Lock()
	m.containers[pid] = gc
	m.mu.Unlock()

	fmt.Printf("[gitcontainer] created container %s for project %s (port %d)\n",
		containerID[:12], pid, apiPort)

	return gc, nil
}

// Commit tells the Git container to commit all changes.
func (gc *GitContainer) Commit(message, branch string) (string, error) {
	body := map[string]string{"message": message, "branch": branch}
	resp, err := apiCall(gc.APIPort, "POST", "/commit", body)
	if err != nil {
		return "", err
	}
	return resp["sha"].(string), nil
}

// Diff returns the latest diff from the Git container.
func (gc *GitContainer) Diff() (string, error) {
	resp, err := apiCall(gc.APIPort, "GET", "/diff", nil)
	if err != nil {
		return "", err
	}
	return resp["diff"].(string), nil
}

// ListFiles returns all tracked files in the Git container.
func (gc *GitContainer) ListFiles() ([]string, error) {
	resp, err := apiCall(gc.APIPort, "GET", "/files", nil)
	if err != nil {
		return nil, err
	}
	raw, ok := resp["files"].([]interface{})
	if !ok {
		return nil, nil
	}
	files := make([]string, len(raw))
	for i, f := range raw {
		files[i] = f.(string)
	}
	return files, nil
}

// Push triggers a git push from the container.
func (gc *GitContainer) Push(remote, branch, token string) error {
	body := map[string]string{"remote": remote, "branch": branch, "token": token}
	_, err := apiCall(gc.APIPort, "POST", "/push", body)
	return err
}

// Cleanup destroys a Git container.
func (m *Manager) Cleanup(pid string) error {
	m.mu.Lock()
	gc, ok := m.containers[pid]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.containers, pid)
	m.mu.Unlock()

	exec.Command("docker", "rm", "-f", gc.ID).Run()
	os.RemoveAll(gc.VolumeDir)
	return nil
}

// StopAll destroys all managed containers.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for pid, gc := range m.containers {
		exec.Command("docker", "rm", "-f", gc.ID).Run()
		os.RemoveAll(gc.VolumeDir)
		delete(m.containers, pid)
	}
}

func (m *Manager) isRunning(id string) bool {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", id).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func (m *Manager) waitForAPI(gc *GitContainer, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := apiCall(gc.APIPort, "GET", "/health", nil)
		if err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for API on port %d", gc.APIPort)
}

// apiCall makes an HTTP request to a cap-git container API.
func apiCall(port int, method, path string, body interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api call %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %s", string(data))
	}

	return result, nil
}

// getFreePort finds an available TCP port.
func getFreePort() (int, error) {
	cmd := exec.Command("python3", "-c", "import socket; s=socket.socket(); s.bind(('',0)); print(s.getsockname()[1]); s.close()")
	out, err := cmd.Output()
	if err != nil {
		// Fallback: use a range
		return 19090, nil
	}
	var port int
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &port)
	return port, nil
}
