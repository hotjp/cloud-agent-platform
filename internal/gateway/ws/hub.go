// Package ws implements the WebSocket Hub for real-time event push.
// It manages rooms (one per Task/Session), client connections, and distributes
// domain events from Redis Stream to subscribed clients.
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	// WebSocket path prefix.
	WebSocketPath = "/api/v1/ws"

	// Room key prefix for room identification.
	RoomKeyPrefix = "room:task:"

	// HeartbeatInterval is the interval for sending ping messages.
	HeartbeatInterval = 30 * time.Second

	// HeartbeatTimeout is the timeout for receiving pong response.
	HeartbeatTimeout = 10 * time.Second

	// ClientSendBufferSize is the buffer size for client send channel.
	ClientSendBufferSize = 256

	// MaxMessageSize is the maximum message size in bytes.
	MaxMessageSize = 8192
)

// Upgrader configures the WebSocket upgrader.
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking in production
		return true
	},
}

// WSEvent represents a WebSocket event sent to clients.
type WSEvent struct {
	Type      string          `json:"type"`
	TaskID    string          `json:"taskId,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// Room represents a WebSocket room for a specific task.
// Clients in the same room receive the same events.
type Room struct {
	key    string
	clients map[*Client]struct{}
	mu      sync.RWMutex
}

// NewRoom creates a new Room with the given key.
func NewRoom(key string) *Room {
	return &Room{
		key:    key,
		clients: make(map[*Client]struct{}),
	}
}

// Add adds a client to the room.
func (r *Room) Add(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[c] = struct{}{}
}

// Remove removes a client from the room.
func (r *Room) Remove(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, c)
}

// Broadcast sends an event to all clients in the room.
func (r *Room) Broadcast(event *WSEvent) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	for client := range r.clients {
		select {
		case client.send <- data:
		default:
			// Client buffer full, skip
		}
	}
}

// ClientCount returns the number of clients in the room.
func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

// Hub maintains the set of active rooms and clients and broadcasts messages to rooms.
type Hub struct {
	// Room management
	rooms    map[string]*Room
	roomsMu  sync.RWMutex

	// Client management
	clients  map[*Client]struct{}
	clientsMu sync.RWMutex

	// Redis Stream consumer
	redisClient *redis.Client
	streamKey   string
	consumerID  string

	// Configuration
	logger          *zap.Logger
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Channel for incoming events from Redis Stream
	eventCh chan *StreamEvent

	// Channel for registration requests
	registerCh chan *Client

	// Channel for unregistration requests
	unregisterCh chan *Client

	// WaitGroup for graceful shutdown
	wg sync.WaitGroup
}

// StreamEvent represents an event from Redis Stream.
type StreamEvent struct {
	ID          string
	AggregateID string
	EventType   string
	Payload     []byte
}

// HubConfig holds configuration for the Hub.
type HubConfig struct {
	RedisClient      *redis.Client
	StreamKey        string
	ConsumerGroup    string
	ConsumerID       string
	Logger           *zap.Logger
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
}

// DefaultHubConfig returns the default hub configuration.
func DefaultHubConfig(redisClient *redis.Client, logger *zap.Logger) HubConfig {
	return HubConfig{
		RedisClient:       redisClient,
		StreamKey:         "stream:domain_events",
		ConsumerGroup:     "websocket-hub",
		ConsumerID:        fmt.Sprintf("ws-hub-%d", time.Now().UnixNano()),
		Logger:            logger,
		HeartbeatInterval: HeartbeatInterval,
		HeartbeatTimeout:  HeartbeatTimeout,
	}
}

// NewHub creates a new Hub.
func NewHub(cfg HubConfig) *Hub {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = HeartbeatInterval
	}
	if cfg.HeartbeatTimeout == 0 {
		cfg.HeartbeatTimeout = HeartbeatTimeout
	}
	if cfg.StreamKey == "" {
		cfg.StreamKey = "stream:domain_events"
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Hub{
		rooms:              make(map[string]*Room),
		clients:            make(map[*Client]struct{}),
		redisClient:        cfg.RedisClient,
		streamKey:          cfg.StreamKey,
		consumerID:         cfg.ConsumerID,
		logger:             cfg.Logger,
		heartbeatInterval: cfg.HeartbeatInterval,
		heartbeatTimeout:  cfg.HeartbeatTimeout,
		ctx:               ctx,
		cancel:            cancel,
		eventCh:           make(chan *StreamEvent, 1024),
		registerCh:        make(chan *Client),
		unregisterCh:      make(chan *Client),
	}
}

// Run starts the hub's main loop.
func (h *Hub) Run() error {
	// Ensure consumer group exists only if Redis client is configured
	if h.redisClient != nil {
		if err := h.ensureConsumerGroup(); err != nil {
			h.logger.Warn("failed to ensure consumer group, will retry on read",
				zap.String("stream", h.streamKey),
				zap.String("group", "websocket-hub"),
				zap.Error(err),
			)
		}
	}

	// Start Redis Stream consumer only if Redis client is configured
	if h.redisClient != nil {
		h.wg.Add(1)
		go h.consumeStream()
	}

	// Start the main event loop
	h.wg.Add(1)
	go h.run()

	h.logger.Info("websocket hub started",
		zap.String("stream", h.streamKey),
		zap.String("consumer_id", h.consumerID),
	)

	return nil
}

// ensureConsumerGroup ensures the consumer group exists for the Redis Stream.
func (h *Hub) ensureConsumerGroup() error {
	return h.redisClient.XGroupCreateMkStream(h.ctx, h.streamKey, "websocket-hub", "0").Err()
}

// consumeStream reads events from Redis Stream and sends them to the event channel.
func (h *Hub) consumeStream() {
	defer h.wg.Done()

	// Guard against nil redis client
	if h.redisClient == nil {
		h.logger.Warn("consumeStream called with nil redis client, exiting")
		return
	}

	for {
		select {
		case <-h.ctx.Done():
			return
		default:
		}

		// Read from stream using consumer group
		streams, err := h.redisClient.XReadGroup(h.ctx, &redis.XReadGroupArgs{
			Group:    "websocket-hub",
			Consumer: h.consumerID,
			Streams:  []string{h.streamKey, ">"},
			Count:    100,
			Block:    5 * time.Second,
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue
			}
			h.logger.Error("failed to read from stream",
				zap.String("stream", h.streamKey),
				zap.Error(err),
			)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				event := &StreamEvent{
					ID:          msg.ID,
					AggregateID: getStringField(msg.Values, "aggregate_id"),
					EventType:   getStringField(msg.Values, "event_type"),
					Payload:     []byte(getStringField(msg.Values, "payload")),
				}

				select {
				case h.eventCh <- event:
					// Acknowledge the message
					h.redisClient.XAck(h.ctx, h.streamKey, "websocket-hub", msg.ID)
				case <-h.ctx.Done():
					return
				}
			}
		}
	}
}

// getStringField safely extracts a string field from Redis hash values.
func getStringField(values map[string]interface{}, field string) string {
	if v, ok := values[field].(string); ok {
		return v
	}
	return ""
}

// run is the main event loop that processes registrations, unregistrations, and stream events.
func (h *Hub) run() {
	defer h.wg.Done()

	for {
		select {
		case <-h.ctx.Done():
			return

		case client := <-h.registerCh:
			h.registerClient(client)

		case client := <-h.unregisterCh:
			h.unregisterClient(client)

		case event := <-h.eventCh:
			h.handleStreamEvent(event)
		}
	}
}

// registerClient registers a new client and adds it to the appropriate room.
func (h *Hub) registerClient(client *Client) {
	h.clientsMu.Lock()
	h.clients[client] = struct{}{}
	h.clientsMu.Unlock()

	// Get or create room for this task
	room := h.getOrCreateRoom(client.taskID)
	room.Add(client)

	h.logger.Debug("client registered",
		zap.String("client_id", client.ID()),
		zap.String("task_id", client.taskID),
		zap.Int("room_size", room.ClientCount()),
	)
}

// unregisterClient removes a client from its room and the hub.
func (h *Hub) unregisterClient(client *Client) {
	h.clientsMu.Lock()
	_, exists := h.clients[client]
	if !exists {
		h.clientsMu.Unlock()
		return // Already unregistered
	}
	delete(h.clients, client)
	h.clientsMu.Unlock()

	// Remove from room
	room := h.getRoom(client.taskID)
	if room != nil {
		room.Remove(client)
		h.logger.Debug("client unregistered",
			zap.String("client_id", client.ID()),
			zap.String("task_id", client.taskID),
			zap.Int("room_size", room.ClientCount()),
		)
	}

	// Close the client's send channel (guard against double-close)
	client.mu.Lock()
	if !client.closed {
		client.closed = true
		close(client.send)
	}
	client.mu.Unlock()
}

// handleStreamEvent distributes a stream event to the appropriate room.
func (h *Hub) handleStreamEvent(event *StreamEvent) {
	// Map event type to WebSocket event type
	wsEvent := h.mapToWSEvent(event)
	if wsEvent == nil {
		return
	}

	// Broadcast to the room for this task
	room := h.getRoom(event.AggregateID)
	if room != nil && room.ClientCount() > 0 {
		room.Broadcast(wsEvent)
		h.logger.Debug("event broadcast to room",
			zap.String("event_type", event.EventType),
			zap.String("task_id", event.AggregateID),
			zap.Int("clients", room.ClientCount()),
		)
	}
}

// mapToWSEvent maps a domain event to a WebSocket event.
func (h *Hub) mapToWSEvent(event *StreamEvent) *WSEvent {
	var wsType string

	switch {
	case isTaskCreatedEvent(event.EventType):
		wsType = "task_created"
	case isTaskUpdatedEvent(event.EventType):
		wsType = "task_updated"
	case isTaskCompletedEvent(event.EventType):
		wsType = "task_completed"
	case isTaskFailedEvent(event.EventType):
		wsType = "task_failed"
	case isSubtaskProgressEvent(event.EventType):
		wsType = "subtask_progress"
	case isTaskStatusChangedEvent(event.EventType):
		wsType = "task.status_changed"
	case isAgentThoughtEvent(event.EventType):
		wsType = "agent.thought"
	case isArtifactCreatedEvent(event.EventType):
		wsType = "artifact.created"
	default:
		// Unknown event type, skip
		return nil
	}

	return &WSEvent{
		Type:      wsType,
		TaskID:    event.AggregateID,
		Payload:   event.Payload,
		Timestamp: time.Now().UTC(),
	}
}

// isTaskCreatedEvent checks if the event type is a task creation.
func isTaskCreatedEvent(eventType string) bool {
	return eventType == "TaskSubmittedV1" || eventType == "TaskCreatedV1"
}

// isTaskUpdatedEvent checks if the event type is a task update.
func isTaskUpdatedEvent(eventType string) bool {
	return eventType == "TaskUpdatedV1" || eventType == "TaskStatusChangedV1"
}

// isTaskCompletedEvent checks if the event type is a task completion.
func isTaskCompletedEvent(eventType string) bool {
	return eventType == "TaskCompletedV1"
}

// isTaskFailedEvent checks if the event type is a task failure.
func isTaskFailedEvent(eventType string) bool {
	return eventType == "TaskFailedV1" || eventType == "TaskCancelledV1"
}

// isSubtaskProgressEvent checks if the event type is a subtask progress update.
func isSubtaskProgressEvent(eventType string) bool {
	return eventType == "SubtaskProgressV1" || eventType == "SubtaskCreatedV1"
}

// isTaskStatusChangedEvent checks if the event type is a task status change.
func isTaskStatusChangedEvent(eventType string) bool {
	return len(eventType) > 4 && eventType[:4] == "Task"
}

// isAgentThoughtEvent checks if the event type is an agent thought.
func isAgentThoughtEvent(eventType string) bool {
	return len(eventType) > 5 && eventType[:5] == "Agent"
}

// isArtifactCreatedEvent checks if the event type is an artifact creation.
func isArtifactCreatedEvent(eventType string) bool {
	return len(eventType) >= 8 && eventType[:8] == "Artifact"
}

// getRoom gets a room by task ID.
func (h *Hub) getRoom(taskID string) *Room {
	h.roomsMu.RLock()
	defer h.roomsMu.RUnlock()
	return h.rooms[RoomKeyPrefix+taskID]
}

// getOrCreateRoom gets an existing room or creates a new one.
func (h *Hub) getOrCreateRoom(taskID string) *Room {
	key := RoomKeyPrefix + taskID

	h.roomsMu.Lock()
	defer h.roomsMu.Unlock()

	if room, ok := h.rooms[key]; ok {
		return room
	}

	room := NewRoom(key)
	h.rooms[key] = room
	return room
}

// Stop gracefully stops the hub.
func (h *Hub) Stop() error {
	h.logger.Info("stopping websocket hub")
	h.cancel()

	// Collect and remove all clients, then close their channels
	h.clientsMu.Lock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.clients = make(map[*Client]struct{})
	h.clientsMu.Unlock()

	// Close all client connections (with guard against double-close)
	for _, client := range clients {
		client.mu.Lock()
		if !client.closed {
			client.closed = true
			close(client.send)
		}
		client.mu.Unlock()
	}

	// Wait for goroutines to finish
	h.wg.Wait()

	h.logger.Info("websocket hub stopped")
	return nil
}

// ServeWS handles WebSocket upgrade requests.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, taskID string) {
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return
	}

	client := NewClient(h, conn, taskID, h.logger)

	// Register the client
	h.registerCh <- client

	// Start the client's read/write loops
	client.Run()
}

// HandleWebSocket handles WebSocket connections at the root path.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract task ID from URL path
	// URL format: /api/v1/ws/{taskId}
	path := r.URL.Path
	taskID := extractTaskID(path)

	h.ServeWS(w, r, taskID)
}

// extractTaskID extracts the task ID from the WebSocket URL path.
func extractTaskID(path string) string {
	// Expected format: /api/v1/ws/{taskId}
	if len(path) > len(WebSocketPath)+1 {
		return path[len(WebSocketPath)+1:]
	}
	return ""
}

// ClientCount returns the total number of connected clients.
func (h *Hub) ClientCount() int {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	return len(h.clients)
}

// RoomCount returns the number of active rooms.
func (h *Hub) RoomCount() int {
	h.roomsMu.RLock()
	defer h.roomsMu.RUnlock()
	return len(h.rooms)
}

// PushApprovalRequired pushes an approval required event to the room for a task.
// It implements the guardian.WSPusher interface.
func (h *Hub) PushApprovalRequired(ctx context.Context, taskID string, taskGoal string, estimatedCost float64, riskLevel string, requestedAt, expiresAt time.Time, timeout time.Duration) error {
	room := h.getRoom(taskID)
	if room == nil {
		return fmt.Errorf("room not found for task: %s", taskID)
	}

	payload := ApprovalPushPayload{
		TaskID:         taskID,
		TaskGoal:       taskGoal,
		EstimatedCost:  estimatedCost,
		RiskLevel:      riskLevel,
		RequestedAt:    requestedAt,
		ExpiresAt:      expiresAt,
		RequireApproval: true,
		Timeout:        timeout.String(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal approval request: %w", err)
	}

	event := &WSEvent{
		Type:      "task.approval_required",
		TaskID:    taskID,
		Payload:   payloadBytes,
		Timestamp: time.Now().UTC(),
	}

	room.Broadcast(event)

	h.logger.Debug("approval required event pushed",
		zap.String("task_id", taskID),
		zap.Int("clients", room.ClientCount()),
	)

	return nil
}

// ApprovalPushPayload is the payload for approval required WebSocket events.
type ApprovalPushPayload struct {
	TaskID         string    `json:"taskId"`
	TaskGoal       string    `json:"taskGoal"`
	EstimatedCost  float64   `json:"estimatedCost"`
	RiskLevel      string    `json:"riskLevel"`
	RequestedAt    time.Time `json:"requestedAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
	RequireApproval bool      `json:"requireApproval"`
	Timeout        string    `json:"timeout"`
}

// PushTaskCreated pushes a task_created event to the room for a task.
func (h *Hub) PushTaskCreated(ctx context.Context, taskID string, payload []byte) error {
	return h.pushEvent(ctx, taskID, "task_created", payload)
}

// PushTaskUpdated pushes a task_updated event to the room for a task.
func (h *Hub) PushTaskUpdated(ctx context.Context, taskID string, payload []byte) error {
	return h.pushEvent(ctx, taskID, "task_updated", payload)
}

// PushTaskCompleted pushes a task_completed event to the room for a task.
func (h *Hub) PushTaskCompleted(ctx context.Context, taskID string, payload []byte) error {
	return h.pushEvent(ctx, taskID, "task_completed", payload)
}

// PushTaskFailed pushes a task_failed event to the room for a task.
func (h *Hub) PushTaskFailed(ctx context.Context, taskID string, payload []byte) error {
	return h.pushEvent(ctx, taskID, "task_failed", payload)
}

// PushSubtaskProgress pushes a subtask_progress event to the room for a task.
func (h *Hub) PushSubtaskProgress(ctx context.Context, taskID string, payload []byte) error {
	return h.pushEvent(ctx, taskID, "subtask_progress", payload)
}

// pushEvent pushes a generic event to the room for a task.
func (h *Hub) pushEvent(ctx context.Context, taskID string, eventType string, payload []byte) error {
	room := h.getRoom(taskID)
	if room == nil {
		return fmt.Errorf("room not found for task: %s", taskID)
	}

	event := &WSEvent{
		Type:      eventType,
		TaskID:    taskID,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	}

	room.Broadcast(event)

	h.logger.Debug("event pushed to room",
		zap.String("event_type", eventType),
		zap.String("task_id", taskID),
		zap.Int("clients", room.ClientCount()),
	)

	return nil
}

// BroadcastToAll sends an event to all connected clients (for dashboard updates).
func (h *Hub) BroadcastToAll(eventType string, payload []byte) {
	h.roomsMu.RLock()
	defer h.roomsMu.RUnlock()

	event := &WSEvent{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	}

	for roomKey, room := range h.rooms {
		room.Broadcast(event)
		h.logger.Debug("broadcast event sent to room",
			zap.String("event_type", eventType),
			zap.String("room", roomKey),
			zap.Int("clients", room.ClientCount()),
		)
	}
}
