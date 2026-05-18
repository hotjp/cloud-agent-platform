// Package mcp provides tests for the MCP server implementation.
package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolSchemas(t *testing.T) {
	tools := GetToolDefinitions()
	assert.Len(t, tools, 9)

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true

		// Verify each tool has required fields
		assert.NotEmpty(t, tool.Name)
		assert.NotEmpty(t, tool.Description)
		assert.NotEmpty(t, tool.InputSchema)

		// Verify inputSchema is valid JSON
		var schema map[string]any
		err := json.Unmarshal(tool.InputSchema, &schema)
		require.NoError(t, err, "tool %s has invalid inputSchema", tool.Name)

		// Verify it has type: object
		assert.Equal(t, "object", schema["type"])
	}

	// Verify all required tools are present
	requiredTools := []string{
		"task_submit", "task_status", "task_list", "task_cancel",
		"context_approve", "context_reject",
		"agent_list", "session_list",
	}
	for _, name := range requiredTools {
		assert.True(t, toolNames[name], "tool %s is missing", name)
	}
}

func TestValidateToolParams_TaskSubmit(t *testing.T) {
	tests := []struct {
		name    string
		params  string
		wantErr bool
	}{
		{
			name:    "valid minimal params",
			params:  `{"goal": "test goal"}`,
			wantErr: false,
		},
		{
			name:    "missing goal",
			params:  `{}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			params:  `{invalid}`,
			wantErr: true,
		},
		{
			name:    "valid with all fields",
			params:  `{"goal": "test", "priority": 5, "tags": ["tag1"]}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolParams("task_submit", json.RawMessage(tt.params))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateToolParams_TaskStatus(t *testing.T) {
	tests := []struct {
		name    string
		params  string
		wantErr bool
	}{
		{
			name:    "valid with taskId",
			params:  `{"taskId": "task_123"}`,
			wantErr: false,
		},
		{
			name:    "missing taskId",
			params:  `{}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolParams("task_status", json.RawMessage(tt.params))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateToolParams_TaskCancel(t *testing.T) {
	tests := []struct {
		name    string
		params  string
		wantErr bool
	}{
		{
			name:    "valid",
			params:  `{"taskId": "task_123", "reason": "user requested"}`,
			wantErr: false,
		},
		{
			name:    "missing reason",
			params:  `{"taskId": "task_123"}`,
			wantErr: true,
		},
		{
			name:    "missing both",
			params:  `{}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolParams("task_cancel", json.RawMessage(tt.params))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateToolParams_TaskList(t *testing.T) {
	tests := []struct {
		name    string
		params  string
		wantErr bool
	}{
		{
			name:    "empty is valid",
			params:  `{}`,
			wantErr: false,
		},
		{
			name:    "with status filter",
			params:  `{"status": "running"}`,
			wantErr: false,
		},
		{
			name:    "invalid status value",
			params:  `{"status": "invalid_status"}`,
			wantErr: true,
		},
		{
			name:    "with limit",
			params:  `{"limit": 50}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolParams("task_list", json.RawMessage(tt.params))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateToolParams_ContextApprove(t *testing.T) {
	tests := []struct {
		name    string
		params  string
		wantErr bool
	}{
		{
			name:    "valid minimal",
			params:  `{"taskId": "task_123", "subtaskId": "sub_001"}`,
			wantErr: false,
		},
		{
			name:    "missing taskId",
			params:  `{"subtaskId": "sub_001"}`,
			wantErr: true,
		},
		{
			name:    "missing subtaskId",
			params:  `{"taskId": "task_123"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolParams("context_approve", json.RawMessage(tt.params))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateToolParams_ContextReject(t *testing.T) {
	tests := []struct {
		name    string
		params  string
		wantErr bool
	}{
		{
			name:    "valid",
			params:  `{"taskId": "task_123", "subtaskId": "sub_001", "feedback": "too risky"}`,
			wantErr: false,
		},
		{
			name:    "missing feedback",
			params:  `{"taskId": "task_123", "subtaskId": "sub_001"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolParams("context_reject", json.RawMessage(tt.params))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateToolParams_AgentList(t *testing.T) {
	tests := []struct {
		name    string
		params  string
		wantErr bool
	}{
		{
			name:    "empty is valid",
			params:  `{}`,
			wantErr: false,
		},
		{
			name:    "with capability filter",
			params:  `{"capability": "coding"}`,
			wantErr: false,
		},
		{
			name:    "invalid capability",
			params:  `{"capability": "invalid"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolParams("agent_list", json.RawMessage(tt.params))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateToolParams_SessionList(t *testing.T) {
	tests := []struct {
		name    string
		params  string
		wantErr bool
	}{
		{
			name:    "empty is valid",
			params:  `{}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolParams("session_list", json.RawMessage(tt.params))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateToolParams_UnknownTool(t *testing.T) {
	err := ValidateToolParams("unknown_tool", json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}

func TestValidateToolParams_NilParams(t *testing.T) {
	// task_submit requires goal, so nil params should fail
	err := ValidateToolParams("task_submit", nil)
	assert.Error(t, err)

	// session_list has no required fields, so nil should be ok
	err = ValidateToolParams("session_list", nil)
	assert.NoError(t, err)
}

func TestValidateToolParams_NullParams(t *testing.T) {
	// task_submit requires goal, so null params should fail
	err := ValidateToolParams("task_submit", json.RawMessage(`null`))
	assert.Error(t, err)

	// session_list has no required fields, so null should be ok
	err = ValidateToolParams("session_list", json.RawMessage(`null`))
	assert.NoError(t, err)
}

func TestJSONRPCRequestParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		method   string
	}{
		{
			name:    "valid request with id",
			input:   `{"jsonrpc": "2.0", "method": "tools/list", "id": 1}`,
			wantErr: false,
			method:  "tools/list",
		},
		{
			name:    "valid notification (no id)",
			input:   `{"jsonrpc": "2.0", "method": "notifications/initialized"}`,
			wantErr: false,
			method:  "notifications/initialized",
		},
		{
			name:    "valid request with params",
			input:   `{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "task_submit"}, "id": 1}`,
			wantErr: false,
			method:  "tools/call",
		},
		{
			name:    "invalid JSON",
			input:   `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req JSONRPCRequest
			err := json.Unmarshal([]byte(tt.input), &req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.method, req.Method)
				assert.Equal(t, "2.0", req.JSONRPC)
			}
		})
	}
}

func TestToolCallResult(t *testing.T) {
	result := ToolCallResult{
		Content: []ContentBlock{
			{Type: "text", Text: "test output"},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed ToolCallResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Len(t, parsed.Content, 1)
	assert.Equal(t, "text", parsed.Content[0].Type)
	assert.Equal(t, "test output", parsed.Content[0].Text)
	assert.False(t, parsed.IsError)
}
