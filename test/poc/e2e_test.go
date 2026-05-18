// Package poc provides proof-of-concept limit tests.
package poc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_SubmitToResult tests end-to-end flow from submit to result.
func TestE2E_SubmitToResult(t *testing.T) {
	handler := &e2eHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	client := server.Client()

	// 1. Submit task
	resp, err := client.Post(
		server.URL+"/api/v1/tasks",
		"application/json",
		stringsReader(`{"goal":"Fix bug","priority":5}`),
	)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	var submitResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&submitResp))
	resp.Body.Close()

	data, ok := submitResp["data"].(map[string]any)
	require.True(t, ok)
	taskID := data["taskId"].(string)
	t.Logf("Submitted task: %s", taskID)

	// 2. Get status
	resp, err = client.Get(server.URL + "/api/v1/tasks/" + taskID)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	var statusResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&statusResp))
	resp.Body.Close()

	taskData := statusResp["data"].(map[string]any)
	t.Logf("Task status: %s", taskData["status"])
}

// TestE2E_MultipleUsersConcurrent tests multiple users submitting concurrently.
func TestE2E_MultipleUsersConcurrent(t *testing.T) {
	handler := &e2eHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	var wg sync.WaitGroup
	successCount := int64(0)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := http.Post(
				server.URL+"/api/v1/tasks",
				"application/json",
				stringsReader(fmt.Sprintf(`{"goal":"User %d task","priority":5}`, idx)),
			)
			if err == nil && resp.StatusCode == 200 {
				atomic.AddInt64(&successCount, 1)
				resp.Body.Close()
			}
		}(i)
	}

	wg.Wait()
	t.Logf("10 users concurrent: success=%d", successCount)
	assert.Equal(t, int64(10), successCount)
}

// TestE2E_CancelDuringExecution tests cancelling a task during execution.
func TestE2E_CancelDuringExecution(t *testing.T) {
	handler := &e2eHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	// Submit
	resp, err := http.Post(
		server.URL+"/api/v1/tasks",
		"application/json",
		stringsReader(`{"goal":"Long task","priority":5}`),
	)
	require.NoError(t, err)
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	data := result["data"].(map[string]any)
	taskID := data["taskId"].(string)

	// Cancel
	req, _ := http.NewRequest("POST",
		server.URL+"/api/v1/tasks/"+taskID+"/cancel",
		stringsReader(`{"reason":"too slow"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()

	t.Logf("Cancel during execution: task=%s", taskID)
}

// TestE2E_FullWorkflow tests complete workflow: submit→status→list→cancel.
func TestE2E_FullWorkflow(t *testing.T) {
	handler := &e2eHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	// Submit
	resp, err := http.Post(
		server.URL+"/api/v1/tasks",
		"application/json",
		stringsReader(`{"goal":"Workflow test","priority":3}`),
	)
	require.NoError(t, err)
	var submit map[string]any
	json.NewDecoder(resp.Body).Decode(&submit)
	resp.Body.Close()
	taskID := submit["data"].(map[string]any)["taskId"].(string)

	// Status
	resp, err = http.Get(server.URL + "/api/v1/tasks/" + taskID)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	// List
	resp, err = http.Get(server.URL + "/api/v1/tasks")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	// Cancel
	req, _ := http.NewRequest("POST",
		server.URL+"/api/v1/tasks/"+taskID+"/cancel",
		stringsReader(`{"reason":"done"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	t.Log("Full workflow: submit→status→list→cancel all passed")
}

// TestE2E_ErrorRecovery tests system recovery after errors.
func TestE2E_ErrorRecovery(t *testing.T) {
	handler := &e2eHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	// Invalid request
	resp, _ := http.Post(
		server.URL+"/api/v1/tasks",
		"application/json",
		stringsReader(`not json`),
	)
	if resp != nil {
		resp.Body.Close()
	}

	// Valid request should still work
	resp, err := http.Post(
		server.URL+"/api/v1/tasks",
		"application/json",
		stringsReader(`{"goal":"After error","priority":5}`),
	)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()

	t.Log("Error recovery: system recovered after invalid request")
}

// e2eHandler is a mock handler for E2E tests.
type e2eHandler struct {
	mu      sync.Mutex
	counter int
	tasks   map[string]string
}

func (h *e2eHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == "POST" && r.URL.Path == "/api/v1/tasks":
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"BAD_REQUEST"}}`))
			return
		}
		h.mu.Lock()
		h.counter++
		id := fmt.Sprintf("e2e_%d", h.counter)
		if h.tasks == nil {
			h.tasks = make(map[string]string)
		}
		h.tasks[id] = "pending"
		h.mu.Unlock()

		_, _ = w.Write([]byte(fmt.Sprintf(
			`{"ok":true,"data":{"taskId":"%s","status":"pending"}}`, id)))

	case r.Method == "GET" && r.URL.Path == "/api/v1/tasks":
		_, _ = w.Write([]byte(`{"ok":true,"data":[]}`))

	case r.Method == "GET" && len(r.URL.Path) > len("/api/v1/tasks/"):
		id := r.URL.Path[len("/api/v1/tasks/"):]
		_, _ = w.Write([]byte(fmt.Sprintf(
			`{"ok":true,"data":{"taskId":"%s","status":"completed"}}`, id)))

	case r.Method == "POST" && len(r.URL.Path) > 7 && r.URL.Path[len(r.URL.Path)-7:] == "/cancel":
		_, _ = w.Write([]byte(`{"ok":true,"data":{"status":"cancelled"}}`))
	}
}

func stringsReader(s string) *stringsReaderType {
	return &stringsReaderType{s: s}
}

type stringsReaderType struct {
	s   string
	pos int
}

func (r *stringsReaderType) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.s) {
		return 0, io.EOF
	}
	n = copy(p, r.s[r.pos:])
	r.pos += n
	return
}

// Ensure context import is used
var _ = context.Background
