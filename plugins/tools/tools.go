// Package tools implements the Agent tool set.
// Includes file operations, Git operations, command execution, and LLM tools.
package tools

import "context"

// Tool represents an executable tool for Agents.
type Tool interface {
	// Info returns tool metadata.
	Info() ToolInfo
	// Run executes the tool with given input.
	Run(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
}

// ToolInfo contains tool metadata.
type ToolInfo struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
}

// Tools provides all available Agent tools.
type Tools struct {
	// TODO: Add tool instances (ReadFile, WriteFile, EditFile, etc.)
}

// New creates a new Tools instance.
func New() *Tools {
	return &Tools{}
}
