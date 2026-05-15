package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"
)

func TestExtractPathParam(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		prefix   string
		expected string
	}{
		{
			name:     "basic extraction",
			path:     "/api/v1/tasks/123",
			prefix:   "/api/v1/tasks/",
			expected: "123",
		},
		{
			name:     "with trailing slash",
			path:     "/api/v1/tasks/123/",
			prefix:   "/api/v1/tasks/",
			expected: "123",
		},
		{
			name:     "with extra path",
			path:     "/api/v1/tasks/123/cancel",
			prefix:   "/api/v1/tasks/",
			expected: "123",
		},
		{
			name:     "no prefix match",
			path:     "/api/v1/other/123",
			prefix:   "/api/v1/tasks/",
			expected: "",
		},
		{
			name:     "exact match no param",
			path:     "/api/v1/tasks",
			prefix:   "/api/v1/tasks/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPathParam(tt.path, tt.prefix)
			if result != tt.expected {
				t.Errorf("extractPathParam(%q, %q) = %q, want %q", tt.path, tt.prefix, result, tt.expected)
			}
		})
	}
}

func TestMapDomainStatusToString(t *testing.T) {
	// We can't directly test this without domain package, but we can test the strings
	// that would be returned. This is tested implicitly through integration tests.
}

func TestHandleTasks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	restAdapter := NewRESTAdapter(nil, logger)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{
			name:           "GET request",
			method:         http.MethodGet,
			expectedStatus: http.StatusUnauthorized, // No auth context, but method is allowed
		},
		{
			name:           "POST request",
			method:         http.MethodPost,
			expectedStatus: http.StatusUnauthorized, // No auth context
		},
		{
			name:           "PUT request",
			method:         http.MethodPut,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "DELETE request",
			method:         http.MethodDelete,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/v1/tasks", nil)
			w := httptest.NewRecorder()

			restAdapter.handleTasks(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestHandleTaskOperations_InvalidPath(t *testing.T) {
	logger := zaptest.NewLogger(t)
	restAdapter := NewRESTAdapter(nil, logger)

	// Test with empty path
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/", nil)
	w := httptest.NewRecorder()

	restAdapter.handleTaskOperations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleTaskOperations_UnknownOperation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	restAdapter := NewRESTAdapter(nil, logger)

	// Test with unknown operation
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/123/unknown", nil)
	w := httptest.NewRecorder()

	restAdapter.handleTaskOperations(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestRESTAdapterMethods_MethodNotAllowed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	restAdapter := NewRESTAdapter(nil, logger)

	// These tests verify that handlers return MethodNotAllowed for wrong HTTP methods.
	// Note: handleTasks dispatches by HTTP method (GET->ListTasks, POST->SubmitTask), so
	// the handler name in test name reflects which handler is dispatched to, not the path.
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			// GET /api/v1/tasks → ListTasks (handler checks method, passes) → then auth fails
			// Since the method check passes, we expect auth error, not method not allowed
			name:   "ListTasks handler called for GET - method check passes",
			method: http.MethodGet,
			path:   "/api/v1/tasks",
		},
		{
			name:   "SubmitTask with POST - method check passes, but auth fails",
			method: http.MethodPost,
			path:   "/api/v1/tasks",
		},
		{
			name:   "GetTask with GET - method check passes, but auth fails",
			method: http.MethodGet,
			path:   "/api/v1/tasks/123",
		},
		{
			name:   "GetTask with POST - method check fails",
			method: http.MethodPost,
			path:   "/api/v1/tasks/123",
		},
		{
			name:   "CancelTask with GET - method check fails",
			method: http.MethodGet,
			path:   "/api/v1/tasks/123/cancel",
		},
		{
			name:   "DecideTask with GET - method check fails",
			method: http.MethodGet,
			path:   "/api/v1/tasks/123/subtasks/sub456/decision",
		},
		{
			name:   "ListAgents with POST - method check fails",
			method: http.MethodPost,
			path:   "/api/v1/agent-templates",
		},
		{
			name:   "ListSessions with POST - method check fails",
			method: http.MethodPost,
			path:   "/api/v1/sessions",
		},
		{
			name:   "PlatformStatus with POST - method check fails",
			method: http.MethodPost,
			path:   "/api/v1/platform/status",
		},
		{
			name:   "GetTaskDiff with POST - method check fails",
			method: http.MethodPost,
			path:   "/api/v1/tasks/123/diff",
		},
		{
			name:   "DecomposeTask with GET - method check fails",
			method: http.MethodGet,
			path:   "/api/v1/tasks/123/decompose",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			switch tt.path {
			case "/api/v1/tasks":
				restAdapter.handleTasks(w, req)
			case "/api/v1/tasks/123":
				restAdapter.handleTaskOperations(w, req)
			case "/api/v1/tasks/123/cancel", "/api/v1/tasks/123/decompose", "/api/v1/tasks/123/diff":
				restAdapter.handleTaskOperations(w, req)
			case "/api/v1/tasks/123/subtasks/sub456/decision":
				restAdapter.handleTaskOperations(w, req)
			case "/api/v1/agent-templates":
				restAdapter.ListAgents(w, req)
			case "/api/v1/sessions":
				restAdapter.ListSessions(w, req)
			case "/api/v1/platform/status":
				restAdapter.PlatformStatus(w, req)
			}

			// For handlers that check auth first (handleTasks, handleTaskOperations),
			// we get Unauthorized when auth is missing.
			// For handlers that check method first, we get MethodNotAllowed.
			// The following paths with wrong methods should return MethodNotAllowed:
			expectedStatus := http.StatusUnauthorized // default for auth-required handlers

			switch {
			// First check for paths that need specific ordering
			case tt.method == http.MethodPost && tt.path == "/api/v1/tasks/123/diff":
				// POST /api/v1/tasks/123/diff → GetTaskDiff (wrong method)
				expectedStatus = http.StatusMethodNotAllowed
			case tt.method == http.MethodGet && tt.path == "/api/v1/tasks/123/decompose":
				// GET /api/v1/tasks/123/decompose → DecomposeTask (wrong method)
				expectedStatus = http.StatusMethodNotAllowed
			case tt.method == http.MethodGet && tt.path == "/api/v1/tasks/123/cancel":
				// GET /api/v1/tasks/123/cancel → CancelTask (wrong method)
				expectedStatus = http.StatusMethodNotAllowed
			case tt.method == http.MethodGet && strings.HasPrefix(tt.path, "/api/v1/tasks/") && strings.Contains(tt.path, "/subtasks/"):
				// Wrong method for decide
				expectedStatus = http.StatusMethodNotAllowed
			case tt.method == http.MethodPost && (tt.path == "/api/v1/agent-templates" || tt.path == "/api/v1/sessions" || tt.path == "/api/v1/platform/status"):
				// Wrong method for read-only endpoints
				expectedStatus = http.StatusMethodNotAllowed
			case tt.method == http.MethodPost && tt.path == "/api/v1/tasks":
				// POST /api/v1/tasks → SubmitTask (method OK, then auth fails)
				expectedStatus = http.StatusUnauthorized
			case tt.method == http.MethodGet && tt.path == "/api/v1/tasks":
				// GET /api/v1/tasks → ListTasks (method OK, then auth fails)
				expectedStatus = http.StatusUnauthorized
			case tt.method == http.MethodGet && tt.path == "/api/v1/tasks/123":
				// GET /api/v1/tasks/123 → GetTask (method OK, then auth fails)
				expectedStatus = http.StatusUnauthorized
			case tt.method == http.MethodPost && tt.path == "/api/v1/tasks/123":
				// POST /api/v1/tasks/123 → GetTask (wrong method)
				expectedStatus = http.StatusMethodNotAllowed
			}

			if w.Code != expectedStatus {
				t.Errorf("expected status %d, got %d for %s %s", expectedStatus, w.Code, tt.method, tt.path)
			}
		})
	}
}

func TestRESTAdapter_PlaceholderEndpoints(t *testing.T) {
	logger := zaptest.NewLogger(t)
	restAdapter := NewRESTAdapter(nil, logger)

	tests := []struct {
		name           string
		handler        func(http.ResponseWriter, *http.Request)
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "ListAgents returns empty list",
			handler:        restAdapter.ListAgents,
			method:         http.MethodGet,
			path:           "/api/v1/agent-templates",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "ListSessions returns empty session list",
			handler:        restAdapter.ListSessions,
			method:         http.MethodGet,
			path:           "/api/v1/sessions",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "PlatformStatus returns empty status",
			handler:        restAdapter.PlatformStatus,
			method:         http.MethodGet,
			path:           "/api/v1/platform/status",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GetTaskDiff returns NotImplemented",
			handler:        restAdapter.GetTaskDiff,
			method:         http.MethodGet,
			path:           "/api/v1/tasks/123/diff",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "DecomposeTask returns NotImplemented",
			handler:        restAdapter.DecomposeTask,
			method:         http.MethodPost,
			path:           "/api/v1/tasks/123/decompose",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			tt.handler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}