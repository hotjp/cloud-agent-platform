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

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// cancelTestHandler simulates the platform API for task cancellation tests.
type cancelTestHandler struct {
	mu        sync.Mutex
	idCounter int
	tasks     map[string]*mcp.TaskStatusResponse
}

func newCancelTestHandler() *cancelTestHandler {
	return &cancelTestHandler{
		tasks: make(map[string]*mcp.TaskStatusResponse),
	}
}

func (h *cancelTestHandler) createTask(goal string, status string) *mcp.TaskStatusResponse {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.idCounter++
	id := fmt.Sprintf("task_cancel_%d", h.idCounter)
	task := &mcp.TaskStatusResponse{
		TaskID:   id,
		Status:   status,
		Goal:     goal,
		Priority: 5,
	}
	h.tasks[id] = task
	return task
}

func marshalData(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (h *cancelTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
		var req mcp.TaskSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "BAD_REQUEST", Message: "invalid JSON"}})
			return
		}
		h.idCounter++
		id := fmt.Sprintf("task_cancel_%d", h.idCounter)
		task := &mcp.TaskStatusResponse{
			TaskID:   id,
			Status:   "pending",
			Goal:     req.Goal,
			Priority: req.Priority,
		}
		h.tasks[id] = task
		_ = json.NewEncoder(w).Encode(mcp.APIResponse{
			OK:   true,
			Data: marshalData(mcp.TaskSubmitResponse{TaskID: id, Status: "pending"}),
		})
		return

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

	case r.Method == http.MethodPost && len(r.URL.Path) > len("/api/v1/tasks/") && r.URL.Path[len(r.URL.Path)-7:] == "/cancel":
		id := r.URL.Path[len("/api/v1/tasks/"):len(r.URL.Path)-7]
		task, ok := h.tasks[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "task not found"}})
			return
		}
		if task.Status == "completed" || task.Status == "failed" {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "INVALID_STATE", Message: "cannot cancel " + task.Status + " task"}})
			return
		}
		task.Status = "cancelled"
		_ = json.NewEncoder(w).Encode(mcp.APIResponse{
			OK:   true,
			Data: marshalData(mcp.CancelTaskResponse{TaskID: id, Status: "cancelled"}),
		})
		return

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks":
		var taskList []*mcp.TaskStatusResponse
		for _, t := range h.tasks {
			taskList = append(taskList, t)
		}
		_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: true, Data: marshalData(taskList)})
		return
	}

	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "endpoint not found"}})
}

func TestTaskCancel_CancelPendingTask(t *testing.T) {
	logger := zap.NewNop()
	handler := newCancelTestHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	// Create a pending task
	task := handler.createTask("Test pending task", "pending")

	// Cancel it
	resp, err := client.CancelTask(ctx, task.TaskID, "no longer needed")
	require.NoError(t, err)
	assert.Equal(t, "cancelled", resp.Status)

	// Verify status
	status, err := client.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", status.Status)

	t.Log("Pending task cancelled successfully")
}

func TestTaskCancel_CancelRunningTask(t *testing.T) {
	logger := zap.NewNop()
	handler := newCancelTestHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	// Create a running task
	task := handler.createTask("Test running task", "running")

	// Cancel it
	resp, err := client.CancelTask(ctx, task.TaskID, "user requested cancellation")
	require.NoError(t, err)
	assert.Equal(t, "cancelled", resp.Status)

	// Verify status changed
	status, err := client.GetTask(ctx, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", status.Status)

	t.Log("Running task cancelled successfully")
}

func TestTaskCancel_CancelCompletedTask(t *testing.T) {
	logger := zap.NewNop()
	handler := newCancelTestHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	// Create a completed task
	task := handler.createTask("Test completed task", "completed")

	// Attempt to cancel — should fail
	_, err := client.CancelTask(ctx, task.TaskID, "try to cancel")
	assert.Error(t, err, "Cancelling completed task should return error")

	t.Log("Completed task correctly rejected for cancellation")
}

func TestTaskCancel_CancelNonExistentTask(t *testing.T) {
	logger := zap.NewNop()
	handler := newCancelTestHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	// Cancel non-existent task
	_, err := client.CancelTask(ctx, "task_nonexistent", "no such task")
	assert.Error(t, err, "Cancelling non-existent task should return error")

	t.Log("Non-existent task correctly returns error")
}
