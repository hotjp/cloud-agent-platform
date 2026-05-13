// Package mcp implements the MCP (Model Context Protocol) server for the Cloud Agent Platform.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SSEEvent represents a Server-Sent Event.
type SSEEvent struct {
	Event string          `json:"event,omitempty"`
	Data  json.RawMessage `json:"data"`
	ID    string          `json:"id,omitempty"`
}

// SSEClient represents a connected SSE client.
type SSEClient struct {
	ID    string
	Chan  chan SSEEvent
	Close func()
}

// SSEServer handles Server-Sent Events transport for MCP.
type SSEServer struct {
	logger    *zap.Logger
	clients   map[string]*SSEClient
	mu        sync.RWMutex
	clientSeq uint64
}

// NewSSEServer creates a new SSE server.
func NewSSEServer(logger *zap.Logger) *SSEServer {
	return &SSEServer{
		logger:  logger,
		clients: make(map[string]*SSEClient),
	}
}

// HandleSSE handles the SSE endpoint for MCP communication.
func (s *SSEServer) HandleSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	client := s.registerClient()
	defer s.unregisterClient(client.ID)

	ctx := r.Context()

	s.sendEvent(client, SSEEvent{
		Event: "connected",
		Data:  json.RawMessage(fmt.Sprintf(`{"clientId":"%s"}`, client.ID)),
	})

	// Keep connection alive with heartbeat
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-client.Chan:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				s.logger.Error("failed to marshal SSE event",
					zap.String("layer", "MCP"),
					zap.Error(err),
				)
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			if event.Event != "" {
				fmt.Fprintf(w, "event: %s\n\n", event.Event)
			}
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

// HandleMCP handles MCP JSON-RPC requests over SSE.
func (s *SSEServer) HandleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp := s.newError(nil, CodeParseError, "Parse error")
		data, _ := json.Marshal(resp)
		w.Write(data)
		return
	}

	// Extract client ID from query or header
	clientID := r.URL.Query().Get("clientId")

	resp := s.handleMessage(clientID, &req)
	if resp == nil {
		return // Notification, no response needed
	}

	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("failed to marshal response",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		return
	}

	w.Write(data)
}

func (s *SSEServer) handleMessage(clientID string, req *JSONRPCRequest) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(clientID, req.ID, req.Params)
	case "tools/list":
		return s.handleToolsList(clientID, req.ID)
	case "tools/call":
		return s.handleToolsCall(clientID, req.ID, req.Params)
	case "resources/list":
		return s.handleResourcesList(clientID, req.ID)
	case "resources/read":
		return s.handleResourcesRead(clientID, req.ID, req.Params)
	case "notifications/initialized":
		s.logger.Info("MCP client initialized via SSE",
			zap.String("layer", "MCP"),
			zap.String("client_id", clientID),
		)
		return nil
	default:
		return s.newError(req.ID, CodeMethodNotFound, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func (s *SSEServer) handleInitialize(clientID string, id json.RawMessage, params json.RawMessage) *JSONRPCResponse {
	var input InitializeParams
	if params != nil {
		if err := json.Unmarshal(params, &input); err != nil {
			return s.newError(id, CodeInvalidParams, "Invalid params")
		}
	}

	s.logger.Info("MCP initialize via SSE",
		zap.String("layer", "MCP"),
		zap.String("client_name", input.ClientInfo.Name),
		zap.String("client_version", input.ClientInfo.Version),
	)

	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools: &struct{}{},
			Resources: &ResourcesCapability{
				Subscribe: true,
				ListHint:  true,
			},
		},
		ServerInfo: ServerInfo{
			Name:    "cloud-agent-platform",
			Version: "1.0.0",
		},
	}

	return s.newResult(id, result)
}

func (s *SSEServer) handleToolsList(clientID string, id json.RawMessage) *JSONRPCResponse {
	tools := GetToolDefinitions()
	result := ToolsListResult{Tools: tools}
	return s.newResult(id, result)
}

func (s *SSEServer) handleToolsCall(clientID string, id json.RawMessage, params json.RawMessage) *JSONRPCResponse {
	var input ToolCallParams
	if params != nil {
		if err := json.Unmarshal(params, &input); err != nil {
			return s.newError(id, CodeInvalidParams, "Invalid params")
		}
	}

	if input.Name == "" {
		return s.newError(id, CodeInvalidParams, "Tool name is required")
	}

	// Execute tool through executor
	// Note: SSE mode uses a shared executor
	result, err := globalExecutor.Execute(context.Background(), input.Name, input.Arguments)
	if err != nil {
		return s.newError(id, CodeToolExecutionErr, fmt.Sprintf("Tool execution failed: %v", err))
	}

	return s.newResult(id, result)
}

func (s *SSEServer) handleResourcesList(clientID string, id json.RawMessage) *JSONRPCResponse {
	resources := GetResourceDefinitions()
	result := ResourcesListResult{Resources: resources}
	return s.newResult(id, result)
}

func (s *SSEServer) handleResourcesRead(clientID string, id json.RawMessage, params json.RawMessage) *JSONRPCResponse {
	var input ResourceReadParams
	if params != nil {
		if err := json.Unmarshal(params, &input); err != nil {
			return s.newError(id, CodeInvalidParams, "Invalid params")
		}
	}

	if input.URI == "" {
		return s.newError(id, CodeInvalidParams, "URI is required")
	}

	content, err := globalRegistry.ReadResource(context.Background(), input.URI)
	if err != nil {
		return s.newError(id, CodeInternalError, fmt.Sprintf("Failed to read resource: %v", err))
	}

	result := ResourceReadResult{Contents: []ResourceContent{*content}}
	return s.newResult(id, result)
}

func (s *SSEServer) newResult(id json.RawMessage, result any) *JSONRPCResponse {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  resultJSON,
		ID:      id,
	}
}

func (s *SSEServer) newError(id json.RawMessage, code int64, message string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
		ID: id,
	}
}

func (s *SSEServer) registerClient() *SSEClient {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clientSeq++
	clientID := fmt.Sprintf("sse-client-%d", s.clientSeq)

	client := &SSEClient{
		ID:   clientID,
		Chan: make(chan SSEEvent, 100),
		Close: func() {
			s.unregisterClient(clientID)
		},
	}

	s.clients[clientID] = client
	s.logger.Info("SSE client connected",
		zap.String("layer", "MCP"),
		zap.String("client_id", clientID),
		zap.Int("total_clients", len(s.clients)),
	)

	return client
}

func (s *SSEServer) unregisterClient(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if client, ok := s.clients[clientID]; ok {
		close(client.Chan)
		delete(s.clients, clientID)
		s.logger.Info("SSE client disconnected",
			zap.String("layer", "MCP"),
			zap.String("client_id", clientID),
			zap.Int("total_clients", len(s.clients)),
		)
	}
}

func (s *SSEServer) sendEvent(client *SSEClient, event SSEEvent) {
	select {
	case client.Chan <- event:
	default:
		s.logger.Warn("SSE client channel full, dropping event",
			zap.String("layer", "MCP"),
			zap.String("client_id", client.ID),
		)
	}
}

// BroadcastEvent sends an event to all connected SSE clients.
func (s *SSEServer) BroadcastEvent(event SSEEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, client := range s.clients {
		s.sendEvent(client, event)
	}
}
