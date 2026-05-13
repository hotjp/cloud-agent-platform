// Package tools implements the tool set available to agents.
// File tools, Git tools, command execution, and LLM calls.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolResult represents the structured result of a tool execution.
type ToolResult struct {
	Success bool        `json:"success"`
	Output  any         `json:"output,omitempty"`
	Error   string      `json:"error,omitempty"`
	Meta    ResultMeta  `json:"meta,omitempty"`
}

// ResultMeta contains metadata about the tool execution.
type ResultMeta struct {
	DurationMS int64  `json:"duration_ms,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
}

// NewSuccessResult creates a successful tool result.
func NewSuccessResult(output any) *ToolResult {
	return &ToolResult{
		Success: true,
		Output:  output,
	}
}

// NewErrorResult creates an error tool result.
func NewErrorResult(err error) *ToolResult {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return &ToolResult{
		Success: false,
		Error:   errMsg,
	}
}

// ToMap converts the result to a map for JSON serialization.
func (r *ToolResult) ToMap() map[string]any {
	m := map[string]any{
		"success": r.Success,
	}
	if r.Success {
		m["output"] = r.Output
	} else {
		m["error"] = r.Error
	}
	if r.Meta.ToolName != "" {
		m["tool_name"] = r.Meta.ToolName
	}
	if r.Meta.DurationMS > 0 {
		m["duration_ms"] = r.Meta.DurationMS
	}
	return m
}

// FormatForLLM formats the result as a string for LLM consumption.
func (r *ToolResult) FormatForLLM() string {
	if r.Success {
		return fmt.Sprintf("[TOOL SUCCESS] %s", formatValue(r.Output))
	}
	return fmt.Sprintf("[TOOL ERROR] %s", r.Error)
}

func formatValue(v any) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		return val
	case error:
		return val.Error()
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}

// toolBase provides common functionality for all tools.
type toolBase struct {
	name        string
	description string
	inputSchema string
}

// name returns the tool name.
func (tb *toolBase) Name() string { return tb.name }

// Description returns the tool description.
func (tb *toolBase) Description() string { return tb.description }

// InputSchema returns the JSON schema for the tool's input.
func (tb *toolBase) InputSchema() string { return tb.inputSchema }

// Tool interface is implemented by all agent tools.
type Tool interface {
	Name() string
	Description() string
	InputSchema() string
	Execute(ctx context.Context, input map[string]any) (*ToolResult, error)
}