// Package ws implements the WebSocket Hub for real-time event push.
package ws

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = HeartbeatTimeout

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = HeartbeatInterval - (5 * time.Second)

	// Maximum message size in bytes.
	maxMessageSize = MaxMessageSize
)

// Client represents a WebSocket client connection to the hub.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	taskID string
	logger *zap.Logger
	id     string

	// Authentication state
	authenticated bool
	closed       bool
	mu           sync.Mutex
}

// NewClient creates a new WebSocket client.
func NewClient(hub *Hub, conn *websocket.Conn, taskID string, logger *zap.Logger) *Client {
	return &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, ClientSendBufferSize),
		taskID: taskID,
		logger: logger,
		id:     fmt.Sprintf("%s-%d", taskID, time.Now().UnixNano()),
	}
}

// ID returns the unique identifier for this client.
func (c *Client) ID() string {
	return c.id
}

// TaskID returns the task ID this client is subscribed to.
func (c *Client) TaskID() string {
	return c.taskID
}

// Run starts the client's read and write pumps.
func (c *Client) Run() {
	// Start write pump
	go c.writePump()

	// Start read pump
	c.readPump()
}

// writePump pumps messages from the send channel to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				c.logger.Error("failed to get next writer",
					zap.String("client_id", c.id),
					zap.Error(err),
				)
				return
			}

			w.Write(message)

			// Add queued messages to the current WebSocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				c.logger.Error("failed to close writer",
					zap.String("client_id", c.id),
					zap.Error(err),
				)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.logger.Debug("ping failed, closing connection",
					zap.String("client_id", c.id),
					zap.Error(err),
				)
				return
			}
		}
	}
}

// readPump pumps messages from the WebSocket connection to the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregisterCh <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Debug("websocket connection closed unexpectedly",
					zap.String("client_id", c.id),
					zap.Error(err),
				)
			}
			break
		}

		// Parse the incoming message
		c.handleMessage(message)
	}
}

// handleMessage processes incoming WebSocket messages.
func (c *Client) handleMessage(message []byte) {
	// Try to parse as JSON
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		c.logger.Debug("failed to parse message",
			zap.String("client_id", c.id),
			zap.Error(err),
		)
		return
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		c.logger.Debug("message missing type field",
			zap.String("client_id", c.id),
		)
		return
	}

	switch msgType {
	case "auth":
		c.handleAuth(msg)
	case "ping":
		c.handlePing()
	case "subscribe":
		c.handleSubscribe(msg)
	default:
		c.logger.Debug("unknown message type",
			zap.String("client_id", c.id),
			zap.String("type", msgType),
		)
	}
}

// handleAuth handles authentication messages.
// Expected format: { "type": "auth", "token": "jwt-token" }
func (c *Client) handleAuth(msg map[string]interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authenticated {
		c.sendError("already authenticated")
		return
	}

	token, ok := msg["token"].(string)
	if !ok || token == "" {
		c.sendError("missing or invalid token")
		return
	}

	// TODO: Validate JWT token with L3-Authz service
	// For now, we just mark as authenticated
	c.authenticated = true

	c.logger.Info("client authenticated",
		zap.String("client_id", c.id),
		zap.String("task_id", c.taskID),
	)

	// Send auth success response
	response := map[string]interface{}{
		"type": "auth_success",
	}
	c.sendJSON(response)
}

// handlePing handles ping messages and responds with pong.
func (c *Client) handlePing() {
	response := map[string]interface{}{
		"type": "pong",
	}
	c.sendJSON(response)
}

// handleSubscribe handles subscription requests.
// Expected format: { "type": "subscribe", "taskId": "task-id" }
func (c *Client) handleSubscribe(msg map[string]interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.authenticated {
		c.sendError("not authenticated")
		return
	}

	taskID, ok := msg["taskId"].(string)
	if !ok || taskID == "" {
		c.sendError("missing or invalid taskId")
		return
	}

	// Unsubscribe from current room if different
	if c.taskID != taskID {
		// Remove from current room
		if room := c.hub.getRoom(c.taskID); room != nil {
			room.Remove(c)
		}

		// Join new room
		c.taskID = taskID
		room := c.hub.getOrCreateRoom(taskID)
		room.Add(c)

		c.logger.Info("client subscribed to new task",
			zap.String("client_id", c.id),
			zap.String("task_id", taskID),
		)
	}

	// Send subscribe success response
	response := map[string]interface{}{
		"type":    "subscribed",
		"taskId":  taskID,
	}
	c.sendJSON(response)
}

// sendJSON sends a JSON message to the client.
func (c *Client) sendJSON(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("failed to marshal JSON",
			zap.String("client_id", c.id),
			zap.Error(err),
		)
		return
	}

	select {
	case c.send <- data:
	default:
		c.logger.Warn("client send buffer full, dropping message",
			zap.String("client_id", c.id),
		)
	}
}

// sendError sends an error message to the client.
func (c *Client) sendError(message string) {
	response := map[string]interface{}{
		"type":    "error",
		"message": message,
	}
	c.sendJSON(response)
}
