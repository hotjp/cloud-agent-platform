// Package mcp provides tests for the OpenClaw registry integration.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestOpenClawConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		config := DiscoverConfig()
		assert.False(t, config.Enabled)
		assert.Equal(t, "http://localhost:8081", config.APIURL)
		assert.Equal(t, "18080", config.ServerPort)
		assert.Empty(t, config.AuthToken)
	})
}

func TestGetEnv(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		val := getEnv("NONEXISTENT_ENV_VAR_12345", "default")
		assert.Equal(t, "default", val)
	})

	t.Run("returns value when env is set", func(t *testing.T) {
		val := getEnv("PATH", "default")
		assert.NotEqual(t, "default", val)
	})
}

func TestOpenClawRegistry_Register(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	t.Run("disabled config skips registration", func(t *testing.T) {
		config := OpenClawConfig{
			Enabled: false,
			APIURL:  "http://localhost:8081",
		}
		registry := NewOpenClawRegistry(config, logger)

		err := registry.Register(context.Background())
		assert.NoError(t, err)
		assert.False(t, registry.IsRegistered())
	})

	t.Run("successful registration when OpenClaw is available", func(t *testing.T) {
		// Create mock server
		mock := NewMockOpenClawServer()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/mcp/servers" && r.Method == http.MethodPost {
				mock.HandleRegister(w, r)
			} else if r.URL.Path == "/api/v1/health" && r.Method == http.MethodGet {
				mock.HandleHealth(w, r)
			}
		}))
		defer server.Close()

		config := OpenClawConfig{
			Enabled:    true,
			APIURL:     server.URL,
			ServerPort: "18080",
		}
		registry := NewOpenClawRegistry(config, logger)

		err := registry.Register(context.Background())
		assert.NoError(t, err)
		assert.True(t, registry.IsRegistered())
	})

	t.Run("registration does not fail when OpenClaw is unavailable", func(t *testing.T) {
		config := OpenClawConfig{
			Enabled: true,
			APIURL:  "http://localhost:9999", // Non-existent server
		}
		registry := NewOpenClawRegistry(config, logger)

		// Should not error, just log warning
		err := registry.Register(context.Background())
		assert.NoError(t, err)
		assert.False(t, registry.IsRegistered())
	})

	t.Run("duplicate registration is idempotent", func(t *testing.T) {
		mock := NewMockOpenClawServer()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/mcp/servers" && r.Method == http.MethodPost {
				mock.HandleRegister(w, r)
			}
		}))
		defer server.Close()

		config := OpenClawConfig{
			Enabled:    true,
			APIURL:     server.URL,
			ServerPort: "18080",
		}
		registry := NewOpenClawRegistry(config, logger)

		// First registration
		err := registry.Register(context.Background())
		require.NoError(t, err)
		require.True(t, registry.IsRegistered())

		// Second registration should be idempotent
		err = registry.Register(context.Background())
		assert.NoError(t, err)
		assert.True(t, registry.IsRegistered())
	})
}

func TestOpenClawRegistry_Unregister(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	t.Run("unregister when not registered is no-op", func(t *testing.T) {
		config := OpenClawConfig{
			Enabled: true,
			APIURL:  "http://localhost:8081",
		}
		registry := NewOpenClawRegistry(config, logger)

		err := registry.Unregister(context.Background())
		assert.NoError(t, err)
		assert.False(t, registry.IsRegistered())
	})

	t.Run("successful unregistration", func(t *testing.T) {
		mock := NewMockOpenClawServer()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/api/v1/mcp/servers" && r.Method == http.MethodPost:
				mock.HandleRegister(w, r)
			case r.URL.Path == "/api/v1/mcp/servers/cloud-agent-platform" && r.Method == http.MethodDelete:
				mock.HandleUnregister(w, r)
			}
		}))
		defer server.Close()

		config := OpenClawConfig{
			Enabled:    true,
			APIURL:     server.URL,
			ServerPort: "18080",
		}
		registry := NewOpenClawRegistry(config, logger)

		// First register
		err := registry.Register(context.Background())
		require.NoError(t, err)
		require.True(t, registry.IsRegistered())

		// Then unregister
		err = registry.Unregister(context.Background())
		assert.NoError(t, err)
		assert.False(t, registry.IsRegistered())
	})
}

func TestOpenClawRegistry_HealthCheck(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	t.Run("disabled config returns error", func(t *testing.T) {
		config := OpenClawConfig{
			Enabled: false,
		}
		registry := NewOpenClawRegistry(config, logger)

		err := registry.HealthCheck(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
	})

	t.Run("health check succeeds when OpenClaw is healthy", func(t *testing.T) {
		mock := NewMockOpenClawServer()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mock.HandleHealth(w, r)
		}))
		defer server.Close()

		config := OpenClawConfig{
			Enabled: true,
			APIURL:  server.URL,
		}
		registry := NewOpenClawRegistry(config, logger)

		err := registry.HealthCheck(context.Background())
		assert.NoError(t, err)
	})

	t.Run("health check fails when OpenClaw is unhealthy", func(t *testing.T) {
		mock := &MockOpenClawServer{HealthStatus: http.StatusServiceUnavailable}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mock.HandleHealth(w, r)
		}))
		defer server.Close()

		config := OpenClawConfig{
			Enabled: true,
			APIURL:  server.URL,
		}
		registry := NewOpenClawRegistry(config, logger)

		err := registry.HealthCheck(context.Background())
		assert.Error(t, err)
	})
}

func TestOpenClawRegistry_GetConfig(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	config := OpenClawConfig{
		Enabled:    true,
		APIURL:     "http://localhost:8081",
		ServerPort: "18080",
		AuthToken:  "test-token",
	}
	registry := NewOpenClawRegistry(config, logger)

	returnedConfig := registry.GetConfig()
	assert.Equal(t, config.Enabled, returnedConfig.Enabled)
	assert.Equal(t, config.APIURL, returnedConfig.APIURL)
	assert.Equal(t, config.ServerPort, returnedConfig.ServerPort)
	assert.Equal(t, config.AuthToken, returnedConfig.AuthToken)
}

func TestMockOpenClawServer(t *testing.T) {
	mock := NewMockOpenClawServer()

	t.Run("HandleRegister", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":    "test-server",
			"type":    "mcp",
			"version": "1.0.0",
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/servers", bytes.NewReader(body))
		w := httptest.NewRecorder()

		mock.HandleRegister(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.True(t, mock.RegisteredServers["test-server"])
	})

	t.Run("HandleUnregister", func(t *testing.T) {
		mock.RegisteredServers["cloud-agent-platform"] = true

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/mcp/servers/cloud-agent-platform", nil)
		w := httptest.NewRecorder()

		mock.HandleUnregister(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.False(t, mock.RegisteredServers["cloud-agent-platform"])
	})

	t.Run("HandleHealth - healthy", func(t *testing.T) {
		mock.HealthStatus = http.StatusOK

		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		w := httptest.NewRecorder()

		mock.HandleHealth(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("HandleHealth - unhealthy", func(t *testing.T) {
		mock.HealthStatus = http.StatusServiceUnavailable

		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		w := httptest.NewRecorder()

		mock.HandleHealth(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}