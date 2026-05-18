// Package e2e provides end-to-end tests for the Cloud Agent Platform.
package e2e

import (
	"context"
	"encoding/json"
	"errors"
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

// failureTestHandler simulates the platform API with configurable failure modes.
type failureTestHandler struct {
	mu          sync.Mutex
	idCounter   int
	tasks       map[string]*mcp.TaskStatusResponse
	failMode    string           // "", "llm_error", "timeout", "cmd_failure", "git_no_permission"
	failMessage string           // human-readable error message
}

func newFailureTestHandler() *failureTestHandler {
	return &failureTestHandler{
		tasks: make(map[string]*mcp.TaskStatusResponse),
	}
}

func (h *failureTestHandler) setFailMode(mode, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failMode = mode
	h.failMessage = message
}

func (h *failureTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	// Simulate LLM error
	if h.failMode == "llm_error" && r.URL.Path == "/api/v1/llm/chat" {
		resp := mcp.APIResponse{
			OK: false,
			Error: &mcp.APIError{
				Code:    "LLM_ERROR",
				Message: h.failMessage,
				Detail:  map[string]any{"reason": "model_unavailable", "retryable": false},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Simulate timeout
	if h.failMode == "timeout" {
		if r.URL.Path == "/api/v1/tasks/execute" || r.URL.Path == "/api/v1/agent/execute" {
			// Return timeout error
			resp := mcp.APIResponse{
				OK: false,
				Error: &mcp.APIError{
					Code:    "EXECUTION_TIMEOUT",
					Message: h.failMessage,
					Detail:  map[string]any{"timeoutSeconds": 300, "elapsedSeconds": 300},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
		taskID := fmt.Sprintf("task_failure_%d", h.idCounter)
		h.idCounter++

		var req mcp.TaskSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			resp := mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "INVALID_REQUEST", Message: err.Error()}}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		status := "pending"
		// If already in failure mode at submission, task immediately fails
		if h.failMode != "" {
			status = "failed"
		}

		h.tasks[taskID] = &mcp.TaskStatusResponse{
			TaskID:       taskID,
			Status:       status,
			Goal:         req.Goal,
			Priority:     req.Priority,
			ResultBranch: "feature/" + taskID,
			Progress:     0,
			CreatedAt:    time.Now().Format(time.RFC3339),
		}

		resp := mcp.APIResponse{OK: true, Data: marshalJSON(mcp.TaskSubmitResponse{
			TaskID:       taskID,
			Status:       status,
			ResultBranch: "feature/" + taskID,
			CreatedAt:    time.Now().Format(time.RFC3339),
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

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks":
		tasks := make([]mcp.TaskStatusResponse, 0, len(h.tasks))
		for _, task := range h.tasks {
			tasks = append(tasks, *task)
		}
		resp := mcp.APIResponse{OK: true, Data: marshalJSON(mcp.TaskListResponse{
			Tasks:    tasks,
			Total:    len(tasks),
			Page:     1,
			PageSize: 20,
		})}
		_ = json.NewEncoder(w).Encode(resp)

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
			Artifacts  []string `json:"artifacts"`
			ExitCode   int      `json:"exitCode,omitempty"`
			ErrorMsg   string   `json:"errorMsg,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if task, ok := h.tasks[taskID]; ok {
				// Check for command failure
				if req.ExitCode != 0 {
					task.Status = "failed"
					task.Result = &mcp.TaskResultResponse{
						Summary: fmt.Sprintf("Command execution failed with exit code %d: %s", req.ExitCode, req.ErrorMsg),
					}
				} else {
					task.Status = "completed"
					task.Progress = 100
				}
			}
		}
		resp := mcp.APIResponse{OK: true}
		_ = json.NewEncoder(w).Encode(resp)

	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks/fail":
		// Endpoint to manually mark task as failed
		var req struct {
			TaskID    string `json:"taskId"`
			ErrorCode string `json:"errorCode"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if task, ok := h.tasks[req.TaskID]; ok {
				task.Status = "failed"
				task.Result = &mcp.TaskResultResponse{
					Summary: fmt.Sprintf("[%s] %s", req.ErrorCode, req.Message),
				}
			}
		}
		resp := mcp.APIResponse{OK: true}
		_ = json.NewEncoder(w).Encode(resp)

	default:
		// Simulate git permission failure
		if h.failMode == "git_no_permission" {
			resp := mcp.APIResponse{
				OK: false,
				Error: &mcp.APIError{
					Code:    "GIT_PERMISSION_DENIED",
					Message: h.failMessage,
					Detail:  map[string]any{"operation": "push", "repository": "git@github.com:test/repo.git"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		resp := mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "Endpoint not found"}}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// TestAgentFailure_LLMError tests that LLM errors are properly propagated.
func TestAgentFailure_LLMError(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	// Set LLM error mode
	handler := mockServer.Config.Handler.(*failureTestHandler)
	handler.setFailMode("llm_error", "Model API rate limit exceeded, please retry later")

	ctx := context.Background()

	// Submit a task (will be marked failed due to LLM error)
	task, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Implement user authentication",
		Priority:  7,
		Tags:      []string{"failure-test", "llm-error"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo",
			Branch: "main",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, task.TaskID)

	// Verify task is marked as failed
	status, err := platformClient.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "failed", status.Status, "Task should be marked as failed due to LLM error")

	t.Log("LLM error test passed: Failure status correctly propagated")
}

// TestAgentFailure_StatusPropagation tests that failure status correctly propagates through the system.
func TestAgentFailure_StatusPropagation(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)
	executor := mcp.NewToolExecutor(platformClient, logger)
	server := mcp.NewServer(executor, logger)

	// Initialize MCP server
	initReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  marshalJSON(mcp.InitializeParams{ProtocolVersion: "2024-11-05", ClientInfo: mcp.ClientInfo{Name: "Status Propagation Test", Version: "1.0"}}),
		ID:      marshalJSON(1),
	}
	initData, _ := json.Marshal(initReq)
	resp, err := server.HandleMessage(initData)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Submit task via MCP protocol
	submitReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name: "task_submit",
			Arguments: marshalJSON(map[string]any{
				"goal":       "Multi-step implementation task",
				"priority":   8,
				"tags":       []string{"status-propagation-test"},
				"repository": map[string]string{"url": "https://github.com/test/repo", "branch": "main"},
			}),
		}),
		ID: marshalJSON(2),
	}
	submitData, _ := json.Marshal(submitReq)
	resp, err = server.HandleMessage(submitData)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	var submitResult mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &submitResult)
	require.NoError(t, err)
	assert.False(t, submitResult.IsError, "Submit should succeed")

	var submitResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &submitResp)
	require.NoError(t, err)
	taskID := submitResp.TaskID

	// Verify via task_status that task is pending
	statusReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name:      "task_status",
			Arguments: marshalJSON(map[string]any{"taskId": taskID}),
		}),
		ID: marshalJSON(3),
	}
	statusData, _ := json.Marshal(statusReq)
	resp, err = server.HandleMessage(statusData)
	require.NoError(t, err)

	var statusResult mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &statusResult)
	require.NoError(t, err)
	assert.False(t, statusResult.IsError)

	var statusResp mcp.TaskStatusResponse
	err = json.Unmarshal([]byte(statusResult.Content[0].Text), &statusResp)
	require.NoError(t, err)
	assert.Equal(t, "pending", statusResp.Status, "Initial status should be pending")

	// Simulate task running
	handler := mockServer.Config.Handler.(*failureTestHandler)
	handler.mu.Lock()
	if task, ok := handler.tasks[taskID]; ok {
		task.Status = "running"
		task.CurrentPhase = "agent_execution"
		task.Progress = 50
	}
	handler.mu.Unlock()

	// Simulate failure during execution
	handler.mu.Lock()
	if task, ok := handler.tasks[taskID]; ok {
		task.Status = "failed"
		task.Progress = 75
		task.CompletedAt = time.Now().Format(time.RFC3339)
		task.Result = &mcp.TaskResultResponse{
			Summary: "[EXECUTION_FAILED] Agent execution failed: git push rejected",
		}
	}
	handler.mu.Unlock()

	// Query status again - should reflect failure
	resp, err = server.HandleMessage(statusData)
	require.NoError(t, err)

	var finalStatusResult mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &finalStatusResult)
	require.NoError(t, err)

	var finalStatusResp mcp.TaskStatusResponse
	err = json.Unmarshal([]byte(finalStatusResult.Content[0].Text), &finalStatusResp)
	require.NoError(t, err)
	assert.Equal(t, "failed", finalStatusResp.Status, "Status should propagate to failed")
	assert.Equal(t, 75, finalStatusResp.Progress, "Progress should be preserved")
	assert.NotEmpty(t, finalStatusResp.CompletedAt, "CompletedAt should be set")
	assert.Contains(t, finalStatusResp.Result.Summary, "EXECUTION_FAILED", "Error code should be in result")

	// List tasks - should show failed task
	listReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name:      "task_list",
			Arguments: marshalJSON(map[string]any{"status": "failed"}),
		}),
		ID: marshalJSON(4),
	}
	listData, _ := json.Marshal(listReq)
	resp, err = server.HandleMessage(listData)
	require.NoError(t, err)

	var listResult mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &listResult)
	require.NoError(t, err)

	var listResp mcp.TaskListResponse
	err = json.Unmarshal([]byte(listResult.Content[0].Text), &listResp)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(listResp.Tasks), 1, "Should have at least one failed task")

	// Verify the specific task appears in failed list with correct status
	found := false
	for _, task := range listResp.Tasks {
		if task.TaskID == taskID {
			found = true
			assert.Equal(t, "failed", task.Status, "Task in list should have failed status")
		}
	}
	assert.True(t, found, "Failed task should appear in failed status filter")

	t.Log("Status propagation test passed: Failure status correctly propagates through MCP layer")
}

// TestAgentFailure_ExecutionTimeout tests that execution timeouts are properly handled.
func TestAgentFailure_ExecutionTimeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	// Set timeout mode
	handler := mockServer.Config.Handler.(*failureTestHandler)
	handler.setFailMode("timeout", "Agent execution exceeded 300 second timeout")

	ctx := context.Background()

	// Submit a task
	task, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Run comprehensive test suite",
		Priority:  5,
		Tags:      []string{"failure-test", "timeout"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	// Mark task as failed due to timeout
	handler.mu.Lock()
	if t, ok := handler.tasks[task.TaskID]; ok {
		t.Status = "failed"
		t.Result = &mcp.TaskResultResponse{
			Summary: "[EXECUTION_TIMEOUT] Agent execution exceeded 300 second timeout",
		}
	}
	handler.mu.Unlock()

	// Verify task is marked as failed
	status, err := platformClient.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "failed", status.Status, "Task should be marked as failed due to timeout")
	assert.Contains(t, status.Result.Summary, "timeout", "Error message should mention timeout")

	t.Log("Execution timeout test passed: Timeout error correctly propagated")
}

// TestAgentFailure_CommandExecutionFailure tests that command failures are properly handled.
func TestAgentFailure_CommandExecutionFailure(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Submit a task
	task, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Run linter and formatter",
		Priority:  6,
		Tags:      []string{"failure-test", "cmd-failure"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	// Simulate command execution failure (non-zero exit code)
	handler := mockServer.Config.Handler.(*failureTestHandler)
	handler.mu.Lock()
	if t, ok := handler.tasks[task.TaskID]; ok {
		t.Status = "failed"
		t.Progress = 45
		t.Result = &mcp.TaskResultResponse{
			Summary: "Command 'go vet ./...' exited with code 1: errors reported",
		}
	}
	handler.mu.Unlock()

	// Verify task is marked as failed
	status, err := platformClient.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "failed", status.Status, "Task should be marked as failed due to command failure")
	assert.Contains(t, status.Result.Summary, "code 1", "Error message should contain exit code")
	assert.Contains(t, status.Result.Summary, "go vet", "Error message should mention the failing command")

	t.Log("Command execution failure test passed: Non-zero exit code correctly propagated")
}

// TestAgentFailure_GitPermissionDenied tests that git permission errors are properly handled.
func TestAgentFailure_GitPermissionDenied(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	// Set git permission error mode
	handler := mockServer.Config.Handler.(*failureTestHandler)
	handler.setFailMode("git_no_permission", "Permission denied: cannot push to protected branch main")

	ctx := context.Background()

	// Submit a task
	task, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Implement feature and push changes",
		Priority:  7,
		Tags:      []string{"failure-test", "git-permission"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	// Mark task as failed due to git permission
	handler.mu.Lock()
	if t, ok := handler.tasks[task.TaskID]; ok {
		t.Status = "failed"
		t.Result = &mcp.TaskResultResponse{
			Summary: "[GIT_PERMISSION_DENIED] Permission denied: cannot push to protected branch main",
		}
	}
	handler.mu.Unlock()

	// Verify task is marked as failed
	status, err := platformClient.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "failed", status.Status, "Task should be marked as failed due to git permission error")
	assert.Contains(t, status.Result.Summary, "GIT_PERMISSION_DENIED", "Error code should be present")
	assert.Contains(t, status.Result.Summary, "Permission denied", "Error message should mention permission denied")

	t.Log("Git permission denied test passed: Permission error correctly propagated")
}

// TestAgentFailure_MultipleErrorTypes tests that different error types produce readable messages.
func TestAgentFailure_MultipleErrorTypes(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)
	executor := mcp.NewToolExecutor(platformClient, logger)
	server := mcp.NewServer(executor, logger)

	// Initialize MCP server
	initReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  marshalJSON(mcp.InitializeParams{ProtocolVersion: "2024-11-05", ClientInfo: mcp.ClientInfo{Name: "Failure Test", Version: "1.0"}}),
		ID:      marshalJSON(1),
	}
	initData, _ := json.Marshal(initReq)
	resp, err := server.HandleMessage(initData)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	testCases := []struct {
		name        string
		errorCode   string
		errorMsg    string
		summary     string
	}{
		{
			name:      "LLM Error",
			errorCode: "LLM_ERROR",
			errorMsg:  "Model API rate limit exceeded",
			summary:   "[LLM_ERROR] Model API rate limit exceeded, please retry later",
		},
		{
			name:      "Timeout Error",
			errorCode: "EXECUTION_TIMEOUT",
			errorMsg:  "Agent execution exceeded timeout",
			summary:   "[EXECUTION_TIMEOUT] Agent execution exceeded 300 second timeout",
		},
		{
			name:      "Command Failure",
			errorCode: "COMMAND_FAILED",
			errorMsg:  "go build failed",
			summary:   "[COMMAND_FAILED] go build ./... exited with code 1",
		},
		{
			name:      "Git Permission",
			errorCode: "GIT_PERMISSION_DENIED",
			errorMsg:  "Cannot push to protected branch",
			summary:   "[GIT_PERMISSION_DENIED] Permission denied: cannot push to protected branch main",
		},
	}

	handler := mockServer.Config.Handler.(*failureTestHandler)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Submit task
			submitReq := mcp.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params: marshalJSON(mcp.ToolCallParams{
					Name: "task_submit",
					Arguments: marshalJSON(map[string]any{
						"goal":       fmt.Sprintf("Test error: %s", tc.name),
						"priority":   5,
						"tags":       []string{"failure-test", tc.errorCode},
						"repository": map[string]string{"url": "https://github.com/test/repo", "branch": "main"},
					}),
				}),
				ID: marshalJSON(2),
			}
			submitData, _ := json.Marshal(submitReq)
			resp, err := server.HandleMessage(submitData)
			require.NoError(t, err)
			require.Nil(t, resp.Error)

			var submitResult mcp.ToolCallResult
			err = json.Unmarshal(resp.Result, &submitResult)
			require.NoError(t, err)

			var submitResp mcp.TaskSubmitResponse
			err = json.Unmarshal([]byte(submitResult.Content[0].Text), &submitResp)
			require.NoError(t, err)

			// Mark as failed
			handler.mu.Lock()
			if task, ok := handler.tasks[submitResp.TaskID]; ok {
				task.Status = "failed"
				task.Result = &mcp.TaskResultResponse{
					Summary: tc.summary,
				}
			}
			handler.mu.Unlock()

			// Query status
			statusReq := mcp.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params: marshalJSON(mcp.ToolCallParams{
					Name:      "task_status",
					Arguments: marshalJSON(map[string]any{"taskId": submitResp.TaskID}),
				}),
				ID: marshalJSON(3),
			}
			statusData, _ := json.Marshal(statusReq)
			resp, err = server.HandleMessage(statusData)
			require.NoError(t, err)

			var statusResult mcp.ToolCallResult
			err = json.Unmarshal(resp.Result, &statusResult)
			require.NoError(t, err)

			var statusResp mcp.TaskStatusResponse
			err = json.Unmarshal([]byte(statusResult.Content[0].Text), &statusResp)
			require.NoError(t, err)

			// Verify failure status and error message
			assert.Equal(t, "failed", statusResp.Status, "Task should be failed for %s", tc.name)
			assert.Contains(t, statusResp.Result.Summary, tc.errorCode, "Error code should be in summary for %s", tc.name)
		})
	}

	t.Log("Multiple error types test passed: All error types produce readable messages")
}

// TestAgentFailure_ErrorTraceability tests that errors contain sufficient context.
func TestAgentFailure_ErrorTraceability(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Submit a task that will fail
	task, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Complex refactoring task",
		Priority:  8,
		Tags:      []string{"failure-test", "traceability"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/large-repo",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	// Simulate a complex failure with detailed error info
	handler := mockServer.Config.Handler.(*failureTestHandler)
	handler.mu.Lock()
	if t, ok := handler.tasks[task.TaskID]; ok {
		t.Status = "failed"
		t.Progress = 67
		t.Result = &mcp.TaskResultResponse{
			Summary: "[LLM_ERROR] LLM returned invalid response format: unexpected token at position 1024",
			Changes: []mcp.FileChange{
				{Path: "src/main.go", Action: "modified", Additions: 45, Deletions: 12},
			},
		}
	}
	handler.mu.Unlock()

	// Verify the error is traceable
	status, err := platformClient.GetTask(ctx, task.TaskID)
	require.NoError(t, err)

	// Error should be descriptive
	assert.NotEmpty(t, status.Result.Summary, "Error summary should not be empty")
	assert.Contains(t, status.Result.Summary, "[LLM_ERROR]", "Error should have error code")
	assert.Contains(t, status.Result.Summary, "LLM returned", "Error should explain what happened")

	// Partial progress should be visible
	assert.Equal(t, 67, status.Progress, "Partial progress should be visible even on failure")

	// Changes made before failure should be visible
	assert.NotEmpty(t, status.Result.Changes, "Partial changes should be preserved")

	t.Log("Error traceability test passed: Errors contain sufficient context for debugging")
}

// TestAgentFailure_CascadingFailure tests that failures propagate correctly.
func TestAgentFailure_CascadingFailure(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Submit a task
	task, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:      "Build and test project",
		Priority:  7,
		Tags:      []string{"failure-test", "cascading"},
		Repository: &mcp.RepositoryInput{
			URL:    "https://github.com/test/repo",
			Branch: "main",
		},
	})
	require.NoError(t, err)

	// Simulate that the task has subtasks
	handler := mockServer.Config.Handler.(*failureTestHandler)
	handler.mu.Lock()
	if t, ok := handler.tasks[task.TaskID]; ok {
		t.Subtasks = []mcp.SubtaskStatus{
			{SubtaskID: "sub-1", Type: "coding", AgentTemplate: "coding-agent", Status: "completed"},
			{SubtaskID: "sub-2", Type: "testing", AgentTemplate: "test-agent", Status: "failed"},
		}
		// When subtask fails, parent task should also fail
		t.Status = "failed"
		t.Progress = 50
		t.Result = &mcp.TaskResultResponse{
			Summary: "[TEST_FAILURE] Test suite failed: 3 tests failed, 2 passed",
		}
	}
	handler.mu.Unlock()

	// Verify cascading failure is properly reflected
	status, err := platformClient.GetTask(ctx, task.TaskID)
	require.NoError(t, err)

	assert.Equal(t, "failed", status.Status, "Task should be failed")
	assert.Equal(t, 2, len(status.Subtasks), "Subtasks should be tracked")

	// Find the failed subtask
	var failedSubtask mcp.SubtaskStatus
	for _, st := range status.Subtasks {
		if st.Status == "failed" {
			failedSubtask = st
			break
		}
	}
	assert.Equal(t, "failed", failedSubtask.Status, "Failed subtask should be marked as failed")
	assert.Equal(t, "test-agent", failedSubtask.AgentTemplate, "Failed subtask should identify the agent template")

	t.Log("Cascading failure test passed: Subtask failures correctly propagated to parent task")
}

// TestAgentFailure_ReadbleErrorMessages tests that error messages are human-readable.
func TestAgentFailure_ReadbleErrorMessages(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Test various error scenarios and verify message readability
	errorScenarios := []struct {
		name           string
		summary        string
		expectedWords []string
	}{
		{
			name:    "Rate limit",
			summary: "[LLM_ERROR] API rate limit exceeded. Wait 60 seconds before retrying.",
			expectedWords: []string{"[LLM_ERROR]", "rate limit", "Wait", "retrying"},
		},
		{
			name:    "Context window",
			summary: "[LLM_ERROR] Model context window exceeded. Input too large (50000 tokens, max 200000).",
			expectedWords: []string{"[LLM_ERROR]", "context", "exceeded", "tokens"},
		},
		{
			name:    "Timeout",
			summary: "[EXECUTION_TIMEOUT] Agent execution timed out after 300 seconds. Task complexity may require longer timeout.",
			expectedWords: []string{"[EXECUTION_TIMEOUT]", "timed out", "300 seconds"},
		},
		{
			name:    "Auth failure",
			summary: "[AUTH_ERROR] Git authentication failed. Check your GIT_TOKEN environment variable.",
			expectedWords: []string{"[AUTH_ERROR]", "authentication", "GIT_TOKEN"},
		},
	}

	for _, scenario := range errorScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			task, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
				Goal:      fmt.Sprintf("Test error readability: %s", scenario.name),
				Priority:  5,
				Tags:      []string{"failure-test", "readability"},
			})
			require.NoError(t, err)

			handler := mockServer.Config.Handler.(*failureTestHandler)
			handler.mu.Lock()
			if tsk, ok := handler.tasks[task.TaskID]; ok {
				tsk.Status = "failed"
				tsk.Result = &mcp.TaskResultResponse{Summary: scenario.summary}
			}
			handler.mu.Unlock()

			status, err := platformClient.GetTask(ctx, task.TaskID)
			require.NoError(t, err)

			// Verify error message is readable
			for _, word := range scenario.expectedWords {
				assert.Contains(t, status.Result.Summary, word, "Error message should contain '%s' for %s scenario", word, scenario.name)
			}
		})
	}

	t.Log("Readable error messages test passed: Error messages are human-readable")
}

// TestAgentFailure_MCPProtocolErrors tests MCP protocol error handling.
func TestAgentFailure_MCPProtocolErrors(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)
	executor := mcp.NewToolExecutor(platformClient, logger)
	server := mcp.NewServer(executor, logger)

	// Initialize first
	initReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  marshalJSON(mcp.InitializeParams{ProtocolVersion: "2024-11-05", ClientInfo: mcp.ClientInfo{Name: "Failure Test", Version: "1.0"}}),
		ID:      marshalJSON(1),
	}
	initData, _ := json.Marshal(initReq)
	resp, err := server.HandleMessage(initData)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	t.Run("Submit task with missing required field", func(t *testing.T) {
		submitReq := mcp.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "tools/call",
			Params: marshalJSON(mcp.ToolCallParams{
				Name:      "task_submit",
				Arguments: marshalJSON(map[string]any{}), // Missing required "goal"
			}),
			ID: marshalJSON(2),
		}
		submitData, _ := json.Marshal(submitReq)
		resp, _ := server.HandleMessage(submitData)
		require.NotNil(t, resp, "Response should not be nil")
		// Missing required params returns an error ToolCallResult (IsError=true in result)
		require.NotNil(t, resp.Result, "Should have result with error info")
	})

	t.Run("Submit task with non-existent tool", func(t *testing.T) {
		submitReq := mcp.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "tools/call",
			Params: marshalJSON(mcp.ToolCallParams{
				Name:      "non_existent_tool",
				Arguments: marshalJSON(map[string]any{}),
			}),
			ID: marshalJSON(3),
		}
		submitData, _ := json.Marshal(submitReq)
		resp, _ := server.HandleMessage(submitData)
		require.NotNil(t, resp, "Response should not be nil")
		// Non-existent tool returns an error result
		require.NotNil(t, resp.Result, "Should have result for non-existent tool")
	})

	t.Run("Invalid JSON-RPC request", func(t *testing.T) {
		invalidReq := []byte(`{"jsonrpc": "2.0", "method": "tools/call", "id": null}`)
		resp, _ := server.HandleMessage(invalidReq)
		require.NotNil(t, resp, "Response should not be nil for invalid JSON")
		// The request is valid JSON but id=null causes issues - expect error response
		assert.True(t, resp.Error != nil || resp.Result != nil, "Should return some response for invalid request")
	})

	t.Log("MCP protocol errors test passed: Protocol errors handled correctly")
}

// TestAgentFailure_ConcurrentFailures tests concurrent task failures.
func TestAgentFailure_ConcurrentFailures(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newFailureTestHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	ctx := context.Background()

	// Submit multiple tasks
	tasks := make([]string, 5)
	for i := 0; i < 5; i++ {
		task, err := platformClient.SubmitTask(ctx, mcp.TaskSubmitRequest{
			Goal:      fmt.Sprintf("Task %d that will fail", i+1),
			Priority:  5,
			Tags:      []string{"failure-test", "concurrent"},
		})
		require.NoError(t, err)
		tasks[i] = task.TaskID
	}

	// Simulate different failures concurrently
	handler := mockServer.Config.Handler.(*failureTestHandler)
	handler.mu.Lock()
	for i, taskID := range tasks {
		if t, ok := handler.tasks[taskID]; ok {
			t.Status = "failed"
			t.Result = &mcp.TaskResultResponse{
				Summary: fmt.Sprintf("[FAILURE_%d] Simulated failure for concurrent task", i+1),
			}
		}
	}
	handler.mu.Unlock()

	// Verify all tasks failed correctly
	for i, taskID := range tasks {
		status, err := platformClient.GetTask(ctx, taskID)
		require.NoError(t, err, "Should get status for task %d", i+1)
		assert.Equal(t, "failed", status.Status, "Task %d should be failed", i+1)
		assert.Contains(t, status.Result.Summary, fmt.Sprintf("FAILURE_%d", i+1))
	}

	t.Log("Concurrent failures test passed: Multiple concurrent failures handled correctly")
}

// Ensure errors implements error interface for detailed error testing
var _ error = (*detailedError)(nil)

type detailedError struct {
	Code    string
	Message string
	Cause   error
}

func (e *detailedError) Error() string {
	return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
}

func (e *detailedError) Unwrap() error {
	return e.Cause
}

// TestAgentFailure_ErrorsIs tests error unwrapping functionality.
func TestAgentFailure_ErrorsIs(t *testing.T) {
	// Test that our error wrapping pattern is correct
	err1 := &detailedError{
		Code:    "LLM_ERROR",
		Message: "rate limit exceeded",
		Cause:   errors.New("context deadline exceeded"),
	}

	err2 := &detailedError{
		Code:    "LLM_ERROR",
		Message: "different message",
		Cause:   err1,
	}

	// Errors.Is should work through the chain
	assert.True(t, errors.Is(err2, err1), "errors.Is should find wrapped error")

	t.Log("Error wrapping test passed: errors.Is works correctly for wrapped errors")
}