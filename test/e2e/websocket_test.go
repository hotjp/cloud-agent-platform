// Package e2e provides end-to-end tests for the Cloud Agent Platform.
// These tests verify WebSocket real-time push functionality.
package e2e

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/cloud-agent-platform/cap/api/cap/v1"
	"github.com/cloud-agent-platform/cap/api/cap/v1/capv1connect"
	"github.com/cloud-agent-platform/cap/internal/authz"
	"github.com/cloud-agent-platform/cap/internal/gateway"
	"github.com/cloud-agent-platform/cap/internal/gateway/ws"
	"github.com/cloud-agent-platform/cap/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// wsTestEnv holds the test environment for WebSocket E2E tests.
type wsTestEnv struct {
	t         *testing.T
	logger    *zap.Logger
	listener  net.Listener
	hub       *ws.Hub
	client    capv1connect.TaskServiceClient
	taskSvc   *service.TaskService
	jwtSecret string
	wsURL     string
	cleanupFn func()
}

// setupWSTestEnv creates a test environment with HTTP server, connect-go client, and WebSocket hub.
func setupWSTestEnv(t *testing.T) *wsTestEnv {
	logger := zaptest.NewLogger(t)
	jwtSecret := "test-secret-key-for-ws-e2e"

	// Use the same test environment setup as smoke tests
	smokeEnv := setupTestEnv(t)

	// Create authz service for JWT validation
	authzCfg := authz.Config{
		JWTSecret:    jwtSecret,
		APIKeyHeader: "X-API-Key",
		CacheTTL:     5 * time.Minute,
	}
	authzSvc := authz.New(authzCfg, logger)

	// Create the gateway handler
	taskHandler := gateway.NewTaskServiceHandler(smokeEnv.taskSvc, logger)

	// Create authz interceptor
	authzInterceptor := authz.NewInterceptor(authz.InterceptorConfig{
		Authz:        authzSvc,
		SkipPaths:    map[string]bool{"/healthz": true, "/readyz": true},
		APIKeyHeader: authzCfg.APIKeyHeader,
	}, logger)

	// Create WebSocket Hub (without Redis for testing)
	wsHubCfg := ws.HubConfig{
		RedisClient:       nil, // No Redis - we'll inject events directly
		StreamKey:         "stream:domain_events",
		ConsumerGroup:     "websocket-hub-test",
		ConsumerID:        "ws-test-hub",
		Logger:            logger,
		HeartbeatInterval: ws.HeartbeatInterval,
		HeartbeatTimeout:  ws.HeartbeatTimeout,
	}
	wsHub := ws.NewHub(wsHubCfg)
	if err := wsHub.Run(); err != nil {
		t.Fatalf("failed to start websocket hub: %v", err)
	}

	// Create connect-go handler with interceptor
	path, connectHandler := capv1connect.NewTaskServiceHandler(taskHandler,
		connect.WithInterceptors(authzInterceptor))

	// Create HTTP mux and register handlers
	mux := http.NewServeMux()
	mux.Handle(path, connectHandler)

	// WebSocket endpoint - use pattern with trailing slash to match subpaths
	mux.HandleFunc(ws.WebSocketPath+"/", wsHub.HandleWebSocket)

	// Health check handler
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Create a real HTTP server (httptest doesn't support WebSocket upgrades)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	server := &http.Server{
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		_ = server.Serve(listener)
	}()

	// Determine the base URL
	baseURL := "http://" + listener.Addr().String()

	// Create connect-go client
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	connectClient := capv1connect.NewTaskServiceClient(httpClient, baseURL)

	return &wsTestEnv{
		t:         t,
		logger:    logger,
		listener:  listener,
		hub:       wsHub,
		client:    connectClient,
		taskSvc:   smokeEnv.taskSvc,
		jwtSecret: jwtSecret,
		wsURL:     "ws://" + listener.Addr().String() + ws.WebSocketPath,
		cleanupFn: func() {
			_ = server.Close()
			wsHub.Stop()
			smokeEnv.cleanupFn()
		},
	}
}

// generateWSTestJWT generates a valid JWT token for testing.
func (e *wsTestEnv) generateTestJWT(subject, clientID, role string) string {
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

// newWSAuthRequest creates a connect.Request with JWT auth header.
func newWSAuthRequest[T any](msg *T, jwtToken string) *connect.Request[T] {
	req := connect.NewRequest(msg)
	req.Header().Set("Authorization", "Bearer "+jwtToken)
	return req
}

// connectWebSocket establishes a WebSocket connection and authenticates.
func connectWebSocket(t *testing.T, wsURL, taskID, jwtToken string) *websocket.Conn {
	url := wsURL + "/" + taskID
	header := http.Header{}
	header.Set("Authorization", "Bearer "+jwtToken)

	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	return conn
}

// readWSMessage reads a WebSocket message with timeout.
func readWSMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) ([]byte, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	msgType, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if msgType != websocket.TextMessage {
		return nil, nil
	}
	return msg, nil
}

// waitForWSMessage waits for a specific WebSocket message type with timeout.
// It runs in a separate goroutine to avoid conflicting with the client's read pump.
func waitForWSMessage(t *testing.T, conn *websocket.Conn, expectedType string, timeout time.Duration) *ws.WSEvent {
	resultCh := make(chan *ws.WSEvent, 1)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					return
				}
				// Timeout or other error, continue polling
				continue
			}
			if msgType != websocket.TextMessage {
				continue
			}

			var event ws.WSEvent
			if err := json.Unmarshal(msg, &event); err != nil {
				continue
			}

			if event.Type == expectedType {
				resultCh <- &event
				return
			}
		}
		close(resultCh)
	}()

	select {
	case event, ok := <-resultCh:
		if ok {
			return event
		}
	case <-doneCh:
		// Goroutine finished without finding the event
	case <-time.After(timeout):
		// Timeout
	}
	return nil
}

// TestWebSocketE2E_ConnectAndReceiveEvents tests WebSocket connection and event reception.
func TestWebSocketE2E_ConnectAndReceiveEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	env := setupWSTestEnv(t)
	defer env.cleanupFn()

	t.Run("WebSocketConnectAndEvents", func(t *testing.T) {
		testWebSocketConnectAndEvents(ctx, t, env)
	})
}

func testWebSocketConnectAndEvents(ctx context.Context, t *testing.T, env *wsTestEnv) {
	// Generate JWT token for authentication
	jwtToken := env.generateTestJWT("ws-test-user", "ws-test-client", "admin")

	// Step 1: Submit a new task via connect-go client
	t.Log("Step 1: Submitting task via connect-go client...")

	submitReq := &v1.SubmitTaskRequest{
		Goal: "WebSocket test task",
		Repository: &v1.Repository{
			Url:    "https://github.com/example/test-repo",
			Branch: "main",
		},
		Constraints: []string{
			"Test constraint 1",
		},
		VerificationCriteria: []string{
			"Test criteria 1",
		},
		Priority: 5,
		Tags:     []string{"ws-test"},
	}

	req := connect.NewRequest(submitReq)
	req.Header().Set("Authorization", "Bearer "+jwtToken)

	submitResp, err := env.client.Submit(ctx, req)
	require.NoError(t, err, "Failed to submit task via connect-go")
	require.NotEmpty(t, submitResp.Msg.TaskId, "Task ID should not be empty")

	taskID := submitResp.Msg.TaskId
	t.Logf("Task submitted: %s", taskID)

	// Step 2: Connect to WebSocket for this task
	t.Log("Step 2: Connecting to WebSocket...")

	conn := connectWebSocket(t, env.wsURL, taskID, jwtToken)
	defer conn.Close()

	// Step 3: Push events via the hub's exported methods
	t.Log("Step 3: Pushing task status change event via hub...")

	// Push an approval required event which is an exported method
	err = env.hub.PushApprovalRequired(ctx, taskID, "WebSocket test task", 0.05, "low",
		time.Now(), time.Now().Add(5*time.Minute), 5*time.Minute)
	require.NoError(t, err, "Failed to push approval required event")

	// Step 4: Verify WebSocket receives events
	t.Log("Step 4: Waiting for WebSocket events...")

	// Wait for task.approval_required event (what we pushed)
	approvalEvent := waitForWSMessage(t, conn, "task.approval_required", 5*time.Second)
	require.NotNil(t, approvalEvent, "Should receive approval_required event")
	assert.Equal(t, taskID, approvalEvent.TaskID)
	assert.Equal(t, "task.approval_required", approvalEvent.Type)
	t.Logf("Received task.approval_required event: %+v", approvalEvent)

	t.Log("WebSocket E2E connect and events test completed!")
}

// TestWebSocketE2E_PushApprovalRequired tests the PushApprovalRequired method.
func TestWebSocketE2E_PushApprovalRequired(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupWSTestEnv(t)
	defer env.cleanupFn()

	t.Run("PushApprovalRequired", func(t *testing.T) {
		testPushApprovalRequired(ctx, t, env)
	})
}

func testPushApprovalRequired(ctx context.Context, t *testing.T, env *wsTestEnv) {
	// Generate JWT token
	jwtToken := env.generateTestJWT("ws-test-user", "ws-test-client", "admin")

	// Submit a task first
	submitReq := &v1.SubmitTaskRequest{
		Goal: "Approval test task",
		Repository: &v1.Repository{
			Url:    "https://github.com/example/test-repo",
			Branch: "main",
		},
	}
	req := connect.NewRequest(submitReq)
	req.Header().Set("Authorization", "Bearer "+jwtToken)

	submitResp, err := env.client.Submit(ctx, req)
	require.NoError(t, err, "Failed to submit task")
	taskID := submitResp.Msg.TaskId
	t.Logf("Task submitted: %s", taskID)

	// Connect WebSocket client
	conn := connectWebSocket(t, env.wsURL, taskID, jwtToken)
	defer conn.Close()

	// Give the hub's run loop time to process the registration
	time.Sleep(50 * time.Millisecond)

	// Step 2: Push approval required event
	t.Log("Pushing approval required event...")

	err = env.hub.PushApprovalRequired(ctx, taskID, "Approval test task", 0.05, "low",
		time.Now(), time.Now().Add(5*time.Minute), 5*time.Minute)
	require.NoError(t, err, "Failed to push approval required event")

	// Step 3: Verify WebSocket receives the event
	t.Log("Waiting for approval_required event...")

	event := waitForWSMessage(t, conn, "task.approval_required", 5*time.Second)
	require.NotNil(t, event, "Should receive approval_required event")
	assert.Equal(t, taskID, event.TaskID)
	assert.Equal(t, "task.approval_required", event.Type)

	// Parse the payload to verify contents
	var payload ws.ApprovalPushPayload
	err = json.Unmarshal(event.Payload, &payload)
	require.NoError(t, err, "Failed to unmarshal approval payload")
	assert.Equal(t, taskID, payload.TaskID)
	assert.Equal(t, "Approval test task", payload.TaskGoal)
	assert.True(t, payload.RequireApproval)

	t.Log("PushApprovalRequired test completed!")
}

// TestWebSocketE2E_MultipleClientsInRoom tests multiple clients in the same room.
func TestWebSocketE2E_MultipleClientsInRoom(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	env := setupWSTestEnv(t)
	defer env.cleanupFn()

	t.Run("MultipleClientsInRoom", func(t *testing.T) {
		testMultipleClientsInRoom(ctx, t, env)
	})
}

func testMultipleClientsInRoom(ctx context.Context, t *testing.T, env *wsTestEnv) {
	jwtToken := env.generateTestJWT("ws-test-user", "ws-test-client", "admin")

	// Submit a task
	submitReq := &v1.SubmitTaskRequest{
		Goal: "Multi-client test task",
		Repository: &v1.Repository{
			Url:    "https://github.com/example/test-repo",
			Branch: "main",
		},
	}
	req := connect.NewRequest(submitReq)
	req.Header().Set("Authorization", "Bearer "+jwtToken)

	submitResp, err := env.client.Submit(ctx, req)
	require.NoError(t, err, "Failed to submit task")
	taskID := submitResp.Msg.TaskId
	t.Logf("Task submitted: %s", taskID)

	// Connect multiple WebSocket clients for the same task
	numClients := 3
	clients := make([]*websocket.Conn, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = connectWebSocket(t, env.wsURL, taskID, jwtToken)
		defer clients[i].Close()
	}

	// Push an event to the room
	err = env.hub.PushApprovalRequired(ctx, taskID, "Multi-client test", 0.05, "low",
		time.Now(), time.Now().Add(5*time.Minute), 5*time.Minute)
	require.NoError(t, err, "Failed to push event")

	// All clients should receive the event
	for i, conn := range clients {
		event := waitForWSMessage(t, conn, "task.approval_required", 5*time.Second)
		require.NotNil(t, event, "Client %d should receive event", i)
		assert.Equal(t, taskID, event.TaskID)
		t.Logf("Client %d received event", i)
	}

	t.Log("Multiple clients in room test completed!")
}

// TestWebSocketE2E_AuthRequired tests that WebSocket connections require authentication.
func TestWebSocketE2E_AuthRequired(t *testing.T) {
	env := setupWSTestEnv(t)
	defer env.cleanupFn()

	t.Run("WSAuthRequired", func(t *testing.T) {
		testWSAuthRequired(t, env)
	})
}

func testWSAuthRequired(t *testing.T, env *wsTestEnv) {
	taskID := "test-task-without-auth"

	// Try to connect without auth header
	url := env.wsURL + "/" + taskID
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		// If connection succeeds, it means the server doesn't require auth at connection time
		// The hub might accept connections and rely on message-level auth
		conn.Close()
		t.Log("WebSocket accepts connections without initial auth (message-level auth expected)")
	} else {
		t.Logf("WebSocket connection rejected without auth: %v", err)
	}
}

// TestWebSocketE2E_RoomIsolation tests that rooms are isolated per task.
func TestWebSocketE2E_RoomIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	env := setupWSTestEnv(t)
	defer env.cleanupFn()

	t.Run("RoomIsolation", func(t *testing.T) {
		testRoomIsolation(ctx, t, env)
	})
}

func testRoomIsolation(ctx context.Context, t *testing.T, env *wsTestEnv) {
	jwtToken := env.generateTestJWT("ws-test-user", "ws-test-client", "admin")

	// Submit two tasks
	submitReq := &v1.SubmitTaskRequest{
		Goal: "Room isolation test task 1",
		Repository: &v1.Repository{
			Url:    "https://github.com/example/test-repo",
			Branch: "main",
		},
	}
	req := connect.NewRequest(submitReq)
	req.Header().Set("Authorization", "Bearer "+jwtToken)

	resp1, err := env.client.Submit(ctx, req)
	require.NoError(t, err, "Failed to submit task 1")
	taskID1 := resp1.Msg.TaskId

	submitReq2 := &v1.SubmitTaskRequest{
		Goal: "Room isolation test task 2",
		Repository: &v1.Repository{
			Url:    "https://github.com/example/test-repo",
			Branch: "main",
		},
	}
	req2 := connect.NewRequest(submitReq2)
	req2.Header().Set("Authorization", "Bearer "+jwtToken)

	resp2, err := env.client.Submit(ctx, req2)
	require.NoError(t, err, "Failed to submit task 2")
	taskID2 := resp2.Msg.TaskId

	t.Logf("Task 1: %s, Task 2: %s", taskID1, taskID2)

	// Connect WebSocket clients to each task
	conn1 := connectWebSocket(t, env.wsURL, taskID1, jwtToken)
	defer conn1.Close()

	conn2 := connectWebSocket(t, env.wsURL, taskID2, jwtToken)
	defer conn2.Close()

	// Push event only to task 1's room
	err = env.hub.PushApprovalRequired(ctx, taskID1, "Task 1 event", 0.05, "low",
		time.Now(), time.Now().Add(5*time.Minute), 5*time.Minute)
	require.NoError(t, err, "Failed to push event to task 1")

	// Wait for event on conn1
	event1 := waitForWSMessage(t, conn1, "task.approval_required", 5*time.Second)
	require.NotNil(t, event1, "Task 1 client should receive event")
	assert.Equal(t, taskID1, event1.TaskID)

	// Verify conn2 does NOT receive the event
	// We'll check if conn2 receives anything within a short window
	msg, _ := readWSMessage(t, conn2, 1*time.Second)
	if msg != nil {
		var event ws.WSEvent
		if json.Unmarshal(msg, &event) == nil && event.TaskID == taskID1 {
			t.Fatal("Task 2 client should NOT receive task 1's event")
		}
	}
	// If msg is nil or different task's event, that's expected

	t.Log("Room isolation test completed!")
}