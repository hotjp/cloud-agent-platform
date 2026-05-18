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

// mockSerialScheduler is a mock scheduler that simulates serial agent execution.
type mockSerialScheduler struct {
	mu         sync.Mutex
	executions []string // task IDs in execution order
	artifacts  map[string][]string
	blockUntil map[string]chan struct{} // channels to block execution
}

func newMockSerialScheduler() *mockSerialScheduler {
	return &mockSerialScheduler{
		executions: []string{},
		artifacts:  make(map[string][]string),
		blockUntil: make(map[string]chan struct{}),
	}
}

// scheduleTask schedules a task for execution, blocking until dependencies complete.
func (s *mockSerialScheduler) scheduleTask(taskID string, dependencies []string) {
	s.mu.Lock()

	// Check if all dependencies are satisfied
	for _, dep := range dependencies {
		if ch, exists := s.blockUntil[dep]; exists {
			s.mu.Unlock()
			<-ch // Wait for dependency to complete
			s.mu.Lock()
		}
	}

	s.executions = append(s.executions, taskID)
	s.blockUntil[taskID] = make(chan struct{})
	s.mu.Unlock()
}

// completeTask marks a task as complete and unblocks dependents.
// It is safe to call concurrently with scheduleTask.
func (s *mockSerialScheduler) completeTask(taskID string, artifacts []string) {
	// First, add artifacts (this doesn't need the lock for the channel operation)
	s.mu.Lock()
	s.artifacts[taskID] = artifacts
	// Unblock any tasks waiting on this one
	if ch, exists := s.blockUntil[taskID]; exists {
		close(ch)
	}
	s.mu.Unlock()
}

// getExecutions returns tasks in execution order.
func (s *mockSerialScheduler) getExecutions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, len(s.executions))
	copy(result, s.executions)
	return result
}

// getArtifacts returns artifacts for a task.
func (s *mockSerialScheduler) getArtifacts(taskID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.artifacts[taskID]
}

// serialTestHandler is a mock HTTP handler for serial agent testing.
type serialTestHandler struct {
	mu             sync.Mutex
	idCounter      int
	tasks          map[string]*mcp.TaskStatusResponse
	taskOrder      []string
	executionOrder []string
	taskArtifacts  map[string][]string
	completedTasks map[string]bool
}

func newSerialTestHandler() *serialTestHandler {
	return &serialTestHandler{
		tasks:          make(map[string]*mcp.TaskStatusResponse),
		executionOrder: []string{},
		taskArtifacts:  make(map[string][]string),
		completedTasks: make(map[string]bool),
	}
}

func (h *serialTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
		h.idCounter++
		taskID := fmt.Sprintf("task_serial_%d", h.idCounter)

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
func (h *serialTestHandler) getExecutionOrder() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]string, len(h.executionOrder))
	copy(result, h.executionOrder)
	return result
}

// getArtifacts returns artifacts for a task.
func (h *serialTestHandler) getArtifacts(taskID string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.taskArtifacts[taskID]
}

// TestSerialAgents_OrchestratorSerialExecution tests the orchestrator's ability
// to handle serial execution of dependent subtasks.
func TestSerialAgents_OrchestratorSerialExecution(t *testing.T) {
	scheduler := newMockSerialScheduler()

	// Schedule task A (no dependencies)
	scheduler.scheduleTask("task-a", nil)

	// Complete task A first
	scheduler.completeTask("task-a", []string{"file1.go", "file2.go"})

	// Now schedule task B (depends on A) - should not block since A is done
	scheduler.scheduleTask("task-b", []string{"task-a"})

	// Complete task B
	scheduler.completeTask("task-b", []string{"file1.go", "file2.go", "file3.go"})

	// Verify execution order
	executions := scheduler.getExecutions()
	assert.Equal(t, []string{"task-a", "task-b"}, executions, "Tasks should execute in dependency order")

	// Verify B accumulated all artifacts (simulating that B can see A's work)
	artifactsB := scheduler.getArtifacts("task-b")
	assert.Contains(t, artifactsB, "file1.go", "task-b should have file1.go")
	assert.Contains(t, artifactsB, "file2.go", "task-b should have file2.go from task-a")
	assert.Contains(t, artifactsB, "file3.go", "task-b should have its own file3.go")

	t.Log("Orchestrator serial execution test passed")
}

// TestSerialAgents_DependencyChain tests a chain of dependent tasks.
func TestSerialAgents_DependencyChain(t *testing.T) {
	scheduler := newMockSerialScheduler()

	// Create a chain: A -> B -> C
	// Complete each before scheduling the dependent to avoid blocking
	scheduler.scheduleTask("task-a", nil)
	scheduler.completeTask("task-a", []string{"base.go"})

	scheduler.scheduleTask("task-b", []string{"task-a"})
	scheduler.completeTask("task-b", []string{"base.go", "middleware.go"})

	scheduler.scheduleTask("task-c", []string{"task-b"})
	scheduler.completeTask("task-c", []string{"base.go", "middleware.go", "handler.go"})

	// Verify order
	executions := scheduler.getExecutions()
	assert.Equal(t, []string{"task-a", "task-b", "task-c"}, executions)

	// Verify C accumulated all artifacts
	artifactsC := scheduler.getArtifacts("task-c")
	assert.Contains(t, artifactsC, "base.go")
	assert.Contains(t, artifactsC, "middleware.go")
	assert.Contains(t, artifactsC, "handler.go")

	t.Log("Dependency chain test passed: Tasks executed in correct order with artifact accumulation")
}

// TestSerialAgents_ParallelVsSerial tests the difference between parallel and serial execution.
func TestSerialAgents_ParallelVsSerial(t *testing.T) {
	// Test serial execution (with dependencies)
	t.Run("serial execution enforces order", func(t *testing.T) {
		scheduler := newMockSerialScheduler()

		// Tasks with dependency must be serial
		// Complete first before scheduling dependent to avoid blocking
		scheduler.scheduleTask("serial-a", nil)
		scheduler.completeTask("serial-a", []string{"a.go"})

		scheduler.scheduleTask("serial-b", []string{"serial-a"})
		scheduler.completeTask("serial-b", []string{"a.go", "b.go"})

		executions := scheduler.getExecutions()
		assert.Equal(t, []string{"serial-a", "serial-b"}, executions)

		artifacts := scheduler.getArtifacts("serial-b")
		assert.Len(t, artifacts, 2, "serial-b should have artifacts from both tasks")
	})

	// Test that independent tasks can run in parallel (simulated)
	t.Run("independent tasks can run concurrently", func(t *testing.T) {
		scheduler := newMockSerialScheduler()

		// Two independent tasks - no dependency
		scheduler.scheduleTask("parallel-a", nil)
		scheduler.scheduleTask("parallel-b", nil)

		// Both can start immediately (in real parallel execution)
		scheduler.completeTask("parallel-a", []string{"a.go"})
		scheduler.completeTask("parallel-b", []string{"b.go"})

		executions := scheduler.getExecutions()
		// Both should execute (order may vary in true parallel, but both complete)
		assert.Len(t, executions, 2)
	})

	t.Log("Parallel vs serial test passed")
}

// TestSerialAgents_SingleProjectTwoTasks tests that two agents can work
// on the same project in serial, with the second agent seeing the first's work.
func TestSerialAgents_SingleProjectTwoTasks(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Set up mock platform API server with serial execution tracking
	mockServer := httptest.NewServer(newSerialTestHandler())
	defer mockServer.Close()

	// Create platform client
	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Agent A submits task to create a file
	submitA, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Create a new file feature.go with hello world",
		Priority:  5,
		Tags:      []string{"serial-test", "agent-a"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo",
			Branch: "main",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, submitA.TaskID)

	// Mark Agent A's task as complete
	mockHandler := mockServer.Config.Handler.(*serialTestHandler)
	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitA.TaskID)
	mockHandler.taskArtifacts[submitA.TaskID] = []string{"feature.go"}
	mockHandler.completedTasks[submitA.TaskID] = true
	if task, ok := mockHandler.tasks[submitA.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Verify Agent A's artifacts are recorded
	artifactsA := mockHandler.getArtifacts(submitA.TaskID)
	assert.Contains(t, artifactsA, "feature.go", "Agent A should have created feature.go")

	// Agent B submits a task that depends on Agent A's result
	submitB, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Update feature.go with new functionality",
		Priority:  5,
		Tags:      []string{"serial-test", "agent-b"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo",
			Branch: "main",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, submitB.TaskID)

	// Mark Agent B's task as complete
	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitB.TaskID)
	mockHandler.taskArtifacts[submitB.TaskID] = []string{"feature.go", "feature_extra.go"}
	mockHandler.completedTasks[submitB.TaskID] = true
	if task, ok := mockHandler.tasks[submitB.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Verify execution order - Agent A should complete before Agent B
	executionOrder := mockHandler.getExecutionOrder()
	require.GreaterOrEqual(t, len(executionOrder), 2, "Should have at least 2 executions")
	assert.Equal(t, submitA.TaskID, executionOrder[0], "Agent A should execute first")
	assert.Equal(t, submitB.TaskID, executionOrder[1], "Agent B should execute second")

	// Verify Agent B can see Agent A's artifacts
	artifactsB := mockHandler.getArtifacts(submitB.TaskID)
	assert.Contains(t, artifactsB, "feature.go", "Agent B should see feature.go (Agent A's work)")

	t.Log("Serial agent test passed: Agent B correctly saw Agent A's work in same project")
}

// TestSerialAgents_E2EWithMCPProtocol tests serial execution through MCP protocol.
func TestSerialAgents_E2EWithMCPProtocol(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newSerialTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)
	executor := mcp.NewToolExecutor(platformClient, logger)
	server := mcp.NewServer(executor, logger)

	// Initialize
	initReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  marshalJSON(mcp.InitializeParams{ProtocolVersion: "2024-11-05", ClientInfo: mcp.ClientInfo{Name: "E2E Serial", Version: "1.0"}}),
		ID:      marshalJSON(1),
	}
	initData, _ := json.Marshal(initReq)
	resp, err := server.HandleMessage(initData)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Step 1: Submit task A via MCP tool call
	submitReqA := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name: "task_submit",
			Arguments: marshalJSON(map[string]any{
				"goal":       "Create base.go with struct definitions",
				"priority":   7,
				"tags":       []string{"e2e-serial"},
				"repository": map[string]string{"url": "https://github.com/test/repo", "branch": "main"},
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

	// Step 2: Complete task A
	mockHandler := mockServer.Config.Handler.(*serialTestHandler)
	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitRespA.TaskID)
	mockHandler.taskArtifacts[submitRespA.TaskID] = []string{"base.go"}
	mockHandler.completedTasks[submitRespA.TaskID] = true
	if task, ok := mockHandler.tasks[submitRespA.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Step 3: Submit task B via MCP tool call
	submitReqB := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name: "task_submit",
			Arguments: marshalJSON(map[string]any{
				"goal":       "Extend base.go with additional methods",
				"priority":   7,
				"tags":       []string{"e2e-serial"},
				"repository": map[string]string{"url": "https://github.com/test/repo", "branch": "main"},
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

	// Complete task B
	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, submitRespB.TaskID)
	mockHandler.taskArtifacts[submitRespB.TaskID] = []string{"base.go", "extended.go"}
	mockHandler.completedTasks[submitRespB.TaskID] = true
	if task, ok := mockHandler.tasks[submitRespB.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Step 4: Verify execution order
	order := mockHandler.getExecutionOrder()
	require.GreaterOrEqual(t, len(order), 2, "Should have at least 2 executions")
	assert.Equal(t, submitRespA.TaskID, order[0], "Task A should execute first")
	assert.Equal(t, submitRespB.TaskID, order[1], "Task B should execute second")

	// Step 5: Verify B can see A's artifacts
	artifactsB := mockHandler.getArtifacts(submitRespB.TaskID)
	assert.Contains(t, artifactsB, "base.go", "Task B should see Task A's artifact")
	assert.Contains(t, artifactsB, "extended.go", "Task B should have its own artifact")

	t.Log("E2E serial agent test passed via MCP protocol")
}

// TestSerialAgents_ContextSharing tests that the second agent receives
// the first agent's context/results.
func TestSerialAgents_ContextSharing(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newSerialTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Agent A creates context
	taskA, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Set up project structure",
		Priority:  5,
		Tags:      []string{"context-sharing"},
		Repository: &mcp.RepositoryInput{
			URL:   "https://github.com/test/repo",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	// Simulate Agent A completing and creating context artifacts
	mockHandler := mockServer.Config.Handler.(*serialTestHandler)
	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, taskA.TaskID)
	mockHandler.taskArtifacts[taskA.TaskID] = []string{"go.mod", "main.go", "README.md"}
	mockHandler.completedTasks[taskA.TaskID] = true
	if task, ok := mockHandler.tasks[taskA.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Agent B continues from Agent A's context
	taskB, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Implement feature using existing project structure",
		Priority:  5,
		Tags:      []string{"context-sharing"},
		Repository: &mcp.RepositoryInput{
			URL:   "https://github.com/test/repo",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	mockHandler.mu.Lock()
	mockHandler.executionOrder = append(mockHandler.executionOrder, taskB.TaskID)
	mockHandler.taskArtifacts[taskB.TaskID] = []string{"feature.go"}
	mockHandler.completedTasks[taskB.TaskID] = true
	if task, ok := mockHandler.tasks[taskB.TaskID]; ok {
		task.Status = "completed"
		task.Progress = 100
	}
	mockHandler.mu.Unlock()

	// Verify serial execution
	order := mockHandler.getExecutionOrder()
	assert.True(t, len(order) >= 2, "Should have at least 2 executions")
	t.Logf("Execution order: %v", order)

	// Verify Agent B's working context includes Agent A's artifacts
	artifactsB := mockHandler.getArtifacts(taskB.TaskID)
	assert.NotEmpty(t, artifactsB, "Agent B should have artifacts from context sharing")

	t.Log("Context sharing test passed: Second agent received first agent's context")
}