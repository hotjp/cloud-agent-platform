// Package mcp provides tests for the MCP server implementation.
package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRegistry(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	registry := NewRegistry(logger)

	t.Run("register and get tool", func(t *testing.T) {
		handler := func(ctx context.Context, params json.RawMessage) (*ToolCallResult, error) {
			return &ToolCallResult{
				Content: []ContentBlock{{Type: "text", Text: "test"}},
			}, nil
		}

		registry.RegisterTool(
			"test_tool",
			"A test tool",
			json.RawMessage(`{"type":"object"}`),
			handler,
		)

		tool, ok := registry.GetTool("test_tool")
		assert.True(t, ok)
		assert.Equal(t, "test_tool", tool.Name)
		assert.Equal(t, "A test tool", tool.Description)
	})

	t.Run("register and get resource", func(t *testing.T) {
		handler := func(ctx context.Context, uri string) (*ResourceContent, error) {
			return &ResourceContent{
				URI:      uri,
				MimeType: "text/plain",
				Text:     "test content",
			}, nil
		}

		registry.RegisterResource(
			"cap://test/resource",
			"Test Resource",
			"A test resource",
			"text/plain",
			handler,
		)

		resource, ok := registry.GetResource("cap://test/resource")
		assert.True(t, ok)
		assert.Equal(t, "cap://test/resource", resource.URI)
		assert.Equal(t, "Test Resource", resource.Name)
	})

	t.Run("list tools", func(t *testing.T) {
		tools := registry.ListTools()
		assert.NotEmpty(t, tools)
		// Should contain our registered tool
		found := false
		for _, t := range tools {
			if t.Name == "test_tool" {
				found = true
				break
			}
		}
		assert.True(t, found)
	})

	t.Run("list resources", func(t *testing.T) {
		resources := registry.ListResources()
		assert.NotEmpty(t, resources)
		// Should contain our registered resource
		found := false
		for _, r := range resources {
			if r.URI == "cap://test/resource" {
				found = true
				break
			}
		}
		assert.True(t, found)
	})
}

func TestResourceTypes(t *testing.T) {
	t.Run("Resource struct", func(t *testing.T) {
		resource := Resource{
			URI:         "cap://tasks/task123/log",
			Name:        "Task Log",
			Description: "Log for a task",
			MimeType:    "text/plain",
		}

		data, err := json.Marshal(resource)
		require.NoError(t, err)

		var parsed Resource
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "cap://tasks/task123/log", parsed.URI)
		assert.Equal(t, "Task Log", parsed.Name)
		assert.Equal(t, "text/plain", parsed.MimeType)
	})

	t.Run("ResourcesListResult", func(t *testing.T) {
		result := ResourcesListResult{
			Resources: []Resource{
				{URI: "cap://tasks/task1/log", Name: "Log 1"},
				{URI: "cap://tasks/task2/log", Name: "Log 2"},
			},
		}

		data, err := json.Marshal(result)
		require.NoError(t, err)

		var parsed ResourcesListResult
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Len(t, parsed.Resources, 2)
	})

	t.Run("ResourceReadResult", func(t *testing.T) {
		result := ResourceReadResult{
			Contents: []ResourceContent{
				{URI: "cap://tasks/task1/log", MimeType: "text/plain", Text: "log content"},
			},
		}

		data, err := json.Marshal(result)
		require.NoError(t, err)

		var parsed ResourceReadResult
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Len(t, parsed.Contents, 1)
		assert.Equal(t, "log content", parsed.Contents[0].Text)
	})
}

func TestResourceReadParams(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantURI string
		wantErr bool
	}{
		{
			name:    "valid with URI",
			input:   `{"uri":"cap://tasks/task123/log"}`,
			wantURI: "cap://tasks/task123/log",
			wantErr: false,
		},
		{
			name:    "valid with cursor",
			input:   `{"uri":"cap://tasks/task123/log","cursor":"abc123"}`,
			wantURI: "cap://tasks/task123/log",
			wantErr: false,
		},
		{
			name:    "missing URI",
			input:   `{}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var params ResourceReadParams
			err := json.Unmarshal([]byte(tt.input), &params)
			require.NoError(t, err)

			if tt.wantErr {
				assert.Empty(t, params.URI)
			} else {
				assert.Equal(t, tt.wantURI, params.URI)
			}
		})
	}
}

func TestServerCapabilities(t *testing.T) {
	t.Run("with resources", func(t *testing.T) {
		caps := ServerCapabilities{
			Tools: &struct{}{},
			Resources: &ResourcesCapability{
				Subscribe: true,
				ListHint:  true,
			},
		}

		data, err := json.Marshal(caps)
		require.NoError(t, err)

		var parsed ServerCapabilities
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.NotNil(t, parsed.Tools)
		assert.NotNil(t, parsed.Resources)
		assert.True(t, parsed.Resources.Subscribe)
		assert.True(t, parsed.Resources.ListHint)
	})
}

func TestSSEEvent(t *testing.T) {
	t.Run("basic event", func(t *testing.T) {
		event := SSEEvent{
			Event: "message",
			Data:  json.RawMessage(`{"key":"value"}`),
			ID:    "123",
		}

		data, err := json.Marshal(event)
		require.NoError(t, err)

		var parsed SSEEvent
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "message", parsed.Event)
		assert.Equal(t, "123", parsed.ID)
	})

	t.Run("event without ID", func(t *testing.T) {
		event := SSEEvent{
			Event: "ping",
			Data:  json.RawMessage(`{}`),
		}

		data, err := json.Marshal(event)
		require.NoError(t, err)

		var parsed SSEEvent
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "ping", parsed.Event)
		assert.Empty(t, parsed.ID)
	})
}

func TestGlobalRegistry(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	t.Run("init and get registry", func(t *testing.T) {
		InitRegistry(logger)
		assert.NotNil(t, GetRegistry())
	})

	t.Run("default resources registered", func(t *testing.T) {
		reg := GetRegistry()
		require.NotNil(t, reg)

		resources := reg.ListResources()
		assert.NotEmpty(t, resources)

		// Check for default resources
		resourceURIs := make(map[string]bool)
		for _, r := range resources {
			resourceURIs[r.URI] = true
		}

		assert.True(t, resourceURIs["cap://tasks/{taskId}/log"])
		assert.True(t, resourceURIs["cap://tasks/{taskId}/timeline"])
		assert.True(t, resourceURIs["cap://templates/{templateId}/prompt"])
	})
}

func TestExtractPathParam(t *testing.T) {
	tests := []struct {
		name   string
		uri    string
		prefix string
		suffix string
		want   string
	}{
		{
			name:   "extract taskId from tasks URI",
			uri:    "cap://tasks/task123",
			prefix: "cap://tasks/",
			suffix: "/",
			want:   "task123",
		},
		{
			name:   "extract with no suffix",
			uri:    "cap://tasks/task123",
			prefix: "cap://tasks/",
			suffix: "",
			want:   "task123",
		},
		{
			name:   "extract from agents URI",
			uri:    "cap://agents",
			prefix: "cap://",
			suffix: "",
			want:   "agents",
		},
		{
			name:   "extract from sessions URI",
			uri:    "cap://sessions",
			prefix: "cap://",
			suffix: "",
			want:   "sessions",
		},
		{
			name:   "empty prefix",
			uri:    "task123",
			prefix: "",
			suffix: "",
			want:   "task123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPathParam(tt.uri, tt.prefix, tt.suffix)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPlatformConfigResponse(t *testing.T) {
	t.Run("marshal config response", func(t *testing.T) {
		config := PlatformConfigResponse{
			Server: ServerConfig{
				Port:        "8080",
				MetricsPort: "9090",
			},
			Sandbox: SandboxConfig{
				Backend: "docker",
			},
			RateLimit: RateLimitConfig{
				Enabled: true,
				QPS:     100,
				Burst:   200,
			},
		}

		data, err := json.Marshal(config)
		require.NoError(t, err)

		var parsed PlatformConfigResponse
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "8080", parsed.Server.Port)
		assert.Equal(t, "docker", parsed.Sandbox.Backend)
		assert.True(t, parsed.RateLimit.Enabled)
		assert.Equal(t, 100, parsed.RateLimit.QPS)
	})
}

func TestNewResourceDefinitions(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	registry := NewRegistry(logger)

	// Register a mock resource
	registry.RegisterResource(
		"cap://test/resource",
		"Test Resource",
		"A test resource",
		"application/json",
		func(ctx context.Context, uri string) (*ResourceContent, error) {
			return &ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     `{"test":"data"}`,
			}, nil
		},
	)

	t.Run("read registered resource", func(t *testing.T) {
		content, err := registry.ReadResource(context.Background(), "cap://test/resource")
		require.NoError(t, err)
		assert.Equal(t, "cap://test/resource", content.URI)
		assert.Equal(t, "application/json", content.MimeType)
		assert.Equal(t, `{"test":"data"}`, content.Text)
	})

	t.Run("read non-existent resource", func(t *testing.T) {
		_, err := registry.ReadResource(context.Background(), "cap://non/existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestResourceURITemplates(t *testing.T) {
	t.Run("task resource URI template", func(t *testing.T) {
		// Verify task ID extraction pattern
		uri := "cap://tasks/task_abc123"
		taskID := extractPathParam(uri, "cap://tasks/", "/")
		assert.Equal(t, "task_abc123", taskID)
	})

	t.Run("agents resource URI", func(t *testing.T) {
		uri := "cap://agents"
		val := extractPathParam(uri, "cap://", "")
		assert.Equal(t, "agents", val)
	})

	t.Run("sessions resource URI", func(t *testing.T) {
		uri := "cap://sessions"
		val := extractPathParam(uri, "cap://", "")
		assert.Equal(t, "sessions", val)
	})

	t.Run("config resource URI", func(t *testing.T) {
		uri := "cap://config"
		val := extractPathParam(uri, "cap://", "")
		assert.Equal(t, "config", val)
	})
}
