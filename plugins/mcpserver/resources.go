// Package mcpserver implements MCP Server with 9 Tools and 4 Resources.
// Exposes platform capabilities via Model Context Protocol.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"go.uber.org/zap"
)

// ResourceHandler is a function that handles a resource read request.
type ResourceHandler func(ctx context.Context, uri string) (*mcp.ResourceContent, error)

// Resource represents an MCP resource definition with its handler.
type Resource struct {
	URI         string
	Name        string
	Description string
	MimeType    string
	Handler     ResourceHandler
}

// Resources returns all 4 MCP resources.
func Resources() []Resource {
	return []Resource{
		TaskLogResource(),
		TaskTimelineResource(),
		TaskArtifactResource(),
		PlatformStatusResource(),
	}
}

// TaskLogResource creates the task log resource.
// URI: cap://tasks/{taskId}/log
// Returns the execution log for a task.
func TaskLogResource() Resource {
	return Resource{
		URI:         "cap://tasks/{taskId}/log",
		Name:        "Task Execution Log",
		Description: "Returns the execution log for a task, including all agent actions and outputs.",
		MimeType:    "text/plain",
		Handler: func(ctx context.Context, uri string) (*mcp.ResourceContent, error) {
			taskID := extractTaskID(uri, "cap://tasks/", "/log")
			if taskID == "" {
				return &mcp.ResourceContent{
					URI:      uri,
					MimeType: "text/plain",
					Text:     "Error: taskId is required",
				}, nil
			}

			return &mcp.ResourceContent{
				URI:      uri,
				MimeType: "text/plain",
				Text:     fmt.Sprintf("Log for task %s - implement via platform API", taskID),
			}, nil
		},
	}
}

// TaskTimelineResource creates the task timeline resource.
// URI: cap://tasks/{taskId}/timeline
// Returns the decision timeline for a task.
func TaskTimelineResource() Resource {
	return Resource{
		URI:         "cap://tasks/{taskId}/timeline",
		Name:        "Task Decision Timeline",
		Description: "Returns the decision timeline for a task, showing all state transitions and decisions.",
		MimeType:    "application/json",
		Handler: func(ctx context.Context, uri string) (*mcp.ResourceContent, error) {
			taskID := extractTaskID(uri, "cap://tasks/", "/timeline")
			if taskID == "" {
				return &mcp.ResourceContent{
					URI:      uri,
					MimeType: "application/json",
					Text:     `{"error":"taskId is required"}`,
				}, nil
			}

			// Return placeholder timeline - actual implementation would query audit logs
			timeline := map[string]interface{}{
				"taskId":   taskID,
				"timeline": []interface{}{},
				"message":  "Timeline data - implement via platform API",
			}
			data, err := json.Marshal(timeline)
			if err != nil {
				return nil, fmt.Errorf("marshal timeline: %w", err)
			}

			return &mcp.ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(data),
			}, nil
		},
	}
}

// TaskArtifactResource creates the task artifact resource.
// URI: cap://tasks/{taskId}/artifact/{artifactId}
// Returns an artifact produced by a subtask.
func TaskArtifactResource() Resource {
	return Resource{
		URI:         "cap://tasks/{taskId}/artifact/{artifactId}",
		Name:        "Task Artifact",
		Description: "Returns an artifact produced by a subtask, such as analysis reports, diffs, or test results.",
		MimeType:    "application/octet-stream",
		Handler: func(ctx context.Context, uri string) (*mcp.ResourceContent, error) {
			// Extract taskId and artifactId from URI
			// URI format: cap://tasks/{taskId}/artifact/{artifactId}
			parts := strings.Split(uri, "/artifact/")
			if len(parts) != 2 {
				return &mcp.ResourceContent{
					URI:      uri,
					MimeType: "application/octet-stream",
					Text:     "Error: invalid URI format, expected cap://tasks/{taskId}/artifact/{artifactId}",
				}, nil
			}

			// Extract taskId from the first part
			taskID := strings.TrimPrefix(parts[0], "cap://tasks/")
			if taskID == "" {
				return &mcp.ResourceContent{
					URI:      uri,
					MimeType: "application/octet-stream",
					Text:     "Error: taskId is required",
				}, nil
			}

			artifactID := parts[1]
			if artifactID == "" {
				return &mcp.ResourceContent{
					URI:      uri,
					MimeType: "application/octet-stream",
					Text:     "Error: artifactId is required",
				}, nil
			}

			// Return placeholder - actual implementation would fetch from MinIO/storage
			artifact := map[string]interface{}{
				"taskId":     taskID,
				"artifactId": artifactID,
				"message":    "Artifact data - implement via platform API",
			}
			data, err := json.Marshal(artifact)
			if err != nil {
				return nil, fmt.Errorf("marshal artifact: %w", err)
			}

			return &mcp.ResourceContent{
				URI:      uri,
				MimeType: "application/octet-stream",
				Text:     string(data),
			}, nil
		},
	}
}

// PlatformStatusResource creates the platform status resource.
// URI: cap://platform/status
// Returns the platform's real-time status.
func PlatformStatusResource() Resource {
	return Resource{
		URI:         "cap://platform/status",
		Name:        "Platform Status",
		Description: "Returns the platform's real-time status including worker pool, queue, and model availability.",
		MimeType:    "application/json",
		Handler: func(ctx context.Context, uri string) (*mcp.ResourceContent, error) {
			// Return placeholder - actual implementation would fetch from platform
			status := map[string]interface{}{
				"pool": map[string]interface{}{
					"total":       0,
					"idle":        0,
					"busy":        0,
					"maxCapacity": 0,
				},
				"queue": map[string]interface{}{
					"pending":     0,
					"avgWaitTime": 0.0,
				},
				"models":  []interface{}{},
				"uptime":  0,
				"message": "Platform status - implement via platform API",
			}
			data, err := json.Marshal(status)
			if err != nil {
				return nil, fmt.Errorf("marshal status: %w", err)
			}

			return &mcp.ResourceContent{
				URI:      uri,
				MimeType: "application/json",
				Text:     string(data),
			}, nil
		},
	}
}

// extractTaskID extracts the task ID from a URI.
// For "cap://tasks/task123/log" with prefix="cap://tasks/" and suffix="/log"
// returns "task123".
func extractTaskID(uri, prefix, suffix string) string {
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

// ResourceRegistry manages dynamic registration of MCP resources.
type ResourceRegistry struct {
	mu        sync.RWMutex
	resources map[string]Resource
	logger    *zap.Logger
}

// NewResourceRegistry creates a new resource registry.
func NewResourceRegistry(logger *zap.Logger) *ResourceRegistry {
	return &ResourceRegistry{
		resources: make(map[string]Resource),
		logger:    logger,
	}
}

// Register registers a new resource.
func (r *ResourceRegistry) Register(res Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.resources[res.URI] = res
	r.logger.Info("MCP resource registered",
		zap.String("layer", "MCP"),
		zap.String("uri", res.URI),
		zap.Int("total_resources", len(r.resources)),
	)
}

// Get returns a registered resource by URI.
func (r *ResourceRegistry) Get(uri string) (Resource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res, ok := r.resources[uri]
	return res, ok
}

// List returns all registered resources.
func (r *ResourceRegistry) List() []Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resources := make([]Resource, 0, len(r.resources))
	for _, res := range r.resources {
		resources = append(resources, res)
	}
	return resources
}

// Read reads a resource by URI.
func (r *ResourceRegistry) Read(ctx context.Context, uri string) (*mcp.ResourceContent, error) {
	res, ok := r.Get(uri)
	if !ok {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}

	content, err := res.Handler(ctx, uri)
	if err != nil {
		return nil, err
	}

	return content, nil
}
