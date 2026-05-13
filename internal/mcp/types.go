// Package mcp implements the MCP (Model Context Protocol) server for the Cloud Agent Platform.
// It exposes 9 tools that allow AI agents to interact with the platform via JSON-RPC 2.0 over stdio.
package mcp

import (
	"encoding/json"
)

// MCP JSON-RPC 2.0 message types

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// JSONRPCNotification represents a JSON-RPC 2.0 notification (no id).
type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCP error codes
const (
	CodeParseError       = -32700
	CodeInvalidRequest   = -32600
	CodeMethodNotFound   = -32601
	CodeInvalidParams    = -32602
	CodeInternalError    = -32603
	CodeToolNotFound     = -32001
	CodeToolExecutionErr = -32002
)

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolsListResult is the result of tools/list.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallParams are the parameters for tools/call.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolCallResult is the result of a tool call.
type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentBlock represents a content block in tool result.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// InitializeParams for the initialize method.
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities     map[string]any        `json:"capabilities"`
	ClientInfo      ClientInfo            `json:"clientInfo"`
}

// ClientInfo contains client information.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is returned from initialize.
type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo     ServerInfo     `json:"serverInfo"`
}

// ServerCapabilities represents server capabilities.
type ServerCapabilities struct {
	Tools     *struct{}          `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
}

// ResourcesCapability indicates resource capabilities.
type ResourcesCapability struct {
	Subscribe bool `json:"subscribe,omitempty"`
	ListHint  bool `json:"listHint,omitempty"`
}

// ServerInfo contains server information.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Resource represents an MCP resource definition.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourcesListResult is the result of resources/list.
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// ResourceContent represents content of a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"` // base64 encoded binary content
}

// ResourceReadResult is the result of resources/read.
type ResourceReadResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ResourceReadParams are the parameters for resources/read.
type ResourceReadParams struct {
	URI    string `json:"uri"`
	Cursor string `json:"cursor,omitempty"`
}

// Resource template URIs
const (
	TaskLogURI         = "cap://tasks/%s/log"
	TaskTimelineURI    = "cap://tasks/%s/timeline"
	TaskArtifactURI    = "cap://tasks/%s/artifact/%s"
	TemplatePromptURI   = "cap://templates/%s/prompt"
)
