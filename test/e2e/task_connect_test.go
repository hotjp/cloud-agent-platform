// Package e2e provides end-to-end tests for the Cloud Agent Platform.
// These tests verify the task lifecycle via connect-go client.
package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/cloud-agent-platform/cap/api/cap/v1"
	"github.com/cloud-agent-platform/cap/api/cap/v1/capv1connect"
	"github.com/cloud-agent-platform/cap/internal/gateway"
	"github.com/cloud-agent-platform/cap/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// connectTestEnv holds the test environment for connect-go E2E tests.
type connectTestEnv struct {
	t           *testing.T
	logger      *zap.Logger
	server      *httptest.Server
	client      capv1connect.TaskServiceClient
	taskSvc     *service.TaskService
	jwtSecret   string
	clientID    string
	cleanupFn   func()
}

// setupConnectTestEnv creates a test environment with httptest server and connect-go client.
func setupConnectTestEnv(t *testing.T) *connectTestEnv {
	logger := zaptest.NewLogger(t)
	jwtSecret := "test-secret-key-for-e2e"
	clientID := "test-client-e2e"

	// Use the same test environment setup as smoke tests
	smokeEnv := setupTestEnv(t)

	// Create the gateway handler
	taskHandler := gateway.NewTaskServiceHandler(smokeEnv.taskSvc, logger)

	// Create connect-go handler (without auth interceptor for testing)
	path, connectHandler := capv1connect.NewTaskServiceHandler(taskHandler)

	// Create HTTP mux and register connect handler
	mux := http.NewServeMux()
	mux.Handle(path, connectHandler)

	// Health check handler
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Apply auth middleware to inject user context from JWT
	authMiddleware := gateway.Auth(gateway.AuthConfig{JWTSecret: jwtSecret})

	// Create httptest server with auth middleware
	server := httptest.NewServer(authMiddleware(mux))

	// Create connect-go client
	client := capv1connect.NewTaskServiceClient(http.DefaultClient, server.URL)

	return &connectTestEnv{
		t:         t,
		logger:    logger,
		server:    server,
		client:    client,
		taskSvc:   smokeEnv.taskSvc,
		jwtSecret: jwtSecret,
		clientID:  clientID,
		cleanupFn: func() {
			server.Close()
			smokeEnv.cleanupFn()
		},
	}
}

// generateTestJWT generates a valid JWT token for testing.
func (e *connectTestEnv) generateTestJWT(subject, clientID, role string) string {
	claims := jwt.MapClaims{
		"sub":       subject,
		"client_id": clientID,
		"role":      role,
		"exp":       time.Now().Add(time.Hour).Unix(),
		"iat":       time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(e.jwtSecret))
	require.NoError(e.t, err, "failed to generate test JWT")
	return tokenString
}

// newAuthRequest creates a connect.Request with JWT auth header.
func newAuthRequest[T any](msg *T, jwtToken string) *connect.Request[T] {
	req := connect.NewRequest(msg)
	req.Header().Set("Authorization", "Bearer "+jwtToken)
	return req
}

// TestSmokeE2E_ConnectSubmitGetList tests the full Submit+Get+List flow via connect-go client.
func TestSmokeE2E_ConnectSubmitGetList(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	env := setupConnectTestEnv(t)
	defer env.cleanupFn()

	t.Run("ConnectSubmitGetList", func(t *testing.T) {
		testConnectSubmitGetList(ctx, t, env)
	})
}

func testConnectSubmitGetList(ctx context.Context, t *testing.T, env *connectTestEnv) {
	// Generate JWT token for authentication
	jwtToken := env.generateTestJWT("test-user-001", env.clientID, "admin")

	// Step 1: Submit a new task via connect-go client
	t.Log("Step 1: Submitting task via connect-go client...")

	submitReq := &v1.SubmitTaskRequest{
		Goal: "Implement user authentication feature",
		Repository: &v1.Repository{
			Url:    "https://github.com/example/test-repo",
			Branch: "main",
		},
		Constraints: []string{
			"Must use JWT for authentication",
			"Must support OAuth2 providers",
		},
		VerificationCriteria: []string{
			"Users can sign up with email",
			"Users can login with email/password",
		},
		Priority: 5,
		Tags:     []string{"auth", "security"},
	}

	// Create connect request with auth header
	req := connect.NewRequest(submitReq)
	req.Header().Set("Authorization", "Bearer "+jwtToken)

	submitResp, err := env.client.Submit(ctx, req)
	require.NoError(t, err, "Failed to submit task via connect-go")
	require.NotNil(t, submitResp, "Submit response should not be nil")
	require.NotEmpty(t, submitResp.Msg.TaskId, "Task ID should not be empty")

	taskID := submitResp.Msg.TaskId
	t.Logf("Task submitted successfully via connect-go: %s", taskID)

	// Verify the response has expected fields
	assert.Equal(t, v1.TaskStatus_TASK_STATUS_PENDING, submitResp.Msg.Status)
	assert.NotEmpty(t, submitResp.Msg.ResultBranch)

	// Step 2: Get the task by ID via connect-go client
	t.Log("Step 2: Getting task via connect-go client...")

	getReq := &v1.GetTaskRequest{
		TaskId: taskID,
	}
	getResp, err := env.client.Get(ctx, newAuthRequest(getReq, jwtToken))
	require.NoError(t, err, "Failed to get task via connect-go")
	require.NotNil(t, getResp, "Get response should not be nil")
	require.NotNil(t, getResp.Msg.Task, "Task should not be nil")

	// Verify task fields match what we submitted
	actualTask := getResp.Msg.Task
	assert.Equal(t, taskID, actualTask.TaskId)
	assert.Equal(t, "Implement user authentication feature", actualTask.Goal)
	assert.Equal(t, v1.TaskStatus_TASK_STATUS_PENDING, actualTask.Status)
	assert.Equal(t, int32(5), actualTask.Priority)
	assert.NotEmpty(t, actualTask.CreatedAt)

	t.Logf("Task retrieved via connect-go: ID=%s, Status=%s, Goal=%s",
		actualTask.TaskId, actualTask.Status, actualTask.Goal)

	// Step 3: List tasks via connect-go client and verify our task is included
	t.Log("Step 3: Listing tasks via connect-go client...")

	listReq := &v1.ListTasksRequest{
		Page:     1,
		PageSize: 10,
	}
	listResp, err := env.client.List(ctx, newAuthRequest(listReq, jwtToken))
	require.NoError(t, err, "Failed to list tasks via connect-go")
	require.NotNil(t, listResp, "List response should not be nil")

	// Verify the list contains our submitted task
	found := false
	for _, task := range listResp.Msg.Tasks {
		if task.TaskId == taskID {
			found = true
			assert.Equal(t, "Implement user authentication feature", task.Goal)
			assert.Equal(t, v1.TaskStatus_TASK_STATUS_PENDING, task.Status)
			t.Logf("Found our task in list: ID=%s, Status=%s", task.TaskId, task.Status)
			break
		}
	}
	assert.True(t, found, "Submitted task should be present in list results")
	assert.GreaterOrEqual(t, listResp.Msg.Total, int32(1), "Total count should be at least 1")

	// Step 4: List tasks with status filter and verify our task is in pending list
	t.Log("Step 4: Listing tasks with status filter...")

	listPendingReq := &v1.ListTasksRequest{
		Status:   v1.TaskStatus_TASK_STATUS_PENDING,
		Page:     1,
		PageSize: 10,
	}
	listPendingResp, err := env.client.List(ctx, newAuthRequest(listPendingReq, jwtToken))
	require.NoError(t, err, "Failed to list pending tasks via connect-go")

	found = false
	for _, task := range listPendingResp.Msg.Tasks {
		if task.TaskId == taskID {
			found = true
			assert.Equal(t, v1.TaskStatus_TASK_STATUS_PENDING, task.Status)
			break
		}
	}
	assert.True(t, found, "Our pending task should be in the pending list")

	t.Log("E2E connect-go Submit+Get+List test completed successfully!")
}

// TestSmokeE2E_ConnectSubmitGetList_MultipleClients tests Submit+Get+List with multiple clients.
func TestSmokeE2E_ConnectSubmitGetList_MultipleClients(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	env := setupConnectTestEnv(t)
	defer env.cleanupFn()

	t.Run("MultipleClients", func(t *testing.T) {
		testConnectSubmitGetListMultipleClients(ctx, t, env)
	})
}

func testConnectSubmitGetListMultipleClients(ctx context.Context, t *testing.T, env *connectTestEnv) {
	// Create clients with different client IDs
	clients := []struct {
		subject  string
		clientID string
		role     string
	}{
		{"user-1", "client-1", "user"},
		{"user-2", "client-2", "user"},
		{"admin-1", "client-admin", "admin"},
	}

	// Submit tasks for each client
	submittedTaskIDs := make(map[string]string)
	for _, c := range clients {
		jwtToken := env.generateTestJWT(c.subject, c.clientID, c.role)

		submitReq := &v1.SubmitTaskRequest{
			Goal: "Task for " + c.clientID,
			Repository: &v1.Repository{
				Url:    "https://github.com/example/repo",
				Branch: "main",
			},
		}

		req := connect.NewRequest(submitReq)
		req.Header().Set("Authorization", "Bearer "+jwtToken)

		submitResp, err := env.client.Submit(ctx, req)
		require.NoError(t, err, "Failed to submit task for %s", c.clientID)
		submittedTaskIDs[c.clientID] = submitResp.Msg.TaskId
		t.Logf("Submitted task for %s: %s", c.clientID, submitResp.Msg.TaskId)
	}

	// List tasks and verify each client's task appears
	for _, c := range clients {
		jwtToken := env.generateTestJWT(c.subject, c.clientID, c.role)

		listReq := &v1.ListTasksRequest{
			Page:     1,
			PageSize: 100,
		}
		listResp, err := env.client.List(ctx, newAuthRequest(listReq, jwtToken))
		require.NoError(t, err, "Failed to list tasks for %s", c.clientID)

		// Verify the task for this client is in the list
		taskID := submittedTaskIDs[c.clientID]
		found := false
		for _, task := range listResp.Msg.Tasks {
			if task.TaskId == taskID {
				found = true
				break
			}
		}
		assert.True(t, found, "Task for %s should be in list", c.clientID)

		// Verify Get returns the correct task
		getReq := &v1.GetTaskRequest{TaskId: taskID}
		getResp, err := env.client.Get(ctx, newAuthRequest(getReq, jwtToken))
		require.NoError(t, err, "Failed to get task for %s", c.clientID)
		assert.Equal(t, taskID, getResp.Msg.Task.TaskId)
		assert.Equal(t, "Task for "+c.clientID, getResp.Msg.Task.Goal)
	}

	t.Log("E2E connect-go multiple clients test completed successfully!")
}

// TestSmokeE2E_ConnectAuthRequired tests that connect-go endpoints require authentication.
func TestSmokeE2E_ConnectAuthRequired(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupConnectTestEnv(t)
	defer env.cleanupFn()

	t.Run("AuthRequired", func(t *testing.T) {
		testConnectAuthRequired(ctx, t, env)
	})
}

func testConnectAuthRequired(ctx context.Context, t *testing.T, env *connectTestEnv) {
	// Try to submit without auth header
	submitReq := &v1.SubmitTaskRequest{
		Goal: "Test task",
		Repository: &v1.Repository{
			Url:    "https://github.com/example/repo",
			Branch: "main",
		},
	}

	req := connect.NewRequest(submitReq)
	// No auth header set

	_, err := env.client.Submit(ctx, req)
	require.Error(t, err, "Should fail without authentication")

	connectErr, ok := err.(*connect.Error)
	require.True(t, ok, "Error should be connect.Error")
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code(), "Should return Unauthenticated error")

	t.Log("Auth required test passed - endpoints correctly reject unauthenticated requests")
}
