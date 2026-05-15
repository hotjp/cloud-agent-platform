// Package mcp implements the MCP (Model Context Protocol) server for the Cloud Agent Platform.
// It communicates with AI agents via JSON-RPC 2.0 over stdio.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
)

// Server implements an MCP server that handles JSON-RPC 2.0 requests over stdio.
type Server struct {
	executor     *ToolExecutor
	logger       *zap.Logger
	protocolVer  string
	initialized  bool
	mu           sync.Mutex
}

// NewServer creates a new MCP server.
func NewServer(executor *ToolExecutor, logger *zap.Logger) *Server {
	return &Server{
		executor:    executor,
		logger:      logger,
		protocolVer: "2024-11-05",
	}
}

// Run starts the MCP server and handles requests from stdin.
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("starting MCP server",
		zap.String("layer", "MCP"),
		zap.String("protocol_version", s.protocolVer),
	)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if err := s.handleMessage(line); err != nil {
			s.logger.Error("failed to handle message",
				zap.String("layer", "MCP"),
				zap.Error(err),
			)
			// Send error response but continue processing
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin scanner error: %w", err)
	}

	return nil
}

// HandleMessage processes a single JSON-RPC message and returns the response.
// This is exported for testing purposes.
func (s *Server) HandleMessage(data []byte) (*JSONRPCResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Try to parse as request
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		errResp := JSONRPCResponse{
			JSONRPC: "2.0",
			Error: &JSONRPCError{
				Code:    CodeParseError,
				Message: "Parse error",
			},
		}
		return &errResp, err
	}

	// Check if it's a notification (no id)
	isNotification := req.ID == nil

	var id json.RawMessage
	if !isNotification {
		id = req.ID
	}

	// Process the method and get result
	var resp *JSONRPCResponse
	switch req.Method {
	case "initialize":
		resp = s.handleInitializeForTest(id, req.Params)
	case "tools/list":
		resp = s.handleToolsListForTest(id)
	case "tools/call":
		resp = s.handleToolsCallForTest(id, req.Params)
	case "resources/list":
		resp = s.handleResourcesListForTest(id)
	case "resources/read":
		resp = s.handleResourcesReadForTest(id, req.Params)
	case "notifications/initialized":
		s.initialized = true
		s.logger.Info("MCP client initialized",
			zap.String("layer", "MCP"),
		)
		if !isNotification {
			return &JSONRPCResponse{JSONRPC: "2.0", ID: id}, nil
		}
		return nil, nil
	default:
		errResp := JSONRPCResponse{
			JSONRPC: "2.0",
			Error: &JSONRPCError{
				Code:    CodeMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
			ID: id,
		}
		return &errResp, nil
	}

	return resp, nil
}

// handleInitializeForTest handles the initialize method for testing.
func (s *Server) handleInitializeForTest(id json.RawMessage, params json.RawMessage) *JSONRPCResponse {
	var input InitializeParams
	if params != nil {
		json.Unmarshal(params, &input)
	}

	s.logger.Info("MCP initialize",
		zap.String("layer", "MCP"),
		zap.String("client_name", input.ClientInfo.Name),
		zap.String("client_version", input.ClientInfo.Version),
	)

	result := InitializeResult{
		ProtocolVersion: s.protocolVer,
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

	resultJSON, _ := json.Marshal(result)
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  resultJSON,
		ID:      id,
	}
}

// handleToolsListForTest handles the tools/list method for testing.
func (s *Server) handleToolsListForTest(id json.RawMessage) *JSONRPCResponse {
	tools := GetToolDefinitions()
	result := ToolsListResult{Tools: tools}
	resultJSON, _ := json.Marshal(result)
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  resultJSON,
		ID:      id,
	}
}

// handleToolsCallForTest handles the tools/call method for testing.
func (s *Server) handleToolsCallForTest(id json.RawMessage, params json.RawMessage) *JSONRPCResponse {
	var input ToolCallParams
	if params != nil {
		json.Unmarshal(params, &input)
	}

	if input.Name == "" {
		errResp := JSONRPCResponse{
			JSONRPC: "2.0",
			Error: &JSONRPCError{
				Code:    CodeInvalidParams,
				Message: "Tool name is required",
			},
			ID: id,
		}
		return &errResp
	}

	ctx := context.Background()
	result, err := s.executor.Execute(ctx, input.Name, input.Arguments)
	if err != nil {
		errResp := JSONRPCResponse{
			JSONRPC: "2.0",
			Error: &JSONRPCError{
				Code:    CodeToolExecutionErr,
				Message: fmt.Sprintf("Tool execution failed: %v", err),
			},
			ID: id,
		}
		return &errResp
	}

	resultJSON, _ := json.Marshal(result)
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  resultJSON,
		ID:      id,
	}
}

// handleResourcesListForTest handles the resources/list method for testing.
func (s *Server) handleResourcesListForTest(id json.RawMessage) *JSONRPCResponse {
	resources := GetResourceDefinitions()
	result := ResourcesListResult{Resources: resources}
	resultJSON, _ := json.Marshal(result)
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  resultJSON,
		ID:      id,
	}
}

// handleResourcesReadForTest handles the resources/read method for testing.
func (s *Server) handleResourcesReadForTest(id json.RawMessage, params json.RawMessage) *JSONRPCResponse {
	var input ResourceReadParams
	if params != nil {
		json.Unmarshal(params, &input)
	}

	if input.URI == "" {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			Error: &JSONRPCError{
				Code:    CodeInvalidParams,
				Message: "URI is required",
			},
			ID: id,
		}
	}

	if reg := GetRegistry(); reg != nil {
		content, err := reg.ReadResource(context.Background(), input.URI)
		if err != nil {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				Error: &JSONRPCError{
					Code:    CodeInternalError,
					Message: fmt.Sprintf("Failed to read resource: %v", err),
				},
				ID: id,
			}
		}
		result := ResourceReadResult{Contents: []ResourceContent{*content}}
		resultJSON, _ := json.Marshal(result)
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  resultJSON,
			ID:      id,
		}
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		Error: &JSONRPCError{
			Code:    CodeInternalError,
			Message: "Resource registry not initialized",
		},
		ID: id,
	}
}

func (s *Server) handleMessage(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Try to parse as request
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return s.sendError(nil, CodeParseError, "Parse error", nil)
	}

	// Check if it's a notification (no id)
	isNotification := req.ID == nil

	var id json.RawMessage
	if !isNotification {
		id = req.ID
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(id, req.Params)

	case "tools/list":
		return s.handleToolsList(id)

	case "tools/call":
		return s.handleToolsCall(id, req.Params)

	case "resources/list":
		return s.handleResourcesList(id)

	case "resources/read":
		return s.handleResourcesRead(id, req.Params)

	case "notifications/initialized":
		// Client has finished initialization, acknowledge silently
		s.initialized = true
		s.logger.Info("MCP client initialized",
			zap.String("layer", "MCP"),
		)
		if !isNotification {
			return s.sendResult(id, struct{}{})
		}
		return nil

	default:
		return s.sendError(id, CodeMethodNotFound, fmt.Sprintf("Method not found: %s", req.Method), nil)
	}
}

// handleInitialize handles the initialize method.
func (s *Server) handleInitialize(id json.RawMessage, params json.RawMessage) error {
	var input InitializeParams
	if params != nil {
		if err := json.Unmarshal(params, &input); err != nil {
			return s.sendError(id, CodeInvalidParams, "Invalid params", nil)
		}
	}

	s.logger.Info("MCP initialize",
		zap.String("layer", "MCP"),
		zap.String("client_name", input.ClientInfo.Name),
		zap.String("client_version", input.ClientInfo.Version),
	)

	result := InitializeResult{
		ProtocolVersion: s.protocolVer,
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

	return s.sendResult(id, result)
}

// handleToolsList handles the tools/list method.
func (s *Server) handleToolsList(id json.RawMessage) error {
	tools := GetToolDefinitions()
	result := ToolsListResult{Tools: tools}
	return s.sendResult(id, result)
}

// handleToolsCall handles the tools/call method.
func (s *Server) handleToolsCall(id json.RawMessage, params json.RawMessage) error {
	var input ToolCallParams
	if params != nil {
		if err := json.Unmarshal(params, &input); err != nil {
			return s.sendError(id, CodeInvalidParams, "Invalid params", nil)
		}
	}

	if input.Name == "" {
		return s.sendError(id, CodeInvalidParams, "Tool name is required", nil)
	}

	ctx := context.Background()
	result, err := s.executor.Execute(ctx, input.Name, input.Arguments)
	if err != nil {
		return s.sendError(id, CodeToolExecutionErr, fmt.Sprintf("Tool execution failed: %v", err), nil)
	}

	return s.sendResult(id, result)
}

// handleResourcesList handles the resources/list method.
func (s *Server) handleResourcesList(id json.RawMessage) error {
	resources := GetResourceDefinitions()
	result := ResourcesListResult{Resources: resources}
	return s.sendResult(id, result)
}

// handleResourcesRead handles the resources/read method.
func (s *Server) handleResourcesRead(id json.RawMessage, params json.RawMessage) error {
	var input ResourceReadParams
	if params != nil {
		if err := json.Unmarshal(params, &input); err != nil {
			return s.sendError(id, CodeInvalidParams, "Invalid params", nil)
		}
	}

	if input.URI == "" {
		return s.sendError(id, CodeInvalidParams, "URI is required", nil)
	}

	// Read resource through registry
	if reg := GetRegistry(); reg != nil {
		content, err := reg.ReadResource(context.Background(), input.URI)
		if err != nil {
			return s.sendError(id, CodeInternalError, fmt.Sprintf("Failed to read resource: %v", err), nil)
		}
		result := ResourceReadResult{Contents: []ResourceContent{*content}}
		return s.sendResult(id, result)
	}

	return s.sendError(id, CodeInternalError, "Resource registry not initialized", nil)
}

// sendResult sends a JSON-RPC success response.
func (s *Server) sendResult(id json.RawMessage, result any) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  resultJSON,
		ID:      id,
	}

	return s.writeJSON(resp)
}

// sendError sends a JSON-RPC error response.
func (s *Server) sendError(id json.RawMessage, code int64, message string, data any) error {
	var dataJSON json.RawMessage
	if data != nil {
		var err error
		dataJSON, err = json.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshal error data: %w", err)
		}
	}

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    dataJSON,
		},
		ID: id,
	}

	return s.writeJSON(resp)
}

// writeJSON writes a JSON-RPC response to stdout.
func (s *Server) writeJSON(resp JSONRPCResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	fmt.Println(string(data))
	return nil
}
