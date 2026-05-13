// Package mcpserver implements MCP Server with 9 Tools and 4 Resources.
// Exposes platform capabilities via Model Context Protocol.
package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"go.uber.org/zap"
)

// MCPServer implements the MCP Server with tools and resources.
type MCPServer struct {
	registry *mcp.Registry
	logger   *zap.Logger
}

// NewMCPServer creates a new MCPServer instance.
func NewMCPServer(logger *zap.Logger) *MCPServer {
	return &MCPServer{
		logger: logger,
	}
}

// Tools returns all registered MCP tools.
func (s *MCPServer) Tools() []Tool {
	return Tools()
}

// Resources returns all registered MCP resources.
func (s *MCPServer) Resources() []Resource {
	return Resources()
}

// ExecuteTool executes a tool by name with the given parameters.
func (s *MCPServer) ExecuteTool(ctx context.Context, name string, params json.RawMessage) (*mcp.ToolCallResult, error) {
	tools := Tools()
	for _, tool := range tools {
		if tool.Name == name {
			return tool.Handler(ctx, params)
		}
	}
	return &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: "Unknown tool: " + name}},
		IsError: true,
	}, nil
}
