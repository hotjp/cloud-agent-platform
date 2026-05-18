// Package e2e provides end-to-end tests for the Cloud Agent Platform concurrent user scenarios.
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

// concurrentUserTestHandler is a mock HTTP handler that tracks user-task relationships.
type concurrentUserTestHandler struct {
	mu              sync.Mutex
	idCounter       int
	tasks           map[string]*mcp.TaskStatusResponse
	taskOrder       []string
	taskOwners      map[string]string // task ID -> user token
	userTasks       map[string][]string // user token -> task IDs
	completedTasks  map[string]bool
}

func newConcurrentUserTestHandler() *concurrentUserTestHandler {
	return &concurrentUserTestHandler{
		tasks:          make(map[string]*mcp.TaskStatusResponse),
		taskOwners:     make(map[string]string),
		userTasks:      make(map[string][]string),
		completedTasks: make(map[string]bool),
	}
}

func (h *concurrentUserTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	// Extract user token from Authorization header
	userToken := r.Header.Get("Authorization")
	if len(userToken) > 7 && userToken[:7] == "Bearer " {
		userToken = userToken[7:]
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
		h.idCounter++
		taskID := fmt.Sprintf("task_concurrent_%d", h.idCounter)

		var req mcp.TaskSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			resp := mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "INVALID_REQUEST", Message: err.Error()}}
			_ = json.NewEncoder(w).Encode(resp)
			return
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
		h.taskOwners[taskID] = userToken
		h.userTasks[userToken] = append(h.userTasks[userToken], taskID)

		resp := mcp.APIResponse{OK: true, Data: marshalJSON(mcp.TaskSubmitResponse{
			TaskID:       taskID,
			Status:       "pending",
			ResultBranch: "feature/" + taskID,
			CreatedAt:    time.Now().Format(time.RFC3339),
		})}
		_ = json.NewEncoder(w).Encode(resp)

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks":
		// List tasks - filter by user token (user isolation)
		userTaskIDs := h.userTasks[userToken]
		tasks := make([]mcp.TaskStatusResponse, 0, len(userTaskIDs))
		for _, id := range userTaskIDs {
			if task, ok := h.tasks[id]; ok {
				tasks = append(tasks, *task)
			}
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
		// Verify task ownership (user isolation)
		if owner, ok := h.taskOwners[taskID]; ok {
			if owner != userToken {
				resp := mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "FORBIDDEN", Message: "Task belongs to another user"}}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
		}
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
		// Verify task ownership
		if owner, ok := h.taskOwners[taskID]; ok {
			if owner != userToken {
				resp := mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "FORBIDDEN", Message: "Task belongs to another user"}}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
		}
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

// getUserTaskCount returns the number of tasks owned by a user.
func (h *concurrentUserTestHandler) getUserTaskCount(userToken string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.userTasks[userToken])
}

// getTaskOwner returns the owner token of a task.
func (h *concurrentUserTestHandler) getTaskOwner(taskID string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.taskOwners[taskID]
}

// TestConcurrentUsers_ThreeUsersSubmitTasks simultaneously tests that 3 different
// users can submit tasks concurrently and that task isolation is maintained.
func TestConcurrentUsers_ThreeUsersSubmitTasks(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newConcurrentUserTestHandler())
	defer mockServer.Close()

	// Create 3 different user clients with different tokens
	user1Client := mcp.NewPlatformClient(mockServer.URL, "user1-jwt-token", logger)
	user2Client := mcp.NewPlatformClient(mockServer.URL, "user2-jwt-token", logger)
	user3Client := mcp.NewPlatformClient(mockServer.URL, "user3-jwt-token", logger)

	ctx := context.Background()

	// Channels to collect results
	type submitResult struct {
		taskID  string
		userIdx int
		err     error
	}
	results := make(chan submitResult, 3)

	var wg sync.WaitGroup

	// User 1 submits task
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := user1Client.SubmitTask(ctx, mcp.TaskSubmitRequest{
			Goal:      "User 1: Implement login feature",
			Priority:  8,
			Tags:      []string{"user1", "auth"},
			Repository: &mcp.RepositoryInput{
				URL:    "https://github.com/user1/project",
				Branch: "main",
			},
		})
		if err != nil {
			results <- submitResult{err: err, userIdx: 1}
			return
		}
		results <- submitResult{taskID: resp.TaskID, userIdx: 1}
	}()

	// User 2 submits task
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := user2Client.SubmitTask(ctx, mcp.TaskSubmitRequest{
			Goal:      "User 2: Fix bug in payment module",
			Priority:  9,
			Tags:      []string{"user2", "bugfix"},
			Repository: &mcp.RepositoryInput{
				URL:    "https://github.com/user2/project",
				Branch: "main",
			},
		})
		if err != nil {
			results <- submitResult{err: err, userIdx: 2}
			return
		}
		results <- submitResult{taskID: resp.TaskID, userIdx: 2}
	}()

	// User 3 submits task
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := user3Client.SubmitTask(ctx, mcp.TaskSubmitRequest{
			Goal:      "User 3: Add new API endpoint",
			Priority:  7,
			Tags:      []string{"user3", "api"},
			Repository: &mcp.RepositoryInput{
				URL:    "https://github.com/user3/project",
				Branch: "main",
			},
		})
		if err != nil {
			results <- submitResult{err: err, userIdx: 3}
			return
		}
		results <- submitResult{taskID: resp.TaskID, userIdx: 3}
	}()

	// Wait for all submissions to complete
	wg.Wait()
	close(results)

	// Collect results
	taskIDs := make(map[int]string)
	for res := range results {
		require.NoError(t, res.err, "User %d should successfully submit task", res.userIdx)
		require.NotEmpty(t, res.taskID, "User %d should receive a task ID", res.userIdx)
		taskIDs[res.userIdx] = res.taskID
	}

	// Verify all 3 tasks were created (1 per user)
	mockHandler := mockServer.Config.Handler.(*concurrentUserTestHandler)
	assert.Equal(t, 1, mockHandler.getUserTaskCount("user1-jwt-token"), "User 1 should have 1 task")
	assert.Equal(t, 1, mockHandler.getUserTaskCount("user2-jwt-token"), "User 2 should have 1 task")
	assert.Equal(t, 1, mockHandler.getUserTaskCount("user3-jwt-token"), "User 3 should have 1 task")

	// Verify task ownership
	assert.Equal(t, "user1-jwt-token", mockHandler.getTaskOwner(taskIDs[1]), "Task 1 should belong to user 1")
	assert.Equal(t, "user2-jwt-token", mockHandler.getTaskOwner(taskIDs[2]), "Task 2 should belong to user 2")
	assert.Equal(t, "user3-jwt-token", mockHandler.getTaskOwner(taskIDs[3]), "Task 3 should belong to user 3")

	// Verify each user can see their own task via list
	list1, err := user1Client.ListTasks(ctx, "", nil, 10)
	require.NoError(t, err)
	assert.Len(t, list1.Tasks, 1, "User 1 should see only 1 task")
	assert.Equal(t, taskIDs[1], list1.Tasks[0].TaskID)

	list2, err := user2Client.ListTasks(ctx, "", nil, 10)
	require.NoError(t, err)
	assert.Len(t, list2.Tasks, 1, "User 2 should see only 1 task")
	assert.Equal(t, taskIDs[2], list2.Tasks[0].TaskID)

	list3, err := user3Client.ListTasks(ctx, "", nil, 10)
	require.NoError(t, err)
	assert.Len(t, list3.Tasks, 1, "User 3 should see only 1 task")
	assert.Equal(t, taskIDs[3], list3.Tasks[0].TaskID)

	// Verify each user can get their own task status
	status1, err := user1Client.GetTask(ctx, taskIDs[1])
	require.NoError(t, err)
	assert.Equal(t, "User 1: Implement login feature", status1.Goal)

	status2, err := user2Client.GetTask(ctx, taskIDs[2])
	require.NoError(t, err)
	assert.Equal(t, "User 2: Fix bug in payment module", status2.Goal)

	status3, err := user3Client.GetTask(ctx, taskIDs[3])
	require.NoError(t, err)
	assert.Equal(t, "User 3: Add new API endpoint", status3.Goal)

	t.Log("Three concurrent users test passed: All users submitted tasks and saw proper task isolation")
}

// TestConcurrentUsers_TaskIsolation tests that users cannot see or access other users' tasks.
func TestConcurrentUsers_TaskIsolation(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newConcurrentUserTestHandler())
	defer mockServer.Close()

	user1Client := mcp.NewPlatformClient(mockServer.URL, "user1-token", logger)
	user2Client := mcp.NewPlatformClient(mockServer.URL, "user2-token", logger)

	ctx := context.Background()

	// User 1 creates a task
	task1, err := user1Client.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:     "User 1 private task",
		Priority: 5,
		Tags:    []string{"private"},
	})
	require.NoError(t, err)

	// User 2 tries to list tasks - should see empty list (not user1's task)
	listByUser2, err := user2Client.ListTasks(ctx, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, listByUser2.Tasks, "User 2 should not see User 1's tasks")

	// User 2 tries to get User 1's task - should get error
	_, err = user2Client.GetTask(ctx, task1.TaskID)
	require.Error(t, err, "User 2 should not be able to get User 1's task")
	assert.Contains(t, err.Error(), "FORBIDDEN", "Should get forbidden error")

	t.Log("Task isolation test passed: Users cannot access other users' tasks")
}

// TestConcurrentUsers_ConcurrentTaskOperations tests concurrent operations
// (submit, list, status) from multiple users.
func TestConcurrentUsers_ConcurrentTaskOperations(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newConcurrentUserTestHandler())
	defer mockServer.Close()

	// Create 3 user clients
	clients := []*mcp.PlatformClient{
		mcp.NewPlatformClient(mockServer.URL, "alice-token", logger),
		mcp.NewPlatformClient(mockServer.URL, "bob-token", logger),
		mcp.NewPlatformClient(mockServer.URL, "charlie-token", logger),
	}

	ctx := context.Background()

	// Each user submits a task concurrently
	var submitWg sync.WaitGroup
	submitResults := make(chan struct {
		userIdx int
		taskID  string
		err     error
	}, 3)

	for i, client := range clients {
		submitWg.Add(1)
		go func(idx int, c *mcp.PlatformClient) {
			defer submitWg.Done()
			resp, err := c.SubmitTask(ctx, mcp.TaskSubmitRequest{
				Goal:      fmt.Sprintf("Task from user %d", idx),
				Priority:  5,
				Tags:      []string{fmt.Sprintf("user%d", idx)},
			})
			submitResults <- struct {
				userIdx int
				taskID  string
				err     error
			}{idx, resp.TaskID, err}
		}(i, client)
	}

	submitWg.Wait()
	close(submitResults)

	// Collect task IDs
	taskIDs := make(map[int]string)
	for res := range submitResults {
		require.NoError(t, res.err)
		taskIDs[res.userIdx] = res.taskID
	}

	// Concurrent list operations
	var listWg sync.WaitGroup
	listResults := make(chan struct {
		userIdx    int
		taskCount  int
		err        error
	}, 3)

	for i, client := range clients {
		listWg.Add(1)
		go func(idx int, c *mcp.PlatformClient) {
			defer listWg.Done()
			list, err := c.ListTasks(ctx, "", nil, 10)
			listResults <- struct {
				userIdx   int
				taskCount int
				err       error
			}{idx, len(list.Tasks), err}
		}(i, client)
	}

	listWg.Wait()
	close(listResults)

	// Verify each user sees only their own task
	for res := range listResults {
		require.NoError(t, res.err)
		assert.Equal(t, 1, res.taskCount, "User %d should see exactly 1 task", res.userIdx)
	}

	// Concurrent status checks
	var statusWg sync.WaitGroup
	statusResults := make(chan struct {
		userIdx int
		goal    string
		err     error
	}, 3)

	for i, client := range clients {
		statusWg.Add(1)
		go func(idx int, c *mcp.PlatformClient) {
			defer statusWg.Done()
			status, err := c.GetTask(ctx, taskIDs[idx])
			statusResults <- struct {
				userIdx int
				goal    string
				err     error
			}{idx, status.Goal, err}
		}(i, client)
	}

	statusWg.Wait()
	close(statusResults)

	// Verify each user can get their own task
	for res := range statusResults {
		require.NoError(t, res.err)
		assert.Contains(t, res.goal, fmt.Sprintf("Task from user %d", res.userIdx))
	}

	t.Log("Concurrent task operations test passed: Multiple users performed operations concurrently with proper isolation")
}

// TestConcurrentUsers_ParallelSubmissionLoad tests high-concurrency scenario
// with multiple tasks submitted by multiple users simultaneously.
func TestConcurrentUsers_ParallelSubmissionLoad(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newConcurrentUserTestHandler())
	defer mockServer.Close()

	ctx := context.Background()

	// 3 users, each submitting 3 tasks concurrently = 9 total tasks
	userTokens := []string{"load-user-1", "load-user-2", "load-user-3"}
	tasksPerUser := 3

	var wg sync.WaitGroup
	taskCounter := 0
	var counterMu sync.Mutex

	results := make(chan struct {
		userIdx int
		taskID  string
		err     error
	}, len(userTokens)*tasksPerUser)

	for userIdx, token := range userTokens {
		client := mcp.NewPlatformClient(mockServer.URL, token, logger)

		for i := 0; i < tasksPerUser; i++ {
			wg.Add(1)
			go func(uIdx, taskNum int, c *mcp.PlatformClient, tkn string) {
				defer wg.Done()
				resp, err := c.SubmitTask(ctx, mcp.TaskSubmitRequest{
					Goal:      fmt.Sprintf("User %d task %d", uIdx, taskNum),
					Priority:  5,
					Tags:      []string{fmt.Sprintf("user%d", uIdx)},
				})
				if resp != nil && err == nil {
					counterMu.Lock()
					taskCounter++
					counterMu.Unlock()
				}
				results <- struct {
					userIdx int
					taskID  string
					err     error
				}{uIdx, resp.TaskID, err}
			}(userIdx, i, client, token)
		}
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Completed normally
	case <-time.After(10 * time.Second):
		t.Fatal("Test timed out waiting for task submissions")
	}
	close(results)

	// Verify all submissions succeeded
	successCount := 0
	for res := range results {
		require.NoError(t, res.err, "User %d task submission should succeed", res.userIdx)
		require.NotEmpty(t, res.taskID)
		successCount++
	}

	expectedTotal := len(userTokens) * tasksPerUser
	assert.Equal(t, expectedTotal, successCount, "All %d tasks should be submitted successfully", expectedTotal)

	// Verify each user has exactly tasksPerUser tasks in their list
	mockHandler := mockServer.Config.Handler.(*concurrentUserTestHandler)
	for userIdx, token := range userTokens {
		count := mockHandler.getUserTaskCount(token)
		assert.Equal(t, tasksPerUser, count, "User %d should have %d tasks", userIdx, tasksPerUser)
	}

	t.Logf("Parallel submission load test passed: %d tasks submitted by %d users concurrently",
		successCount, len(userTokens))
}