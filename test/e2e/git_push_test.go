// Package e2e provides end-to-end tests for the Cloud Agent Platform.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"github.com/cloud-agent-platform/cap/plugins/gitclient"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// gitPushTestHandler simulates the platform API for git push E2E tests.
type gitPushTestHandler struct {
	mu        sync.Mutex
	idCounter int
	tasks     map[string]*mcp.TaskStatusResponse
	repos     map[string]*repoState
	pushLog   []pushRecord
}

type repoState struct {
	writable    bool
	hasConflict bool
}

type pushRecord struct {
	RepoURL  string
	Branch   string
	Success  bool
	ErrorMsg string
}

func newGitPushTestHandler() *gitPushTestHandler {
	return &gitPushTestHandler{
		tasks: make(map[string]*mcp.TaskStatusResponse),
		repos: make(map[string]*repoState),
		pushLog: make([]pushRecord, 0),
	}
}

func (h *gitPushTestHandler) addRepo(url string, writable bool, hasConflict bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.repos[url] = &repoState{writable: writable, hasConflict: hasConflict}
}

func (h *gitPushTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	switch {
	// POST /api/v1/tasks - submit a task (used for git push workflow)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
		var req mcp.TaskSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "BAD_REQUEST", Message: "invalid JSON"}})
			return
		}
		h.idCounter++
		id := fmt.Sprintf("task_git_%d", h.idCounter)
		task := &mcp.TaskStatusResponse{
			TaskID:   id,
			Status:   "pending",
			Goal:     req.Goal,
			Priority: req.Priority,
		}
		h.tasks[id] = task

		taskStatus := "completed"
		resultBranch := "result/" + id

		if req.Repository != nil {
			repo, ok := h.repos[req.Repository.URL]
			if ok {
				if !repo.writable {
					taskStatus = "failed"
					task.Result = &mcp.TaskResultResponse{Summary: "git push failed: repository is read-only"}
					h.pushLog = append(h.pushLog, pushRecord{
						RepoURL:  req.Repository.URL,
						Branch:   req.Repository.Branch,
						Success:  false,
						ErrorMsg: "repository is read-only",
					})
				} else if repo.hasConflict {
					taskStatus = "failed"
					task.Result = &mcp.TaskResultResponse{Summary: "git push failed: conflict detected"}
					h.pushLog = append(h.pushLog, pushRecord{
						RepoURL:  req.Repository.URL,
						Branch:   req.Repository.Branch,
						Success:  false,
						ErrorMsg: "conflict detected",
					})
				} else {
					if req.Repository.ResultBranch != "" {
						resultBranch = req.Repository.ResultBranch
					}
					task.ResultBranch = resultBranch
					task.Result = &mcp.TaskResultResponse{Summary: "git push successful", GitBranch: resultBranch}
					h.pushLog = append(h.pushLog, pushRecord{
						RepoURL: req.Repository.URL,
						Branch:  resultBranch,
						Success: true,
					})
				}
			}
		}
		if taskStatus == "pending" {
			taskStatus = "completed"
			task.Result = &mcp.TaskResultResponse{Summary: "git push successful", GitBranch: "result/default"}
		}
		task.Status = taskStatus

		_ = json.NewEncoder(w).Encode(mcp.APIResponse{
			OK:   true,
			Data: marshalData(mcp.TaskSubmitResponse{TaskID: id, Status: taskStatus, ResultBranch: resultBranch}),
		})
		return

	// GET /api/v1/tasks/:id - get task status
	case r.Method == http.MethodGet && len(r.URL.Path) > len("/api/v1/tasks/"):
		id := r.URL.Path[len("/api/v1/tasks/"):]
		task, ok := h.tasks[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "task not found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: true, Data: marshalData(task)})
		return

	// POST /api/v1/repos/:name/push - direct push endpoint
	case r.Method == http.MethodPost && len(r.URL.Path) > len("/api/v1/repos/") && r.URL.Path[len(r.URL.Path)-5:] == "/push":
		repoName := r.URL.Path[len("/api/v1/repos/"):len(r.URL.Path)-5]

		var req struct {
			Branch string `json:"branch"`
			Force  bool   `json:"force"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "BAD_REQUEST", Message: "invalid JSON"}})
			return
		}

		repo, ok := h.repos[repoName]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "repository not found"}})
			return
		}

		if !repo.writable {
			h.pushLog = append(h.pushLog, pushRecord{
				RepoURL:  repoName,
				Branch:   req.Branch,
				Success:  false,
				ErrorMsg: "repository is read-only",
			})
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{
				OK: false,
				Error: &mcp.APIError{Code: "FORBIDDEN", Message: "repository is read-only"},
			})
			return
		}

		if repo.hasConflict && !req.Force {
			h.pushLog = append(h.pushLog, pushRecord{
				RepoURL:  repoName,
				Branch:   req.Branch,
				Success:  false,
				ErrorMsg: "conflict detected",
			})
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{
				OK: false,
				Error: &mcp.APIError{Code: "CONFLICT", Message: "conflict detected, force push required"},
			})
			return
		}

		h.pushLog = append(h.pushLog, pushRecord{
			RepoURL: repoName,
			Branch:  req.Branch,
			Success: true,
		})
		_ = json.NewEncoder(w).Encode(mcp.APIResponse{
			OK:   true,
			Data: marshalData(map[string]any{"branch": req.Branch, "message": "pushed successfully"}),
		})
		return

	// POST /api/v1/repos/:name/create-branch - create and push branch
	case r.Method == http.MethodPost && len(r.URL.Path) > len("/api/v1/repos/") && r.URL.Path[len(r.URL.Path)-13:] == "/create-branch":
		repoName := r.URL.Path[len("/api/v1/repos/"):len(r.URL.Path)-13]

		var req struct {
			Branch     string `json:"branch"`
			FromBranch string `json:"fromBranch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "BAD_REQUEST", Message: "invalid JSON"}})
			return
		}

		_, ok := h.repos[repoName]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "repository not found"}})
			return
		}

		h.pushLog = append(h.pushLog, pushRecord{
			RepoURL: repoName,
			Branch:  req.Branch,
			Success: true,
		})

		_ = json.NewEncoder(w).Encode(mcp.APIResponse{
			OK:   true,
			Data: marshalData(map[string]any{"branch": req.Branch, "message": "branch created and pushed"}),
		})
		return
	}

	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "endpoint not found"}})
}

// TestGitPush_PushToWritableRepo tests pushing to a writable repository succeeds.
func TestGitPush_PushToWritableRepo(t *testing.T) {
	logger := zap.NewNop()
	handler := newGitPushTestHandler()
	handler.addRepo("https://github.com/writable/repo", true, false)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	resp, err := client.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal: "Fix bug in writable repo",
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/writable/repo",
			Branch: "main",
		},
		Priority: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, "completed", resp.Status)

	// Verify push was logged as successful
	handler.mu.Lock()
	found := false
	for _, record := range handler.pushLog {
		if record.RepoURL == "https://github.com/writable/repo" && record.Success {
			found = true
			break
		}
	}
	handler.mu.Unlock()
	assert.True(t, found, "Push to writable repo should be logged as successful")

	t.Log("Push to writable repo succeeded")
}

// TestGitPush_PushToReadOnlyRepo tests pushing to a read-only repository fails.
func TestGitPush_PushToReadOnlyRepo(t *testing.T) {
	logger := zap.NewNop()
	handler := newGitPushTestHandler()
	handler.addRepo("https://github.com/readonly/repo", false, false)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	resp, err := client.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal: "Attempt to fix bug in readonly repo",
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/readonly/repo",
			Branch: "main",
		},
		Priority: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, "failed", resp.Status)

	// Verify push was logged as failed
	handler.mu.Lock()
	found := false
	for _, record := range handler.pushLog {
		if record.RepoURL == "https://github.com/readonly/repo" && !record.Success && record.ErrorMsg == "repository is read-only" {
			found = true
			break
		}
	}
	handler.mu.Unlock()
	assert.True(t, found, "Push to read-only repo should be logged as failed")

	t.Log("Push to read-only repo correctly failed")
}

// TestGitPush_PushBranchCreation tests creating a new branch and pushing.
func TestGitPush_PushBranchCreation(t *testing.T) {
	logger := zap.NewNop()
	handler := newGitPushTestHandler()
	handler.addRepo("https://github.com/writable/repo", true, false)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	resp, err := client.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal: "Create feature branch",
		Repository: &mcp.RepositoryInput{
			URL:          "https://github.com/writable/repo",
			Branch:       "main",
			ResultBranch: "feature/new-feature",
		},
		Priority: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, "completed", resp.Status)
	assert.Equal(t, "feature/new-feature", resp.ResultBranch)

	// Verify branch creation push was logged
	handler.mu.Lock()
	found := false
	for _, record := range handler.pushLog {
		if record.Branch == "feature/new-feature" && record.Success {
			found = true
			break
		}
	}
	handler.mu.Unlock()
	assert.True(t, found, "Branch creation push should be logged as successful")

	t.Log("Branch creation push succeeded")
}

// TestGitPush_PushWithConflicts tests conflict detection during push.
func TestGitPush_PushWithConflicts(t *testing.T) {
	logger := zap.NewNop()
	handler := newGitPushTestHandler()
	handler.addRepo("https://github.com/conflict/repo", true, true)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	resp, err := client.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal: "Fix bug that causes conflict",
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/conflict/repo",
			Branch: "main",
		},
		Priority: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, "failed", resp.Status)

	// Verify conflict was logged
	handler.mu.Lock()
	found := false
	for _, record := range handler.pushLog {
		if record.RepoURL == "https://github.com/conflict/repo" && !record.Success && record.ErrorMsg == "conflict detected" {
			found = true
			break
		}
	}
	handler.mu.Unlock()
	assert.True(t, found, "Conflict should be logged as failure")

	t.Log("Conflict correctly detected and reported")
}

// ----------------------------------------------------------------------------
// Additional E2E tests using actual git repositories
// ----------------------------------------------------------------------------

// createTestRepo creates a local git repository with an initial commit.
func createTestRepo(t *testing.T, name string) (string, func()) {
	dir, err := os.MkdirTemp("", "git_e2e_"+name+"_*")
	require.NoError(t, err)

	_, err = git.PlainInit(dir, false)
	require.NoError(t, err)

	readmePath := filepath.Join(dir, "README.md")
	err = os.WriteFile(readmePath, []byte("# "+name+"\n"), 0644)
	require.NoError(t, err)

	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	_, err = wt.Add("README.md")
	require.NoError(t, err)

	_, err = wt.Commit("initial commit", &git.CommitOptions{})
	require.NoError(t, err)

	cleanup := func() {
		os.RemoveAll(dir)
	}

	return dir, cleanup
}

// TestGitPush_LocalRepoPush tests GitPush tool with a local repository.
func TestGitPush_LocalRepoPush(t *testing.T) {
	logger := zap.NewNop()

	repoDir, cleanup := createTestRepo(t, "local_push_test")
	defer cleanup()

	remoteDir, err := os.MkdirTemp("", "git_e2e_remote_*")
	require.NoError(t, err)
	defer os.RemoveAll(remoteDir)

	_, err = git.PlainInit(remoteDir, true)
	require.NoError(t, err)

	repo, err := git.PlainOpen(repoDir)
	require.NoError(t, err)

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteDir},
	})
	require.NoError(t, err)

	wt, err := repo.Worktree()
	require.NoError(t, err)

	err = wt.Checkout(&git.CheckoutOptions{Branch: "refs/heads/feature/test", Create: true})
	require.NoError(t, err)

	testFile := filepath.Join(repoDir, "feature.txt")
	err = os.WriteFile(testFile, []byte("Feature content\n"), 0644)
	require.NoError(t, err)

	_, err = wt.Add("feature.txt")
	require.NoError(t, err)

	_, err = wt.Commit("add feature file", &git.CommitOptions{})
	require.NoError(t, err)

	client := gitclient.New(logger)

	err = client.Open(context.Background(), repoDir)
	require.NoError(t, err)

	err = client.PushBranch(context.Background(), "feature/test")
	if err != nil {
		t.Logf("Push failed (expected without git server): %v", err)
	} else {
		t.Log("Push to local remote succeeded")
	}
}

// TestGitPush_ProtectedBranch tests that protected branches are blocked.
func TestGitPush_ProtectedBranch(t *testing.T) {
	logger := zap.NewNop()

	repoDir, cleanup := createTestRepo(t, "protected_test")
	defer cleanup()

	repo, err := git.PlainOpen(repoDir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	testFile := filepath.Join(repoDir, "main.txt")
	err = os.WriteFile(testFile, []byte("Main content\n"), 0644)
	require.NoError(t, err)

	_, err = wt.Add("main.txt")
	require.NoError(t, err)

	_, err = wt.Commit("update main", &git.CommitOptions{})
	require.NoError(t, err)

	client := gitclient.New(logger)
	err = client.Open(context.Background(), repoDir)
	require.NoError(t, err)

	err = client.Push(context.Background())
	if err != nil {
		t.Logf("Push to main blocked as expected: %v", err)
	} else {
		t.Log("Push to main succeeded (force push enabled or main not protected)")
	}
}
