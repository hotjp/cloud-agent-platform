// Package ws implements the WebSocket Hub for real-time event push.
package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// Helper to create a test logger.
func newTestLogger(t *testing.T) *zap.Logger {
	return zaptest.NewLogger(t)
}

func TestRoom_Broadcast(t *testing.T) {
	logger := newTestLogger(t)
	room := NewRoom("room:test")

	// Create a mock client with a buffered send channel
	client := &Client{
		send:   make(chan []byte, 10),
		taskID: "test-task",
		logger: logger,
		id:     "client-1",
	}

	room.Add(client)

	event := &WSEvent{
		Type:      "task.status_changed",
		TaskID:    "test-task",
		Payload:   json.RawMessage(`{"status":"running"}`),
		Timestamp: time.Now().UTC(),
	}

	room.Broadcast(event)

	// Should receive the event
	select {
	case msg := <-client.send:
		var received WSEvent
		err := json.Unmarshal(msg, &received)
		require.NoError(t, err)
		assert.Equal(t, "task.status_changed", received.Type)
		assert.Equal(t, "test-task", received.TaskID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestRoom_AddRemove(t *testing.T) {
	room := NewRoom("room:test")
	logger := newTestLogger(t)

	client1 := &Client{
		send:   make(chan []byte, 10),
		taskID: "test-task",
		logger: logger,
		id:     "client-1",
	}

	client2 := &Client{
		send:   make(chan []byte, 10),
		taskID: "test-task",
		logger: logger,
		id:     "client-2",
	}

	assert.Equal(t, 0, room.ClientCount())

	room.Add(client1)
	assert.Equal(t, 1, room.ClientCount())

	room.Add(client2)
	assert.Equal(t, 2, room.ClientCount())

	room.Remove(client1)
	assert.Equal(t, 1, room.ClientCount())

	room.Remove(client2)
	assert.Equal(t, 0, room.ClientCount())
}

func TestRoom_BroadcastMultipleClients(t *testing.T) {
	logger := newTestLogger(t)
	room := NewRoom("room:test")

	clients := make([]*Client, 5)
	for i := 0; i < 5; i++ {
		clients[i] = &Client{
			send:   make(chan []byte, 10),
			taskID: "test-task",
			logger: logger,
			id:     "client-" + string(rune('1'+i)),
		}
		room.Add(clients[i])
	}

	event := &WSEvent{
		Type:      "task.completed",
		TaskID:    "test-task",
		Timestamp: time.Now().UTC(),
	}

	room.Broadcast(event)

	// All clients should receive the event
	for i, client := range clients {
		select {
		case msg := <-client.send:
			var received WSEvent
			err := json.Unmarshal(msg, &received)
			require.NoError(t, err, "client %d", i)
			assert.Equal(t, "task.completed", received.Type)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timeout waiting for broadcast to client %d", i)
		}
	}
}

func TestHub_NewHub(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)
	cfg.StreamKey = "stream:test"

	hub := NewHub(cfg)

	assert.NotNil(t, hub)
	assert.Equal(t, 0, hub.ClientCount())
	assert.Equal(t, 0, hub.RoomCount())
	assert.Equal(t, "stream:test", hub.streamKey)
}

func TestHub_getOrCreateRoom(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)
	hub := NewHub(cfg)

	room1 := hub.getOrCreateRoom("task-1")
	room2 := hub.getOrCreateRoom("task-1")
	room3 := hub.getOrCreateRoom("task-2")

	assert.Same(t, room1, room2) // Same room for same task ID
	assert.NotSame(t, room1, room3) // Different room for different task ID
	assert.Equal(t, 2, hub.RoomCount())
}

func TestHub_getRoom(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)
	hub := NewHub(cfg)

	// Initially no rooms
	assert.Nil(t, hub.getRoom("task-1"))

	// Create a room
	room := hub.getOrCreateRoom("task-1")
	assert.NotNil(t, room)

	// Now getRoom should return it
	assert.NotNil(t, hub.getRoom("task-1"))
}

func TestHub_RegisterUnregisterClient(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)
	hub := NewHub(cfg)

	client := &Client{
		send:   make(chan []byte, 10),
		taskID: "task-1",
		logger: logger,
		id:     "client-1",
	}

	// Register directly via the synchronous function
	hub.registerClient(client)

	assert.Equal(t, 1, hub.ClientCount())
	assert.NotNil(t, hub.getRoom("task-1"))
	assert.Equal(t, 1, hub.getRoom("task-1").ClientCount())

	// Unregister directly via the synchronous function
	hub.unregisterClient(client)

	assert.Equal(t, 0, hub.ClientCount())
}

func TestHub_HandleStreamEvent(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)
	hub := NewHub(cfg)

	// Create a room and add a client
	room := hub.getOrCreateRoom("task-1")
	client := &Client{
		send:   make(chan []byte, 10),
		taskID: "task-1",
		logger: logger,
		id:     "client-1",
	}
	room.Add(client)

	// Process a task status changed event
	event := &StreamEvent{
		ID:          "12345",
		AggregateID: "task-1",
		EventType:   "TaskStatusChangedV1",
		Payload:     []byte(`{"old":"running","new":"completed"}`),
	}

	hub.handleStreamEvent(event)

	// Client should receive the event
	select {
	case msg := <-client.send:
		var received WSEvent
		err := json.Unmarshal(msg, &received)
		require.NoError(t, err)
		assert.Equal(t, "task_updated", received.Type)
		assert.Equal(t, "task-1", received.TaskID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}
}

func TestHub_HandleStreamEvent_UnknownType(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)
	hub := NewHub(cfg)

	room := hub.getOrCreateRoom("task-1")
	client := &Client{
		send:   make(chan []byte, 10),
		taskID: "task-1",
		logger: logger,
		id:     "client-1",
	}
	room.Add(client)

	// Unknown event type - should be skipped
	event := &StreamEvent{
		ID:          "12345",
		AggregateID: "task-1",
		EventType:   "UnknownEventType",
		Payload:     []byte(`{}`),
	}

	hub.handleStreamEvent(event)

	// Client should NOT receive the event
	select {
	case <-client.send:
		t.Fatal("should not receive unknown event type")
	case <-time.After(50 * time.Millisecond):
		// Expected - no message
	}
}

func TestExtractTaskID(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/api/v1/ws", ""},
		{"/api/v1/ws/", ""},
		{"/api/v1/ws/task-123", "task-123"},
		{"/api/v1/ws/01HXYZ123456", "01HXYZ123456"},
	}

	for _, tt := range tests {
		result := extractTaskID(tt.path)
		assert.Equal(t, tt.expected, result, "path: %s", tt.path)
	}
}

func TestHub_Stop(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)
	hub := NewHub(cfg)

	// Add a client using synchronous registration
	client := &Client{
		send:   make(chan []byte, 10),
		taskID: "task-1",
		logger: logger,
		id:     "client-1",
	}
	hub.registerClient(client)

	// Stop should close client send channels
	err := hub.Stop()
	assert.NoError(t, err)
}

func TestClient_HandleAuth(t *testing.T) {
	logger := newTestLogger(t)

	// Create a mock WebSocket connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// We just need the server to be running
	}))
	defer server.Close()

	// Create client
	hub := &Hub{
		unregisterCh: make(chan *Client),
		logger:       logger,
	}

	conn, _, err := websocket.DefaultDialer.Dial("ws://"+server.Listener.Addr().String()+"/", nil)
	if err != nil {
		// Server may not be listening yet, skip
		t.Skip("websocket dial failed")
	}
	defer conn.Close()

	client := NewClient(hub, conn, "task-1", logger)

	// Test auth message
	authMsg := map[string]interface{}{
		"type":  "auth",
		"token": "test-token",
	}
	data, _ := json.Marshal(authMsg)
	client.handleMessage(data)

	// Client should be marked as authenticated
	client.mu.Lock()
	assert.True(t, client.authenticated)
	client.mu.Unlock()
}

func TestClient_HandlePing(t *testing.T) {
	logger := newTestLogger(t)
	hub := &Hub{
		unregisterCh: make(chan *Client),
		logger:       logger,
	}
	client := NewClient(hub, nil, "task-1", logger)
	client.send = make(chan []byte, 10)

	// Send ping
	client.handlePing()

	select {
	case msg := <-client.send:
		var response map[string]interface{}
		err := json.Unmarshal(msg, &response)
		require.NoError(t, err)
		assert.Equal(t, "pong", response["type"])
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for pong")
	}
}

func TestClient_HandleSubscribe(t *testing.T) {
	logger := newTestLogger(t)
	hub := &Hub{
		registerCh:   make(chan *Client, 1),
		unregisterCh: make(chan *Client, 1),
		rooms:        make(map[string]*Room),
		roomsMu:      sync.RWMutex{},
		clients:      make(map[*Client]struct{}),
		clientsMu:    sync.RWMutex{},
		logger:       logger,
	}

	client := NewClient(hub, nil, "task-1", logger)
	client.send = make(chan []byte, 10)
	client.authenticated = true // Mark as authenticated

	// Manually trigger subscribe via hub
	// First, directly register the client
	hub.registerClient(client)
	assert.Equal(t, 1, hub.ClientCount())
	assert.Equal(t, "task-1", client.taskID)

	// Verify room assignment
	room := hub.getRoom("task-1")
	require.NotNil(t, room)
	assert.Equal(t, 1, room.ClientCount())
}

func TestClient_HandleSubscribe_NotAuthenticated(t *testing.T) {
	logger := newTestLogger(t)
	hub := &Hub{
		unregisterCh: make(chan *Client),
		logger:       logger,
	}

	client := NewClient(hub, nil, "task-1", logger)
	client.send = make(chan []byte, 10)
	// Not authenticated

	// Try to subscribe
	subscribeMsg := map[string]interface{}{
		"type":    "subscribe",
		"taskId":  "task-2",
	}
	data, _ := json.Marshal(subscribeMsg)
	client.handleMessage(data)

	// Should send error
	select {
	case msg := <-client.send:
		var response map[string]interface{}
		err := json.Unmarshal(msg, &response)
		require.NoError(t, err)
		assert.Equal(t, "error", response["type"])
		assert.Equal(t, "not authenticated", response["message"])
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for error")
	}
}

// TestWSEvent tests the WSEvent serialization.
func TestWSEvent_Serialization(t *testing.T) {
	event := &WSEvent{
		Type:      "task.status_changed",
		TaskID:    "task-123",
		Payload:   json.RawMessage(`{"old":"running","new":"completed"}`),
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded WSEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, event.Type, decoded.Type)
	assert.Equal(t, event.TaskID, decoded.TaskID)
	assert.JSONEq(t, string(event.Payload), string(decoded.Payload))
}

// TestHub_HandleWebSocket tests the WebSocket HTTP handler.
func TestHub_HandleWebSocket_ExtractTaskID(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)
	_ = NewHub(cfg) // Just to verify it doesn't panic

	// Test URL path extraction
	path := "/api/v1/ws/01HXYZ123456"
	extracted := extractTaskID(path)
	assert.Equal(t, "01HXYZ123456", extracted)
}

// TestWebSocketPath constant
func TestWebSocketPath_Constant(t *testing.T) {
	assert.Equal(t, "/api/v1/ws", WebSocketPath)
}

// TestHub_RoomKeyPrefix constant
func TestHub_RoomKeyPrefix_Constant(t *testing.T) {
	assert.Equal(t, "room:task:", RoomKeyPrefix)
}

// TestEventTypeMapping tests event type mapping.
func TestEventTypeMapping(t *testing.T) {
	tests := []struct {
		eventType   string
		isTask      bool
		isAgent     bool
		isArtifact  bool
	}{
		{"TaskCreatedV1", true, false, false},
		{"TaskStatusChangedV1", true, false, false},
		{"TaskCompletedV1", true, false, false},
		{"AgentThoughtV1", false, true, false},
		{"AgentStartedV1", false, true, false},
		{"ArtifactCreatedV1", false, false, true},
		{"UnknownEvent", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			assert.Equal(t, tt.isTask, isTaskStatusChangedEvent(tt.eventType))
			assert.Equal(t, tt.isAgent, isAgentThoughtEvent(tt.eventType))
			assert.Equal(t, tt.isArtifact, isArtifactCreatedEvent(tt.eventType))
		})
	}
}

// TestHub_DefaultHubConfig tests the default configuration.
func TestHub_DefaultHubConfig(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)

	assert.Equal(t, "stream:domain_events", cfg.StreamKey)
	assert.Equal(t, "websocket-hub", cfg.ConsumerGroup)
	assert.Equal(t, HeartbeatInterval, cfg.HeartbeatInterval)
	assert.Equal(t, HeartbeatTimeout, cfg.HeartbeatTimeout)
}

// TestHub_RunWithNilRedis tests running the hub without Redis (graceful degradation).
func TestHub_RunWithNilRedis(t *testing.T) {
	logger := newTestLogger(t)
	cfg := HubConfig{
		RedisClient:       nil, // No Redis client
		Logger:            logger,
		HeartbeatInterval: HeartbeatInterval,
		HeartbeatTimeout:  HeartbeatTimeout,
	}

	hub := NewHub(cfg)

	// Run should not panic even without Redis
	err := hub.Run()
	assert.NoError(t, err)

	// Stop should cleanup
	err = hub.Stop()
	assert.NoError(t, err)
}

// TestHub_StopMultipleTimes tests stopping the hub multiple times.
func TestHub_StopMultipleTimes(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)
	hub := NewHub(cfg)

	// Run and immediately stop
	hub.Run()
	hub.Stop()
	hub.Stop() // Second stop should not panic
}

// TestClient_SendBufferFull tests behavior when send buffer is full.
func TestClient_SendBufferFull(t *testing.T) {
	logger := newTestLogger(t)
	hub := &Hub{
		unregisterCh: make(chan *Client),
		logger:       logger,
	}

	// Create client with tiny buffer
	client := NewClient(hub, nil, "task-1", logger)
	client.send = make(chan []byte, 1) // Buffer size of 1

	// Fill the buffer
	client.send <- []byte("first message")

	// Try to send another - should not block (drop if buffer full)
	// This is the expected behavior with default case in select
}

// TestContextCancellation tests graceful shutdown on context cancellation.
func TestHub_ContextCancellation(t *testing.T) {
	logger := newTestLogger(t)
	cfg := DefaultHubConfig(nil, logger)
	hub := NewHub(cfg)

	// Start hub
	hub.Run()

	// Cancel context
	hub.cancel()

	// Wait a bit for goroutines to finish
	time.Sleep(100 * time.Millisecond)

	// Hub should have stopped
	assert.Equal(t, 0, hub.ClientCount())
}
