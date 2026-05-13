// Package mcp implements the MCP (Model Context Protocol) server for the Cloud Agent Platform.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// ToolHandler is a function that handles a tool call.
type ToolHandler func(ctx context.Context, params json.RawMessage) (*ToolCallResult, error)

// ResourceHandler is a function that handles a resource read request.
type ResourceHandler func(ctx context.Context, uri string) (*ResourceContent, error)

// ToolDef represents a dynamically registered tool.
type ToolDef struct {
	Tool
	Handler ToolHandler
}

// ResourceDef represents a dynamically registered resource.
type ResourceDef struct {
	Resource
	Handler ResourceHandler
}

// Registry manages dynamic registration of MCP tools and resources.
type Registry struct {
	mu       sync.RWMutex
	tools    map[string]ToolDef
	resources map[string]ResourceDef
	logger   *zap.Logger
	client   *PlatformClient
}

// NewRegistry creates a new registry.
func NewRegistry(logger *zap.Logger) *Registry {
	return &Registry{
		tools:    make(map[string]ToolDef),
		resources: make(map[string]ResourceDef),
		logger:   logger,
	}
}

// SetClient sets the PlatformClient for API-backed resources.
func (r *Registry) SetClient(client *PlatformClient) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.client = client
}

// getClient returns the PlatformClient (internal use).
func (r *Registry) getClient() *PlatformClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}

// RegisterTool registers a new tool with its handler.
func (r *Registry) RegisterTool(name, description string, inputSchema json.RawMessage, handler ToolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[name] = ToolDef{
		Tool: Tool{
			Name:        name,
			Description: description,
			InputSchema: inputSchema,
		},
		Handler: handler,
	}

	r.logger.Info("MCP tool registered",
		zap.String("layer", "MCP"),
		zap.String("tool", name),
		zap.Int("total_tools", len(r.tools)),
	)
}

// RegisterResource registers a new resource with its handler.
func (r *Registry) RegisterResource(uri, name, description, mimeType string, handler ResourceHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.resources[uri] = ResourceDef{
		Resource: Resource{
			URI:         uri,
			Name:        name,
			Description: description,
			MimeType:    mimeType,
		},
		Handler: handler,
	}

	r.logger.Info("MCP resource registered",
		zap.String("layer", "MCP"),
		zap.String("uri", uri),
		zap.Int("total_resources", len(r.resources)),
	)
}

// GetTool returns a registered tool by name.
func (r *Registry) GetTool(name string) (ToolDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

// GetResource returns a registered resource by URI.
func (r *Registry) GetResource(uri string) (ResourceDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resource, ok := r.resources[uri]
	return resource, ok
}

// ListTools returns all registered tools.
func (r *Registry) ListTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t.Tool)
	}
	return tools
}

// ListResources returns all registered resources.
func (r *Registry) ListResources() []Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resources := make([]Resource, 0, len(r.resources))
	for _, res := range r.resources {
		resources = append(resources, res.Resource)
	}
	return resources
}

// ReadResource reads a resource by URI.
func (r *Registry) ReadResource(ctx context.Context, uri string) (*ResourceContent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resource, ok := r.resources[uri]
	if !ok {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}

	content, err := resource.Handler(ctx, uri)
	if err != nil {
		return nil, err
	}

	return content, nil
}

// globalRegistry is the global registry instance.
var globalRegistry *Registry

// InitRegistry initializes the global registry.
func InitRegistry(logger *zap.Logger) {
	globalRegistry = NewRegistry(logger)
	registerDefaultResources(globalRegistry)
}

// GetRegistry returns the global registry.
func GetRegistry() *Registry {
	return globalRegistry
}

// registerDefaultResources registers the default MCP resources.
func registerDefaultResources(r *Registry) {
	// Task log resource: cap://tasks/{taskId}/log
	r.RegisterResource(
		"cap://tasks/{taskId}/log",
		"Task Execution Log",
		"Returns the execution log for a task",
		"text/plain",
		func(ctx context.Context, uri string) (*ResourceContent, error) {
			// Extract taskId from URI
			var taskID string
			if _, err := fmt.Sscanf(uri, "cap://tasks/%s/log", &taskID); err != nil {
				taskID = extractTaskID(uri)
			}

			return &ResourceContent{
				URI:      uri,
				MimeType: "text/plain",
				Text:     fmt.Sprintf("Log for task %s - implement via platform API", taskID),
			}, nil
		},
	)

	// Task timeline resource: cap://tasks/{taskId}/timeline
	r.RegisterResource(
		"cap://tasks/{taskId}/timeline",
		"Task Decision Timeline",
		"Returns the decision timeline for a task",
		"application/json",
		func(ctx context.Context, uri string) (*ResourceContent, error) {
			taskID := extractTaskID(uri)
			return &ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     fmt.Sprintf(`{"taskId":"%s","timeline":[]}`, taskID),
			}, nil
		},
	)

	// Template prompt resource: cap://templates/{templateId}/prompt
	r.RegisterResource(
		"cap://templates/{templateId}/prompt",
		"Agent Template Prompt",
		"Returns the system prompt for an agent template",
		"text/plain",
		func(ctx context.Context, uri string) (*ResourceContent, error) {
			var templateID string
			if _, err := fmt.Sscanf(uri, "cap://templates/%s/prompt", &templateID); err != nil {
				templateID = "default"
			}
			return &ResourceContent{
				URI:      uri,
				MimeType: "text/plain",
				Text:     fmt.Sprintf("System prompt for template %s", templateID),
			}, nil
		},
	)
}

// extractTaskID extracts the task ID from a URI.
func extractTaskID(uri string) string {
	// Simple extraction - in production would use proper parsing
	if len(uri) > 20 {
		return uri[12:20] // rough extraction for cap://tasks/xxxx/log
	}
	return "unknown"
}

// globalExecutor is the global tool executor instance.
var globalExecutor *ToolExecutor

// InitExecutor initializes the global executor.
func InitExecutor(client *PlatformClient, logger *zap.Logger) {
	globalExecutor = NewToolExecutor(client, logger)
	// Register API-backed resources after client is available
	RegisterAPIResources(client, logger)
}

// GetExecutor returns the global executor.
func GetExecutor() *ToolExecutor {
	return globalExecutor
}

// RegisterAPIResources registers the 4 MCP Resources backed by REST API calls.
// These are registered separately because they require the PlatformClient.
func RegisterAPIResources(client *PlatformClient, logger *zap.Logger) {
	if globalRegistry == nil {
		logger.Warn("global registry not initialized, skipping API resources registration")
		return
	}

	// Set the client in the registry for handlers to use
	globalRegistry.SetClient(client)

	// cap://tasks/{taskId} - Get task details
	globalRegistry.RegisterResource(
		"cap://tasks/{taskId}",
		"Task Details",
		"Returns detailed information about a task including subtasks and status history",
		"application/json",
		func(ctx context.Context, uri string) (*ResourceContent, error) {
			// Extract taskId from URI
			taskID := extractPathParam(uri, "cap://tasks/", "/")
			if taskID == "" {
				return &ResourceContent{
					URI:      uri,
					MimeType: "application/json",
					Text:     `{"error":"taskId is required"}`,
				}, nil
			}

			logger.Debug("reading task resource",
				zap.String("layer", "MCP"),
				zap.String("task_id", taskID),
			)

			task, err := client.GetTask(ctx, taskID)
			if err != nil {
				logger.Error("failed to get task",
					zap.String("layer", "MCP"),
					zap.String("task_id", taskID),
					zap.Error(err),
				)
				return &ResourceContent{
					URI:      uri,
					MimeType: "application/json",
					Text:     fmt.Sprintf(`{"error":"failed to get task: %v"}`, err),
				}, nil
			}

			data, err := json.Marshal(task)
			if err != nil {
				return nil, fmt.Errorf("marshal task: %w", err)
			}

			return &ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(data),
			}, nil
		},
	)

	// cap://agents - List available agents
	globalRegistry.RegisterResource(
		"cap://agents",
		"Available Agents",
		"Returns a list of available agents with their roles and capabilities",
		"application/json",
		func(ctx context.Context, uri string) (*ResourceContent, error) {
			logger.Debug("reading agents resource",
				zap.String("layer", "MCP"),
			)

			agents, err := client.ListAgentTemplates(ctx)
			if err != nil {
				logger.Error("failed to list agents",
					zap.String("layer", "MCP"),
					zap.Error(err),
				)
				return &ResourceContent{
					URI:      uri,
					MimeType: "application/json",
					Text:     fmt.Sprintf(`{"error":"failed to list agents: %v"}`, err),
				}, nil
			}

			data, err := json.Marshal(agents)
			if err != nil {
				return nil, fmt.Errorf("marshal agents: %w", err)
			}

			return &ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(data),
			}, nil
		},
	)

	// cap://sessions - List sessions
	globalRegistry.RegisterResource(
		"cap://sessions",
		"Session List",
		"Returns a list of active sessions with their status",
		"application/json",
		func(ctx context.Context, uri string) (*ResourceContent, error) {
			logger.Debug("reading sessions resource",
				zap.String("layer", "MCP"),
			)

			sessions, err := client.ListSessions(ctx)
			if err != nil {
				logger.Error("failed to list sessions",
					zap.String("layer", "MCP"),
					zap.Error(err),
				)
				return &ResourceContent{
					URI:      uri,
					MimeType: "application/json",
					Text:     fmt.Sprintf(`{"error":"failed to list sessions: %v"}`, err),
				}, nil
			}

			data, err := json.Marshal(sessions)
			if err != nil {
				return nil, fmt.Errorf("marshal sessions: %w", err)
			}

			return &ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(data),
			}, nil
		},
	)

	// cap://config - Platform configuration (non-sensitive)
	globalRegistry.RegisterResource(
		"cap://config",
		"Platform Configuration",
		"Returns non-sensitive platform configuration",
		"application/json",
		func(ctx context.Context, uri string) (*ResourceContent, error) {
			logger.Debug("reading config resource",
				zap.String("layer", "MCP"),
			)

			status, err := client.GetPlatformStatus(ctx)
			if err != nil {
				logger.Error("failed to get platform status",
					zap.String("layer", "MCP"),
					zap.Error(err),
				)
				return &ResourceContent{
					URI:      uri,
					MimeType: "application/json",
					Text:     fmt.Sprintf(`{"error":"failed to get platform status: %v"}`, err),
				}, nil
			}

			// Build non-sensitive config response
			config := PlatformConfigResponse{
				Server: ServerConfig{
					Port:        "8080", // These are placeholder values from config
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
				Platform: PlatformStatusResponse{
					Pool:    status.Pool,
					Queue:   status.Queue,
					Models:  status.Models,
					Uptime:  status.Uptime,
				},
			}

			data, err := json.Marshal(config)
			if err != nil {
				return nil, fmt.Errorf("marshal config: %w", err)
			}

			return &ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(data),
			}, nil
		},
	)

	logger.Info("MCP API resources registered",
		zap.String("layer", "MCP"),
		zap.Int("total_resources", len(globalRegistry.ListResources())),
	)
}

// PlatformConfigResponse represents non-sensitive platform configuration.
type PlatformConfigResponse struct {
	Server    ServerConfig         `json:"server"`
	Sandbox   SandboxConfig        `json:"sandbox"`
	RateLimit RateLimitConfig      `json:"rate_limit"`
	Platform  PlatformStatusResponse `json:"platform"`
}

// ServerConfig represents non-sensitive server configuration.
type ServerConfig struct {
	Port        string `json:"port"`
	MetricsPort string `json:"metricsPort"`
}

// SandboxConfig represents sandbox configuration.
type SandboxConfig struct {
	Backend string `json:"backend"`
}

// RateLimitConfig represents rate limit configuration.
type RateLimitConfig struct {
	Enabled bool `json:"enabled"`
	QPS     int  `json:"qps"`
	Burst   int  `json:"burst"`
}

// extractPathParam extracts a path parameter from a URI.
// For "cap://tasks/task123" with prefix="cap://tasks/" it returns "task123".
// For "cap://tasks/task123/log" with prefix="cap://tasks/" it returns "task123".
func extractPathParam(uri, prefix, suffix string) string {
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	remainder := uri[len(prefix):]
	if suffix == "" {
		return remainder
	}
	// Find the suffix and extract everything before it
	if idx := strings.Index(remainder, suffix); idx >= 0 {
		return remainder[:idx]
	}
	return remainder
}
