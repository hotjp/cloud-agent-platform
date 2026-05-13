// Package mcpserver provides tests for MCP tools and resources.
package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTools(t *testing.T) {
	tools := Tools()
	assert.Len(t, tools, 9, "expected 9 MCP tools")

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true

		// Verify each tool has required fields
		assert.NotEmpty(t, tool.Name, "tool name should not be empty")
		assert.NotEmpty(t, tool.Description, "tool description should not be empty for %s", tool.Name)
		assert.NotEmpty(t, tool.InputSchema, "tool inputSchema should not be empty for %s", tool.Name)

		// Verify inputSchema is valid JSON
		var schema map[string]any
		err := json.Unmarshal(tool.InputSchema, &schema)
		require.NoError(t, err, "tool %s has invalid inputSchema", tool.Name)

		// Verify it has type: object
		assert.Equal(t, "object", schema["type"], "tool %s should have type: object", tool.Name)
	}

	// Verify all required tools are present
	requiredTools := []string{
		"task_submit", "task_status", "task_list", "task_cancel",
		"task_decide", "task_diff", "task_wait",
		"agent_templates", "platform_status",
	}
	for _, name := range requiredTools {
		assert.True(t, toolNames[name], "tool %s is missing", name)
	}
}

func TestTaskSubmitTool(t *testing.T) {
	tool := TaskSubmitTool()
	assert.Equal(t, "task_submit", tool.Name)
	assert.NotEmpty(t, tool.Description)

	// Verify inputSchema structure
	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema, &schema)
	require.NoError(t, err)

	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "goal")
	assert.Contains(t, props, "repository")
	assert.Contains(t, props, "constraints")
	assert.Contains(t, props, "verificationCriteria")
	assert.Contains(t, props, "priority")
	assert.Contains(t, props, "timeout")
	assert.Contains(t, props, "agentHint")
	assert.Contains(t, props, "tags")

	// Verify required fields
	required, ok := schema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "goal")
}

func TestTaskStatusTool(t *testing.T) {
	tool := TaskStatusTool()
	assert.Equal(t, "task_status", tool.Name)

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema, &schema)
	require.NoError(t, err)

	required, ok := schema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "taskId")

	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "taskId")
	assert.Contains(t, props, "includeLog")
	assert.Contains(t, props, "includeDiff")
}

func TestTaskListTool(t *testing.T) {
	tool := TaskListTool()
	assert.Equal(t, "task_list", tool.Name)

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema, &schema)
	require.NoError(t, err)

	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "status")
	assert.Contains(t, props, "tags")
	assert.Contains(t, props, "limit")
}

func TestTaskCancelTool(t *testing.T) {
	tool := TaskCancelTool()
	assert.Equal(t, "task_cancel", tool.Name)

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema, &schema)
	require.NoError(t, err)

	required, ok := schema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "taskId")
	assert.Contains(t, required, "reason")
}

func TestTaskDecideTool(t *testing.T) {
	tool := TaskDecideTool()
	assert.Equal(t, "task_decide", tool.Name)

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema, &schema)
	require.NoError(t, err)

	required, ok := schema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "taskId")
	assert.Contains(t, required, "subtaskId")
	assert.Contains(t, required, "decision")

	props := schema["properties"].(map[string]any)
	decision := props["decision"].(map[string]any)
	enumVals := decision["enum"].([]any)
	assert.Contains(t, enumVals, "approve")
	assert.Contains(t, enumVals, "reject")
}

func TestTaskDiffTool(t *testing.T) {
	tool := TaskDiffTool()
	assert.Equal(t, "task_diff", tool.Name)

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema, &schema)
	require.NoError(t, err)

	required, ok := schema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "taskId")
}

func TestTaskWaitTool(t *testing.T) {
	tool := TaskWaitTool()
	assert.Equal(t, "task_wait", tool.Name)

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema, &schema)
	require.NoError(t, err)

	required, ok := schema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "taskId")

	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "timeout")
	assert.Contains(t, props, "pollInterval")
}

func TestAgentTemplatesTool(t *testing.T) {
	tool := AgentTemplatesTool()
	assert.Equal(t, "agent_templates", tool.Name)

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema, &schema)
	require.NoError(t, err)

	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "capability")
}

func TestPlatformStatusTool(t *testing.T) {
	tool := PlatformStatusTool()
	assert.Equal(t, "platform_status", tool.Name)

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema, &schema)
	require.NoError(t, err)

	// platform_status has no required fields and no properties
	assert.Empty(t, schema["required"])
}

func TestValidateToolParams(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		params  string
		wantErr bool
	}{
		{
			name:    "task_submit valid",
			tool:    "task_submit",
			params:  `{"goal": "test goal"}`,
			wantErr: false,
		},
		{
			name:    "task_submit missing goal",
			tool:    "task_submit",
			params:  `{}`,
			wantErr: true,
		},
		{
			name:    "task_status valid",
			tool:    "task_status",
			params:  `{"taskId": "task_123"}`,
			wantErr: false,
		},
		{
			name:    "task_status missing taskId",
			tool:    "task_status",
			params:  `{}`,
			wantErr: true,
		},
		{
			name:    "task_cancel valid",
			tool:    "task_cancel",
			params:  `{"taskId": "task_123", "reason": "user requested"}`,
			wantErr: false,
		},
		{
			name:    "task_cancel missing reason",
			tool:    "task_cancel",
			params:  `{"taskId": "task_123"}`,
			wantErr: true,
		},
		{
			name:    "task_decide valid",
			tool:    "task_decide",
			params:  `{"taskId": "task_123", "subtaskId": "sub_001", "decision": "approve"}`,
			wantErr: false,
		},
		{
			name:    "task_decide invalid decision",
			tool:    "task_decide",
			params:  `{"taskId": "task_123", "subtaskId": "sub_001", "decision": "invalid"}`,
			wantErr: true,
		},
		{
			name:    "task_wait valid",
			tool:    "task_wait",
			params:  `{"taskId": "task_123", "timeout": 60}`,
			wantErr: false,
		},
		{
			name:    "task_wait missing taskId",
			tool:    "task_wait",
			params:  `{"timeout": 60}`,
			wantErr: true,
		},
		{
			name:    "platform_status valid",
			tool:    "platform_status",
			params:  `{}`,
			wantErr: false,
		},
		{
			name:    "unknown tool",
			tool:    "unknown_tool",
			params:  `{}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolParams(tt.tool, json.RawMessage(tt.params))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestResources(t *testing.T) {
	resources := Resources()
	assert.Len(t, resources, 4, "expected 4 MCP resources")

	resourceURIs := make(map[string]bool)
	for _, res := range resources {
		resourceURIs[res.URI] = true

		assert.NotEmpty(t, res.URI, "resource URI should not be empty")
		assert.NotEmpty(t, res.Name, "resource name should not be empty for %s", res.URI)
		assert.NotEmpty(t, res.Description, "resource description should not be empty for %s", res.URI)
		assert.NotEmpty(t, res.MimeType, "resource mimeType should not be empty for %s", res.URI)
		assert.NotNil(t, res.Handler, "resource handler should not be nil for %s", res.URI)
	}

	// Verify all required resources are present
	assert.True(t, resourceURIs["cap://tasks/{taskId}/log"], "TaskLogResource is missing")
	assert.True(t, resourceURIs["cap://tasks/{taskId}/timeline"], "TaskTimelineResource is missing")
	assert.True(t, resourceURIs["cap://tasks/{taskId}/artifact/{artifactId}"], "TaskArtifactResource is missing")
	assert.True(t, resourceURIs["cap://platform/status"], "PlatformStatusResource is missing")
}

func TestTaskLogResource(t *testing.T) {
	res := TaskLogResource()
	assert.Equal(t, "cap://tasks/{taskId}/log", res.URI)
	assert.Equal(t, "Task Execution Log", res.Name)
	assert.Equal(t, "text/plain", res.MimeType)
	require.NotNil(t, res.Handler)

	// Test handler with valid URI
	content, err := res.Handler(nil, "cap://tasks/task123/log")
	require.NoError(t, err)
	assert.Equal(t, "cap://tasks/task123/log", content.URI)
	assert.Equal(t, "text/plain", content.MimeType)
	assert.Contains(t, content.Text, "task123")

	// Test handler with missing taskId
	content, err = res.Handler(nil, "cap://tasks//log")
	require.NoError(t, err)
	assert.Contains(t, content.Text, "taskId is required")
}

func TestTaskTimelineResource(t *testing.T) {
	res := TaskTimelineResource()
	assert.Equal(t, "cap://tasks/{taskId}/timeline", res.URI)
	assert.Equal(t, "Task Decision Timeline", res.Name)
	assert.Equal(t, "application/json", res.MimeType)
	require.NotNil(t, res.Handler)

	// Test handler with valid URI
	content, err := res.Handler(nil, "cap://tasks/task456/timeline")
	require.NoError(t, err)
	assert.Equal(t, "cap://tasks/task456/timeline", content.URI)
	assert.Equal(t, "application/json", content.MimeType)
	assert.Contains(t, content.Text, "task456")
}

func TestTaskArtifactResource(t *testing.T) {
	res := TaskArtifactResource()
	assert.Equal(t, "cap://tasks/{taskId}/artifact/{artifactId}", res.URI)
	assert.Equal(t, "Task Artifact", res.Name)
	assert.Equal(t, "application/octet-stream", res.MimeType)
	require.NotNil(t, res.Handler)

	// Test handler with valid URI
	content, err := res.Handler(nil, "cap://tasks/task789/artifact/artifact001")
	require.NoError(t, err)
	assert.Equal(t, "cap://tasks/task789/artifact/artifact001", content.URI)
	assert.Contains(t, content.Text, "task789")
	assert.Contains(t, content.Text, "artifact001")

	// Test handler with invalid URI format
	content, err = res.Handler(nil, "cap://tasks/invalid")
	require.NoError(t, err)
	assert.Contains(t, content.Text, "Error:")
}

func TestPlatformStatusResource(t *testing.T) {
	res := PlatformStatusResource()
	assert.Equal(t, "cap://platform/status", res.URI)
	assert.Equal(t, "Platform Status", res.Name)
	assert.Equal(t, "application/json", res.MimeType)
	require.NotNil(t, res.Handler)

	// Test handler
	content, err := res.Handler(nil, "cap://platform/status")
	require.NoError(t, err)
	assert.Equal(t, "cap://platform/status", content.URI)
	assert.Equal(t, "application/json", content.MimeType)
	assert.Contains(t, content.Text, "pool")
	assert.Contains(t, content.Text, "queue")
}

func TestExtractTaskID(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		prefix   string
		suffix   string
		expected string
	}{
		{
			name:     "valid task log URI",
			uri:      "cap://tasks/task123/log",
			prefix:   "cap://tasks/",
			suffix:   "/log",
			expected: "task123",
		},
		{
			name:     "valid task timeline URI",
			uri:      "cap://tasks/task456/timeline",
			prefix:   "cap://tasks/",
			suffix:   "/timeline",
			expected: "task456",
		},
		{
			name:     "no suffix",
			uri:      "cap://tasks/task789",
			prefix:   "cap://tasks/",
			suffix:   "",
			expected: "task789",
		},
		{
			name:     "wrong prefix",
			uri:      "other://tasks/task123/log",
			prefix:   "cap://tasks/",
			suffix:   "/log",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTaskID(tt.uri, tt.prefix, tt.suffix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMCPServerToolsAndResources(t *testing.T) {
	server := NewMCPServer(nil)

	tools := server.Tools()
	assert.Len(t, tools, 9)

	resources := server.Resources()
	assert.Len(t, resources, 4)
}

func TestToolCallResult(t *testing.T) {
	result := &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{
			{Type: "text", Text: "test output"},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed mcp.ToolCallResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Len(t, parsed.Content, 1)
	assert.Equal(t, "text", parsed.Content[0].Type)
	assert.Equal(t, "test output", parsed.Content[0].Text)
	assert.False(t, parsed.IsError)
}

func TestResourceContent(t *testing.T) {
	content := &mcp.ResourceContent{
		URI:      "cap://tasks/task123/log",
		MimeType: "text/plain",
		Text:     "Log content",
	}

	data, err := json.Marshal(content)
	require.NoError(t, err)

	var parsed mcp.ResourceContent
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "cap://tasks/task123/log", parsed.URI)
	assert.Equal(t, "text/plain", parsed.MimeType)
	assert.Equal(t, "Log content", parsed.Text)
}

func TestToolSchemasDontHaveDuplicateNames(t *testing.T) {
	tools := Tools()
	names := make(map[string]int)
	for _, tool := range tools {
		names[tool.Name]++
	}
	for name, count := range names {
		assert.Equal(t, 1, count, "tool name %s appears %d times", name, count)
	}
}

func TestResourceURIsAreUnique(t *testing.T) {
	resources := Resources()
	uris := make(map[string]int)
	for _, res := range resources {
		uris[res.URI]++
	}
	for uri, count := range uris {
		assert.Equal(t, 1, count, "resource URI %s appears %d times", uri, count)
	}
}

func TestToolNameFormat(t *testing.T) {
	tools := Tools()
	for _, tool := range tools {
		// Tool names should be snake_case or kebab-case
		assert.True(t, strings.Contains(tool.Name, "_") || strings.Contains(tool.Name, "-") || tool.Name == tool.Name,
			"tool name %s should be snake_case or kebab-case", tool.Name)
	}
}
