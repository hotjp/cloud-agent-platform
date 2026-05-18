// Package mcp implements the MCP (Model Context Protocol) server for the Cloud Agent Platform.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// OpenClawConfig holds the configuration for OpenClaw integration.
type OpenClawConfig struct {
	Enabled    bool
	APIURL     string // OpenClaw API URL (e.g., http://localhost:8081)
	ServerPort string // CAP server port for MCP endpoint
	AuthToken  string // Optional auth token for OpenClaw API
}

// OpenClawRegistry handles registration of MCP server with OpenClaw.
type OpenClawRegistry struct {
	config    OpenClawConfig
	logger    *zap.Logger
	client    *http.Client
	mu        sync.RWMutex
	registered bool
}

// NewOpenClawRegistry creates a new OpenClaw registry.
func NewOpenClawRegistry(config OpenClawConfig, logger *zap.Logger) *OpenClawRegistry {
	return &OpenClawRegistry{
		config: config,
		logger: logger,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// DiscoverConfig attempts to discover OpenClaw configuration from environment variables.
func DiscoverConfig() OpenClawConfig {
	config := OpenClawConfig{
		Enabled:    false,
		APIURL:     getEnv("OPENCLAW_API_URL", "http://localhost:8081"),
		ServerPort: getEnv("CAP_SERVER_PORT", "18080"),
		AuthToken:  getEnv("OPENCLAW_AUTH_TOKEN", ""),
	}

	// Enable if OPENCLAW_ENABLED is set or if we can reach OpenClaw
	if getEnv("OPENCLAW_ENABLED", "") == "true" {
		config.Enabled = true
	}

	return config
}

// getEnv is a helper to get environment variables with default values.
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// Register registers the MCP server endpoint with OpenClaw.
func (r *OpenClawRegistry) Register(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.config.Enabled {
		r.logger.Debug("OpenClaw integration disabled, skipping registration",
			zap.String("layer", "MCP"),
		)
		return nil
	}

	if r.registered {
		r.logger.Info("MCP server already registered with OpenClaw",
			zap.String("layer", "MCP"),
		)
		return nil
	}

	mcpEndpoint := fmt.Sprintf("http://localhost:%s/mcp", r.config.ServerPort)

	payload := map[string]interface{}{
		"name":        "cloud-agent-platform",
		"type":        "mcp",
		"version":     "1.0.0",
		"endpoint":    mcpEndpoint,
		"transport":   "http",
		"tools":       GetToolDefinitions(),
		"resources":   GetResourceDefinitions(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal registration payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/mcp/servers", r.config.APIURL)
	r.logger.Info("registering MCP server with OpenClaw",
		zap.String("layer", "MCP"),
		zap.String("url", url),
		zap.String("endpoint", mcpEndpoint),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create registration request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if r.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.config.AuthToken)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		// If OpenClaw is not reachable, log warning but don't fail
		r.logger.Warn("failed to register with OpenClaw (OpenClaw may not be running)",
			zap.String("layer", "MCP"),
			zap.String("url", url),
			zap.Error(err),
		)
		return nil // Don't fail startup, just log warning
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		r.logger.Warn("OpenClaw registration returned non-success status",
			zap.String("layer", "MCP"),
			zap.Int("status", resp.StatusCode),
		)
		return nil
	}

	r.registered = true
	r.logger.Info("MCP server successfully registered with OpenClaw",
		zap.String("layer", "MCP"),
		zap.String("endpoint", mcpEndpoint),
	)

	return nil
}

// Unregister removes the MCP server registration from OpenClaw.
func (r *OpenClawRegistry) Unregister(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.registered {
		return nil
	}

	url := fmt.Sprintf("%s/api/v1/mcp/servers/cloud-agent-platform", r.config.APIURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create unregistration request: %w", err)
	}

	if r.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.config.AuthToken)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.Warn("failed to unregister from OpenClaw",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		return nil
	}
	defer resp.Body.Close()

	r.registered = false
	r.logger.Info("MCP server unregistered from OpenClaw",
		zap.String("layer", "MCP"),
	)

	return nil
}

// HealthCheck performs a health check on the OpenClaw integration.
func (r *OpenClawRegistry) HealthCheck(ctx context.Context) error {
	if !r.config.Enabled {
		return fmt.Errorf("OpenClaw integration is disabled")
	}

	url := fmt.Sprintf("%s/api/v1/health", r.config.APIURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create health check request: %w", err)
	}

	if r.config.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.config.AuthToken)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// IsRegistered returns whether the MCP server is registered with OpenClaw.
func (r *OpenClawRegistry) IsRegistered() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registered
}

// GetConfig returns the current OpenClaw configuration.
func (r *OpenClawRegistry) GetConfig() OpenClawConfig {
	return r.config
}

// MockOpenClawServer provides a mock OpenClaw server for testing.
type MockOpenClawServer struct {
	RegisteredServers map[string]bool
	HealthStatus      int
}

// NewMockOpenClawServer creates a mock server for testing.
func NewMockOpenClawServer() *MockOpenClawServer {
	return &MockOpenClawServer{
		RegisteredServers: make(map[string]bool),
		HealthStatus:     http.StatusOK,
	}
}

// HandleRegister handles MCP server registration requests.
func (m *MockOpenClawServer) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	name, ok := payload["name"].(string)
	if !ok {
		http.Error(w, "Missing server name", http.StatusBadRequest)
		return
	}

	m.RegisteredServers[name] = true
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

// HandleUnregister handles MCP server unregistration requests.
func (m *MockOpenClawServer) HandleUnregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	delete(m.RegisteredServers, "cloud-agent-platform")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "unregistered"})
}

// HandleHealth handles health check requests.
func (m *MockOpenClawServer) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(m.HealthStatus)
	if m.HealthStatus == http.StatusOK {
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}
}