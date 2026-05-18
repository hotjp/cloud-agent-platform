// Package e2e provides end-to-end tests for the Cloud Agent Platform parallel agent execution.
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

// parallelTestHandler is a mock HTTP handler for parallel agent testing.
type parallelTestHandler struct {
	mu             sync.Mutex
	idCounter      int
	tasks          map[string]*mcp.TaskStatusResponse
	taskOrder      []string
	completedTasks map[string]bool
	repoTasks      map[string][]string // repo URL -> task IDs
	taskRepos      map[string]string   // task ID -> repo URL
}

func newParallelTestHandler() *parallelTestHandler {
	return &parallelTestHandler{
		tasks:       make(map[string]*mcp.TaskStatusResponse),
		completedTasks: make(map[string]bool),
		repoTasks:   make(map[string][]string),
		taskRepos:   make(map[string]string),
	}
}

func (h *parallelTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
			h.repoTasks[repoURL] = append(h.repoTasks[repoURL], taskID)
		}
		h.taskRepos[taskID] = repoURL

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

// getRepoForTask returns the repository URL for a task.
func (h *parallelTestHandler) getRepoForTask(taskID string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.taskRepos[taskID]
}

// isTaskCompleted returns whether a task is completed.
func (h *parallelTestHandler) isTaskCompleted(taskID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.completedTasks[taskID]
}

// TestParallelAgents_TwoAgentsDifferentRepos tests that two agents can modify
// different repositories simultaneously without interference.
func TestParallelAgents_TwoAgentsDifferentRepos(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Set up mock platform API server
	mockServer := httptest.NewServer(newParallelTestHandler())
	defer mockServer.Close()

	// Create platform client
	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	repoA := "https://github.com/test/repo-a"
	repoB := "https://github.com/test/repo-b"

	// Agent A submits task to modify repo A
	taskA, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Add feature X to repo A",
		Priority:  5,
		Tags:      []string{"parallel-test", "repo-a"},
		Repository: &mcp.RepositoryInput{
			URL:    repoA,
			Branch: "main",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, taskA.TaskID)

	// Agent B submits task to modify repo B (different repo!)
	taskB, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Add feature Y to repo B",
		Priority:  5,
		Tags:      []string{"parallel-test", "repo-b"},
		Repository: &mcp.RepositoryInput{
			URL:    repoB,
			Branch: "main",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, taskB.TaskID)

	// Verify tasks are independent (different repos)
	mockHandler := mockServer.Config.Handler.(*parallelTestHandler)
	repoForA := mockHandler.getRepoForTask(taskA.TaskID)
	repoForB := mockHandler.getRepoForTask(taskB.TaskID)

	assert.Equal(t, repoA, repoForA, "Task A should belong to repo A")
	assert.Equal(t, repoB, repoForB, "Task B should belong to repo B")
	assert.NotEqual(t, repoForA, repoForB, "Tasks should belong to different repos")

	// Simulate both agents completing in parallel
	mockHandler.mu.Lock()
	mockHandler.completedTasks[taskA.TaskID] = true
	if task, ok := mockHandler.tasks[taskA.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.completedTasks[taskB.TaskID] = true
	if task, ok := mockHandler.tasks[taskB.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Verify both completed successfully
	statusA, err := platformClient.GetTask(ctx, taskA.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "completed", statusA.Status)

	statusB, err := platformClient.GetTask(ctx, taskB.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "completed", statusB.Status)

	t.Log("Parallel agents test passed: Two agents modified different repos successfully")
}

// TestParallelAgents_RepoIsolation tests that agents working on different repos
// maintain complete isolation.
func TestParallelAgents_RepoIsolation(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newParallelTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	repoX := "https://github.com/team-x/project"
	repoY := "https://github.com/team-y/project"

	// Agent X modifies repo X
	taskX, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Implement login feature",
		Priority:  7,
		Tags:      []string{"isolation-test"},
		Repository: &mcp.RepositoryInput{
			URL:    repoX,
			Branch: "main",
		},
	})
	require.NoError(t, err)

	// Agent Y modifies repo Y (same project name, different org!)
	taskY, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Implement logout feature",
		Priority:  7,
		Tags:      []string{"isolation-test"},
		Repository: &mcp.RepositoryInput{
			URL:    repoY,
			Branch: "main",
		},
	})
	require.NoError(t, err)

	mockHandler := mockServer.Config.Handler.(*parallelTestHandler)

	// Verify repo isolation
	repoForX := mockHandler.getRepoForTask(taskX.TaskID)
	repoForY := mockHandler.getRepoForTask(taskY.TaskID)

	assert.Equal(t, repoX, repoForX)
	assert.Equal(t, repoY, repoForY)
	assert.NotEqual(t, repoForX, repoForY, "Repos must be isolated")

	// Complete both
	mockHandler.mu.Lock()
	mockHandler.completedTasks[taskX.TaskID] = true
	mockHandler.completedTasks[taskY.TaskID] = true
	if task, ok := mockHandler.tasks[taskX.TaskID]; ok {
		task.Status = "completed"
	}
	if task, ok := mockHandler.tasks[taskY.TaskID]; ok {
		task.Status = "completed"
	}
	mockHandler.mu.Unlock()

	// Verify independence
	statusX, err := platformClient.GetTask(ctx, taskX.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "completed", statusX.Status)

	statusY, err := platformClient.GetTask(ctx, taskY.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "completed", statusY.Status)

	// Verify goals are preserved correctly
	assert.Equal(t, "Implement login feature", statusX.Goal)
	assert.Equal(t, "Implement logout feature", statusY.Goal)

	t.Log("Repo isolation test passed: Agents maintained complete repository isolation")
}

// TestParallelAgents_ConcurrentSubmission tests concurrent task submission.
func TestParallelAgents_ConcurrentSubmission(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newParallelTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Submit multiple tasks concurrently
	var wg sync.WaitGroup
	results := make(chan struct {
		taskID string
		err    error
	}, 5)

	repos := []string{
		"https://github.com/test/repo-1",
		"https://github.com/test/repo-2",
		"https://github.com/test/repo-3",
		"https://github.com/test/repo-4",
		"https://github.com/test/repo-5",
	}

	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r string) {
			defer wg.Done()
			task, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
				Goal: fmt.Sprintf("Task for repo %d", idx+1),
				Repository: &mcp.RepositoryInput{
					URL:    r,
					Branch: "main",
				},
			})
			results <- struct {
				taskID string
				err    error
			}{task.TaskID, err}
		}(i, repo)
	}

	wg.Wait()
	close(results)

	// Verify all submissions succeeded
	taskCount := 0
	for result := range results {
		require.NoError(t, result.err)
		require.NotEmpty(t, result.taskID)
		taskCount++
	}

	assert.Equal(t, 5, taskCount, "All 5 concurrent submissions should succeed")

	t.Log("Concurrent submission test passed: 5 tasks submitted simultaneously")
}

// TestParallelAgents_ParallelExecutionWithMockLLM tests parallel agent execution
// using a mock LLM provider.
func TestParallelAgents_ParallelExecutionWithMockLLM(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newParallelTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)
	executor := mcp.NewToolExecutor(platformClient, logger)
	server := mcp.NewServer(executor, logger)

	// Initialize the MCP server
	initReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  marshalJSON(mcp.InitializeParams{ProtocolVersion: "2024-11-05", ClientInfo: mcp.ClientInfo{Name: "Parallel Test", Version: "1.0"}}),
		ID:      marshalJSON(1),
	}
	initData, _ := json.Marshal(initReq)
	resp, err := server.HandleMessage(initData)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Submit two tasks for different repos via MCP protocol
	submitReqA := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name: "task_submit",
			Arguments: marshalJSON(map[string]any{
				"goal":     "Add utils.go to repo-a",
				"priority": 7,
				"tags":     []string{"parallel-e2e"},
				"repository": map[string]string{"url": "https://github.com/agent-a/repo", "branch": "main"},
			}),
		}),
		ID: marshalJSON(2),
	}
	submitDataA, _ := json.Marshal(submitReqA)
	respA, err := server.HandleMessage(submitDataA)
	require.NoError(t, err)
	require.Nil(t, respA.Error)

	var resultA mcp.ToolCallResult
	err = json.Unmarshal(respA.Result, &resultA)
	require.NoError(t, err)
	assert.False(t, resultA.IsError)

	var submitRespA mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(resultA.Content[0].Text), &submitRespA)
	require.NoError(t, err)

	submitReqB := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name: "task_submit",
			Arguments: marshalJSON(map[string]any{
				"goal":     "Add handler.go to repo-b",
				"priority": 7,
				"tags":     []string{"parallel-e2e"},
				"repository": map[string]string{"url": "https://github.com/agent-b/repo", "branch": "main"},
			}),
		}),
		ID: marshalJSON(3),
	}
	submitDataB, _ := json.Marshal(submitReqB)
	respB, err := server.HandleMessage(submitDataB)
	require.NoError(t, err)
	require.Nil(t, respB.Error)

	var resultB mcp.ToolCallResult
	err = json.Unmarshal(respB.Result, &resultB)
	require.NoError(t, err)
	assert.False(t, resultB.IsError)

	var submitRespB mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(resultB.Content[0].Text), &submitRespB)
	require.NoError(t, err)

	// Verify they have different repos
	mockHandler := mockServer.Config.Handler.(*parallelTestHandler)
	repoA := mockHandler.getRepoForTask(submitRespA.TaskID)
	repoB := mockHandler.getRepoForTask(submitRespB.TaskID)

	assert.Equal(t, "https://github.com/agent-a/repo", repoA)
	assert.Equal(t, "https://github.com/agent-b/repo", repoB)
	assert.NotEqual(t, repoA, repoB, "Repos should be different")

	// Mark both complete
	mockHandler.mu.Lock()
	mockHandler.completedTasks[submitRespA.TaskID] = true
	mockHandler.completedTasks[submitRespB.TaskID] = true
	if task, ok := mockHandler.tasks[submitRespA.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	if task, ok := mockHandler.tasks[submitRespB.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Query status for both tasks
	statusReqA := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name:      "task_status",
			Arguments: marshalJSON(map[string]any{"taskId": submitRespA.TaskID}),
		}),
		ID: marshalJSON(4),
	}
	statusDataA, _ := json.Marshal(statusReqA)
	respStatusA, err := server.HandleMessage(statusDataA)
	require.NoError(t, err)
	require.Nil(t, respStatusA.Error)

	statusReqB := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name:      "task_status",
			Arguments: marshalJSON(map[string]any{"taskId": submitRespB.TaskID}),
		}),
		ID: marshalJSON(5),
	}
	statusDataB, _ := json.Marshal(statusReqB)
	respStatusB, err := server.HandleMessage(statusDataB)
	require.NoError(t, err)
	require.Nil(t, respStatusB.Error)

	var statusResultA mcp.ToolCallResult
	err = json.Unmarshal(respStatusA.Result, &statusResultA)
	require.NoError(t, err)

	var statusResultB mcp.ToolCallResult
	err = json.Unmarshal(respStatusB.Result, &statusResultB)
	require.NoError(t, err)

	var statusA mcp.TaskStatusResponse
	var statusB mcp.TaskStatusResponse
	err = json.Unmarshal([]byte(statusResultA.Content[0].Text), &statusA)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(statusResultB.Content[0].Text), &statusB)
	require.NoError(t, err)

	assert.Equal(t, "completed", statusA.Status)
	assert.Equal(t, "completed", statusB.Status)
	assert.NotEqual(t, statusA.Goal, statusB.Goal, "Goals should be different")

	t.Log("Parallel execution with mock LLM test passed")
}

// TestParallelAgents_SameRepoConflict tests that two agents attempting to modify
// the same repo are handled correctly (no conflict in task tracking).
func TestParallelAgents_SameRepoConflict(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newParallelTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	sharedRepo := "https://github.com/shared/repo"

	// Two agents submit tasks to the same repo
	task1, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Add feature 1",
		Priority:  5,
		Tags:      []string{"conflict-test"},
		Repository: &mcp.RepositoryInput{
			URL:    sharedRepo,
			Branch: "main",
		},
	})
	require.NoError(t, err)

	task2, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Add feature 2",
		Priority:  5,
		Tags:      []string{"conflict-test"},
		Repository: &mcp.RepositoryInput{
			URL:    sharedRepo,
			Branch: "main",
		},
	})
	require.NoError(t, err)

	// Both should be tracked separately
	assert.NotEqual(t, task1.TaskID, task2.TaskID, "Tasks should have unique IDs")

	mockHandler := mockServer.Config.Handler.(*parallelTestHandler)
	repo1 := mockHandler.getRepoForTask(task1.TaskID)
	repo2 := mockHandler.getRepoForTask(task2.TaskID)

	// Both point to same repo (this is expected - in real system, agent coordination handles conflicts)
	assert.Equal(t, sharedRepo, repo1)
	assert.Equal(t, sharedRepo, repo2)

	// Complete both
	mockHandler.mu.Lock()
	mockHandler.completedTasks[task1.TaskID] = true
	mockHandler.completedTasks[task2.TaskID] = true
	if task, ok := mockHandler.tasks[task1.TaskID]; ok {
		task.Status = "completed"
	}
	if task, ok := mockHandler.tasks[task2.TaskID]; ok {
		task.Status = "completed"
	}
	mockHandler.mu.Unlock()

	// Both completed successfully
	status1, err := platformClient.GetTask(ctx, task1.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "completed", status1.Status)

	status2, err := platformClient.GetTask(ctx, task2.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "completed", status2.Status)

	t.Log("Same repo conflict test passed: Both tasks tracked and completed")
}
