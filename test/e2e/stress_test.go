// Package e2e provides end-to-end tests for the Cloud Agent Platform.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// stressTestHandler is a concurrent-safe mock handler for stress tests.
type stressTestHandler struct {
	mu        sync.Mutex
	idCounter int64
	tasks     map[string]*mcp.TaskStatusResponse
}

func newStressTestHandler() *stressTestHandler {
	return &stressTestHandler{
		tasks: make(map[string]*mcp.TaskStatusResponse),
	}
}

func (h *stressTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
		var req mcp.TaskSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "BAD_REQUEST", Message: "invalid JSON"}})
			return
		}
		id := fmt.Sprintf("task_stress_%d", atomic.AddInt64(&h.idCounter, 1))
		task := &mcp.TaskStatusResponse{
			TaskID:   id,
			Status:   "completed",
			Goal:     req.Goal,
			Priority: req.Priority,
			Result:   &mcp.TaskResultResponse{Summary: "done"},
		}
		h.mu.Lock()
		h.tasks[id] = task
		h.mu.Unlock()

		_ = json.NewEncoder(w).Encode(mcp.APIResponse{
			OK:   true,
			Data: marshalData(mcp.TaskSubmitResponse{TaskID: id, Status: "completed"}),
		})
		return

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks":
		h.mu.Lock()
		var list []*mcp.TaskStatusResponse
		for _, t := range h.tasks {
			list = append(list, t)
		}
		h.mu.Unlock()
		_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: true, Data: marshalData(list)})
		return

	case r.Method == http.MethodGet && len(r.URL.Path) > len("/api/v1/tasks/"):
		id := r.URL.Path[len("/api/v1/tasks/"):]
		h.mu.Lock()
		task, ok := h.tasks[id]
		h.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "task not found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: true, Data: marshalData(task)})
		return

	case r.Method == http.MethodPost && len(r.URL.Path) > len("/api/v1/tasks/") && r.URL.Path[len(r.URL.Path)-7:] == "/cancel":
		id := r.URL.Path[len("/api/v1/tasks/") : len(r.URL.Path)-7]
		h.mu.Lock()
		task, ok := h.tasks[id]
		if ok {
			task.Status = "cancelled"
		}
		h.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "task not found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(mcp.APIResponse{
			OK:   true,
			Data: marshalData(mcp.CancelTaskResponse{TaskID: id, Status: "cancelled"}),
		})
		return
	}

	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "endpoint not found"}})
}

func TestStress_10AgentsConcurrent(t *testing.T) {
	logger := zap.NewNop()
	handler := newStressTestHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	const numAgents = 10
	var wg sync.WaitGroup
	results := make(chan string, numAgents)

	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := client.SubmitTask(ctx, mcp.TaskSubmitRequest{
				Goal:     fmt.Sprintf("Agent %d task", idx),
				Priority: 5,
			})
			if err != nil {
				t.Errorf("Agent %d failed: %v", idx, err)
				return
			}
			results <- resp.TaskID
		}(i)
	}

	wg.Wait()
	close(results)

	var taskIDs []string
	for id := range results {
		taskIDs = append(taskIDs, id)
	}
	assert.Len(t, taskIDs, numAgents, "All 10 agents should succeed")

	// Verify each task
	for _, id := range taskIDs {
		status, err := client.GetTask(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, "completed", status.Status)
	}

	t.Logf("10 agents concurrent: %d tasks verified", len(taskIDs))
}

func TestStress_RapidSubmission(t *testing.T) {
	logger := zap.NewNop()
	handler := newStressTestHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	const numTasks = 50
	taskIDs := make([]string, 0, numTasks)

	for i := 0; i < numTasks; i++ {
		resp, err := client.SubmitTask(ctx, mcp.TaskSubmitRequest{
			Goal:     fmt.Sprintf("Rapid task %d", i),
			Priority: i % 10,
		})
		require.NoError(t, err, "Task %d should submit", i)
		taskIDs = append(taskIDs, resp.TaskID)
	}

	assert.Len(t, taskIDs, numTasks)

	// Sample verify
	for _, id := range taskIDs[:5] {
		status, err := client.GetTask(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, "completed", status.Status)
	}

	t.Logf("Rapid submission: %d tasks submitted", numTasks)
}

func TestStress_MixedOperations(t *testing.T) {
	logger := zap.NewNop()
	handler := newStressTestHandler()
	// Pre-populate tasks for status/cancel operations
	preIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("task_stress_pre_%d", i)
		handler.mu.Lock()
		handler.tasks[id] = &mcp.TaskStatusResponse{
			TaskID:   id,
			Status:   "pending",
			Goal:     fmt.Sprintf("Pre-existing task %d", i),
			Priority: 5,
		}
		handler.mu.Unlock()
		preIDs[i] = id
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	client := mcp.NewPlatformClient(server.URL, "test-token", logger)
	ctx := context.Background()

	var wg sync.WaitGroup
	var successCount int64

	// 10 concurrent submits
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := client.SubmitTask(ctx, mcp.TaskSubmitRequest{
				Goal:     fmt.Sprintf("Mixed submit %d", idx),
				Priority: 5,
			})
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	// 10 concurrent status checks on pre-existing tasks
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := client.GetTask(ctx, preIDs[idx%5])
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	// 5 concurrent cancels on pre-existing tasks
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := client.CancelTask(ctx, preIDs[idx], "stress test")
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 25 total ops: 10 submit + 10 status + 5 cancel
	// All should succeed (no panics, no deadlocks)
	assert.Equal(t, int64(25), successCount, "All 25 operations should succeed")

	t.Logf("Mixed operations: 25/25 succeeded")
}
