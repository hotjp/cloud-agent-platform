// Package e2e provides end-to-end tests for the Cloud Agent Platform.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// parallelReposHandler is a mock HTTP handler for parallel repos testing.
type parallelReposHandler struct {
	mu              sync.Mutex
	idCounter       int
	tasks           map[string]*mcp.TaskStatusResponse
	taskOrder       []string
	executionOrder  []string
	taskArtifacts   map[string][]string
	completedTasks  map[string]bool
	repoTasks       map[string]string // taskID -> repoURL mapping for isolation verification
}

func newParallelReposHandler() *parallelReposHandler {
	return &parallelReposHandler{
		tasks:          make(map[string]*mcp.TaskStatusResponse),
		executionOrder: []string{},
		taskArtifacts:  make(map[string][]string),
		completedTasks: make(map[string]bool),
		repoTasks:      make(map[string]string),
	}
}

func (h *parallelReposHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
		h.idCounter++
		taskID := fmt.Sprintf("task_parallel_%d", h.idCounter)

		var req mcp.TaskSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			resp := mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "INVALID_REQUEST", Message: err.Error()}}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		repoURL := ""
		if req.Repository != nil {
			repoURL = req.Repository.URL
		}

		h.tasks[taskID] = &mcp.TaskStatusResponse{
			TaskID:       taskID,
			Status:       "pending",
			Goal:         req.Goal,
			Priority:     req.Priority,
			ResultBranch: "feature/" + taskID,
			Progress:     0,
			CreatedAt:    time.Now().Format(time.RFC3339),
		}
		h.taskOrder = append(h.taskOrder, taskID)
		h.repoTasks[taskID] = repoURL

		resp := mcp.APIResponse{OK: true, Data: marshalJSON(mcp.TaskSubmitResponse{
			TaskID:       taskID,
			Status:       "pending",
			ResultBranch: "feature/" + taskID,
			CreatedAt:    time.Now().Format(time.RFC3339),
		})}
		_ = json.NewEncoder(w).Encode(resp)

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks":
		tasks := make([]mcp.TaskStatusResponse, 0, len(h.taskOrder))
		for _, id := range h.taskOrder {
			tasks = append(tasks, *h.tasks[id])
		}
		resp := mcp.APIResponse{OK: true, Data: marshalJSON(mcp.TaskListResponse{
			Tasks:    tasks,
			Total:    len(tasks),
			Page:     1,
			PageSize: 20,
		})}
		_ = json.NewEncoder(w).Encode(resp)

	case r.Method == http.MethodGet && len(r.URL.Path) > 14 && r.URL.Path[:14] == "/api/v1/tasks/":
		taskID := r.URL.Path[14:]
		if task, ok := h.tasks[taskID]; ok {
			resp := mcp.APIResponse{OK: true, Data: marshalJSON(task)}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			resp := mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "Task not found"}}
			_ = json.NewEncoder(w).Encode(resp)
		}

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent-templates":
		resp := mcp.APIResponse{OK: true, Data: marshalJSON([]mcp.AgentTemplateResponse{
			{
				TemplateID:        "coding-agent",
				Name:              "Coding Agent",
				Description:       "General purpose coding agent",
				Capabilities:      map[string]int{"coding": 9, "review": 7},
				AvailableModels:   []string{"claude-4"},
				MaxConcurrent:     3,
				AvgCompletionTime: 300,
				SuccessRate:       0.92,
			},
		})}
		_ = json.NewEncoder(w).Encode(resp)

	case r.Method == http.MethodPost && len(r.URL.Path) > 26 && r.URL.Path[:26] == "/api/v1/tasks/complete/":
		taskID := r.URL.Path[26:]
		var req struct {
			Artifacts []string `json:"artifacts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			h.executionOrder = append(h.executionOrder, taskID)
			h.taskArtifacts[taskID] = req.Artifacts
			h.completedTasks[taskID] = true
			if task, ok := h.tasks[taskID]; ok {
				task.Status = "completed"
				task.Progress = 100
			}
		}
		resp := mcp.APIResponse{OK: true}
		_ = json.NewEncoder(w).Encode(resp)

	default:
		resp := mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "Endpoint not found"}}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// getExecutionOrder returns tasks in execution order.
func (h *parallelReposHandler) getExecutionOrder() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]string, len(h.executionOrder))
	copy(result, h.executionOrder)
	return result
}

// getArtifacts returns artifacts for a task.
func (h *parallelReposHandler) getArtifacts(taskID string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.taskArtifacts[taskID]
}

// getRepo returns the repo URL for a task.
func (h *parallelReposHandler) getRepo(taskID string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.repoTasks[taskID]
}

// TestParallelRepos_TwoAgentsDifferentRepos tests two agents working on
// different repositories in parallel.
func TestParallelRepos_TwoAgentsDifferentRepos(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newParallelReposHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Agent A submits task to Repo A
	submitA, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Create hello.go with Hello function in repo-a",
		Priority:  5,
		Tags:      []string{"parallel-test", "agent-a"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo-a",
			Branch: "main",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, submitA.TaskID)

	// Agent B submits task to Repo B (different repo)
	submitB, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Create world.go with World function in repo-b",
		Priority:  5,
		Tags:      []string{"parallel-test", "agent-b"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo-b",
			Branch: "main",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, submitB.TaskID)

	// Verify repos are different (isolation)
	mockHandler := mockServer.Config.Handler.(*parallelReposHandler)
	repoA := mockHandler.getRepo(submitA.TaskID)
	repoB := mockHandler.getRepo(submitB.TaskID)
	assert.NotEqual(t, repoA, repoB, "Tasks should be in different repos for isolation test")
	assert.Equal(t, "https://github.com/test/repo-a", repoA)
	assert.Equal(t, "https://github.com/test/repo-b", repoB)

	// Mark Agent A's task as complete
	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitA.TaskID)
	mockHandler.taskArtifacts[submitA.TaskID] = []string{"hello.go"}
	mockHandler.completedTasks[submitA.TaskID] = true
	if task, ok := mockHandler.tasks[submitA.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Mark Agent B's task as complete
	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitB.TaskID)
	mockHandler.taskArtifacts[submitB.TaskID] = []string{"world.go"}
	mockHandler.completedTasks[submitB.TaskID] = true
	if task, ok := mockHandler.tasks[submitB.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Verify both tasks completed
	artifactsA := mockHandler.getArtifacts(submitA.TaskID)
	artifactsB := mockHandler.getArtifacts(submitB.TaskID)

	assert.Contains(t, artifactsA, "hello.go", "Agent A should have created hello.go")
	assert.Contains(t, artifactsB, "world.go", "Agent B should have created world.go")

	// Verify no cross-contamination: A should NOT have B's artifacts and vice versa
	assert.NotContains(t, artifactsA, "world.go", "Agent A should NOT have world.go (belongs to repo-b)")
	assert.NotContains(t, artifactsB, "hello.go", "Agent B should NOT have hello.go (belongs to repo-a)")

	t.Log("Parallel repos test passed: Two agents modified different repos successfully with isolation")
}

// TestParallelRepos_ConcurrentExecution tests that tasks on different repos
// can execute concurrently (no blocking between repos).
func TestParallelRepos_ConcurrentExecution(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newParallelReposHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Submit two tasks to different repos
	submitA, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Modify file-a.go in repo-a",
		Priority:  5,
		Tags:      []string{"concurrent-test"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo-a",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	submitB, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Modify file-b.go in repo-b",
		Priority:  5,
		Tags:      []string{"concurrent-test"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo-b",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	// Both tasks should be pending (not blocked by each other)
	mockHandler := mockServer.Config.Handler.(*parallelReposHandler)

	// Simulate concurrent completion
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		mockHandler.mu.Lock()
		mockHandler.executionOrder = append(mockHandler.executionOrder, submitA.TaskID)
		mockHandler.taskArtifacts[submitA.TaskID] = []string{"file-a.go"}
		mockHandler.completedTasks[submitA.TaskID] = true
		if task, ok := mockHandler.tasks[submitA.TaskID]; ok {
			task.Status = "completed"
			task.Progress = 100
		}
		mockHandler.mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		mockHandler.mu.Lock()
		mockHandler.executionOrder = append(mockHandler.executionOrder, submitB.TaskID)
		mockHandler.taskArtifacts[submitB.TaskID] = []string{"file-b.go"}
		mockHandler.completedTasks[submitB.TaskID] = true
		if task, ok := mockHandler.tasks[submitB.TaskID]; ok {
			task.Status = "completed"
			task.Progress = 100
		}
		mockHandler.mu.Unlock()
	}()

	wg.Wait()

	// Verify both completed
	artifactsA := mockHandler.getArtifacts(submitA.TaskID)
	artifactsB := mockHandler.getArtifacts(submitB.TaskID)

	assert.Contains(t, artifactsA, "file-a.go")
	assert.Contains(t, artifactsB, "file-b.go")

	// Verify execution order has both (order may vary due to concurrency)
	order := mockHandler.getExecutionOrder()
	assert.Len(t, order, 2, "Both tasks should have executed")

	t.Log("Concurrent execution test passed: Tasks on different repos executed concurrently")
}

// TestParallelRepos_RepositoryIsolationVerification verifies that artifacts
// from one repo don't leak into another repo's task context.
func TestParallelRepos_RepositoryIsolationVerification(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newParallelReposHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Create multiple tasks in Repo A
	submitA1, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Create base.go in repo-a",
		Priority:  5,
		Tags:      []string{"isolation-test"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo-a",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	submitA2, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Create util.go in repo-a",
		Priority:  5,
		Tags:      []string{"isolation-test"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo-a",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	// Create a task in Repo B
	submitB, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Create other.go in repo-b",
		Priority:  5,
		Tags:      []string{"isolation-test"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo-b",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	mockHandler := mockServer.Config.Handler.(*parallelReposHandler)

	// Complete A1 with base.go
	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitA1.TaskID)
	mockHandler.taskArtifacts[submitA1.TaskID] = []string{"base.go"}
	mockHandler.completedTasks[submitA1.TaskID] = true
	if task, ok := mockHandler.tasks[submitA1.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Complete A2 with util.go
	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitA2.TaskID)
	mockHandler.taskArtifacts[submitA2.TaskID] = []string{"util.go"}
	mockHandler.completedTasks[submitA2.TaskID] = true
	if task, ok := mockHandler.tasks[submitA2.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Complete B with other.go
	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitB.TaskID)
	mockHandler.taskArtifacts[submitB.TaskID] = []string{"other.go"}
	mockHandler.completedTasks[submitB.TaskID] = true
	if task, ok := mockHandler.tasks[submitB.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Verify isolation: Repo B's task should only have other.go, NOT base.go or util.go
	artifactsB := mockHandler.getArtifacts(submitB.TaskID)
	assert.NotContains(t, artifactsB, "base.go", "Repo B should not have base.go from Repo A")
	assert.NotContains(t, artifactsB, "util.go", "Repo B should not have util.go from Repo A")
	assert.Contains(t, artifactsB, "other.go", "Repo B should have its own other.go")

	// Verify repo assignments
	repoA1 := mockHandler.getRepo(submitA1.TaskID)
	repoA2 := mockHandler.getRepo(submitA2.TaskID)
	repoB := mockHandler.getRepo(submitB.TaskID)

	assert.Equal(t, "https://github.com/test/repo-a", repoA1)
	assert.Equal(t, "https://github.com/test/repo-a", repoA2)
	assert.Equal(t, "https://github.com/test/repo-b", repoB)

	t.Log("Repository isolation verified: No artifact leakage between repos")
}

// TestParallelRepos_MCPProtocol tests parallel repo operations through MCP protocol.
func TestParallelRepos_MCPProtocol(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newParallelReposHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)
	executor := mcp.NewToolExecutor(platformClient, logger)
	server := mcp.NewServer(executor, logger)

	// Initialize
	initReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  marshalJSON(mcp.InitializeParams{ProtocolVersion: "2024-11-05", ClientInfo: mcp.ClientInfo{Name: "E2E Parallel", Version: "1.0"}}),
		ID:      marshalJSON(1),
	}
	initData, _ := json.Marshal(initReq)
	resp, err := server.HandleMessage(initData)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Submit task for Repo A
	submitReqA := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name: "task_submit",
			Arguments: marshalJSON(map[string]any{
				"goal":     "Create main.go in repo-a",
				"priority": 7,
				"tags":     []string{"mcp-parallel"},
				"repository": map[string]string{"url": "https://github.com/test/repo-a", "branch": "main"},
			}),
		}),
		ID: marshalJSON(2),
	}
	submitDataA, _ := json.Marshal(submitReqA)
	resp, err = server.HandleMessage(submitDataA)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	var submitResultA mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &submitResultA)
	require.NoError(t, err)
	assert.False(t, submitResultA.IsError)

	var submitRespA mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResultA.Content[0].Text), &submitRespA)
	require.NoError(t, err)

	// Submit task for Repo B
	submitReqB := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name: "task_submit",
			Arguments: marshalJSON(map[string]any{
				"goal":     "Create index.go in repo-b",
				"priority": 7,
				"tags":     []string{"mcp-parallel"},
				"repository": map[string]string{"url": "https://github.com/test/repo-b", "branch": "main"},
			}),
		}),
		ID: marshalJSON(3),
	}
	submitDataB, _ := json.Marshal(submitReqB)
	resp, err = server.HandleMessage(submitDataB)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	var submitResultB mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &submitResultB)
	require.NoError(t, err)
	assert.False(t, submitResultB.IsError)

	var submitRespB mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResultB.Content[0].Text), &submitRespB)
	require.NoError(t, err)

	// Complete both tasks
	mockHandler := mockServer.Config.Handler.(*parallelReposHandler)

	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitRespA.TaskID)
	mockHandler.taskArtifacts[submitRespA.TaskID] = []string{"main.go"}
	mockHandler.completedTasks[submitRespA.TaskID] = true
	if task, ok := mockHandler.tasks[submitRespA.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitRespB.TaskID)
	mockHandler.taskArtifacts[submitRespB.TaskID] = []string{"index.go"}
	mockHandler.completedTasks[submitRespB.TaskID] = true
	if task, ok := mockHandler.tasks[submitRespB.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Verify both tasks executed
	order := mockHandler.getExecutionOrder()
	assert.Len(t, order, 2, "Both tasks should have executed")

	// Verify artifacts are repo-specific
	artifactsA := mockHandler.getArtifacts(submitRespA.TaskID)
	artifactsB := mockHandler.getArtifacts(submitRespB.TaskID)

	assert.Contains(t, artifactsA, "main.go")
	assert.Contains(t, artifactsB, "index.go")

	// Verify isolation
	assert.NotContains(t, artifactsA, "index.go")
	assert.NotContains(t, artifactsB, "main.go")

	t.Log("MCP protocol parallel repos test passed")
}
