// Package tools implements the tool set available to agents.
// File tools, Git tools, command execution, and LLM calls.
package tools

import (
	"context"
	"strings"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
)

// ----------------------------------------------------------------------------
// Tool Adapter: adapts tools.Tool to react.Tool
// ----------------------------------------------------------------------------

// ToolAdapter wraps a Tool (from this package) to expose it as a react.Tool.
// This allows file/Git/command/LLM tools to be used with the ReAct agent.
type ToolAdapter struct {
	name        string
	description string
	inputSchema string
	tool        *ToolWrapper
}

// ToolWrapper wraps a tools.Tool to provide react.Tool interface.
type ToolWrapper struct {
	tool Tool
}

// NewToolAdapter creates a new adapter that wraps a tools.Tool as a react.Tool.
func NewToolAdapter(t Tool) *ToolAdapter {
	return &ToolAdapter{
		name:        t.Name(),
		description: t.Description(),
		inputSchema: t.InputSchema(),
		tool:        &ToolWrapper{tool: t},
	}
}

// Name returns the tool name.
func (a *ToolAdapter) Name() string { return a.name }

// Description returns the tool description.
func (a *ToolAdapter) Description() string { return a.description }

// InputSchema returns the JSON schema for the tool's input.
func (a *ToolAdapter) InputSchema() string { return a.inputSchema }

// Execute runs the wrapped tool and adapts the result.
func (a *ToolAdapter) Execute(ctx context.Context, input map[string]any) (any, error) {
	result, err := a.tool.tool.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	// result is *ToolResult, extract the output
	tr := result // result is *ToolResult (concrete type returned by tools.Tool)
	if !tr.Success {
		return nil, &ToolExecutionError{Message: tr.Error}
	}
	return tr.Output, nil
}

// ToolExecutionError represents a tool execution error.
type ToolExecutionError struct {
	Message string
}

func (e *ToolExecutionError) Error() string {
	return e.Message
}

// Verify ToolAdapter satisfies react.Tool interface.
var _ react.Tool = (*ToolAdapter)(nil)

// ----------------------------------------------------------------------------
// Tool Registry Adapter
// ----------------------------------------------------------------------------

// ToolRegistryAdapter wraps a react.ToolRegistry and adapts tools.Tool to react.Tool.
type ToolRegistryAdapter struct {
	registry *react.ToolRegistry
}

// NewToolRegistryAdapter creates a new adapter for the given registry.
func NewToolRegistryAdapter(registry *react.ToolRegistry) *ToolRegistryAdapter {
	return &ToolRegistryAdapter{registry: registry}
}

// Register registers a tools.Tool to the registry (after adapting to react.Tool).
func (a *ToolRegistryAdapter) Register(t Tool) error {
	adapter := NewToolAdapter(t)
	return a.registry.Register(adapter)
}

// RegisterAdapter registers an already-adapted react.Tool.
func (a *ToolRegistryAdapter) RegisterAdapter(tool react.Tool) error {
	return a.registry.Register(tool)
}

// Get retrieves a tool by name.
func (a *ToolRegistryAdapter) Get(name string) react.Tool {
	return a.registry.Get(name)
}

// List returns all registered tools.
func (a *ToolRegistryAdapter) List() []react.Tool {
	return a.registry.List()
}

// Len returns the number of registered tools.
func (a *ToolRegistryAdapter) Len() int {
	return a.registry.Len()
}

// ----------------------------------------------------------------------------
// Path Security Helper
// ----------------------------------------------------------------------------

// IsPathTraversal checks if a path attempts to escape the workDir.
// Returns true if the path is unsafe (contains ../ or absolute paths
// that escape the workDir).
func IsPathTraversal(workDir, path string) bool {
	// Block obvious traversal attempts
	if strings.Contains(path, "..") {
		return true
	}

	// Block absolute paths that don't start with workDir
	if strings.HasPrefix(path, "/") {
		return true
	}

	return false
}
