// Package config implements configuration management using koanf.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
)

// DefaultConfig returns a Config struct with default values.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         8080,
			MetricsPort:  9090,
			PprofPort:    6060,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Database: DatabaseConfig{
			MaxOpen:     25,
			MaxIdle:     10,
			MaxLifetime: 5 * time.Minute,
		},
		Redis: RedisConfig{
			Addr: "localhost:6379",
			DB:   0,
		},
		Authz: AuthzConfig{
			JWTSecret:    "",
			APIKeyHeader: "X-API-Key",
			CacheTTL:     5 * time.Minute,
		},
		Telemetry: TelemetryConfig{
			ServiceName: "cloud-agent-platform",
			Endpoint:    "http://localhost:4317",
			SampleRate:  0.1,
		},
		LLM: LLMConfig{
			AnthropicAPIKey: "",
			ZhipuAPIKey:     "",
		},
		Sandbox: SandboxConfig{
			Backend:          "docker",
			FallbackToDocker: true,
		},
		MinIO: MinIOConfig{
			Endpoint:  "localhost:9000",
			AccessKey: "",
			SecretKey: "",
			Bucket:    "cap-artifacts",
			UseSSL:    false,
		},
		Git: GitConfig{
			HTTPSUser:  "",
			HTTPSToken: "",
		},
		RateLimit: RateLimitConfig{
			Enabled: true,
			QPS:     100,
			Burst:   200,
		},
		Plugins: PluginsConfig{
			Search: SearchPluginConfig{
				Enabled: false,
			},
		},
		Approval: DefaultApprovalConfig(),
	}
}

// Load loads configuration from YAML file and environment variables.
// Environment variables with APP_ prefix override YAML values (e.g., APP_SERVER_PORT=8080).
func Load(path string) (*Config, error) {
	k := koanf.New(".")

	// 1. Load default config
	if err := k.Load(structs.Provider(DefaultConfig(), "koanf"), nil); err != nil {
		return nil, fmt.Errorf("load default config: %w", err)
	}

	// 2. Load YAML file if exists
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
				return nil, fmt.Errorf("load yaml config %s: %w", path, err)
			}
		}
	}

	// 3. Load environment variables with APP_ prefix
	// APP_SERVER_PORT -> server.port, APP_DATABASE_DSN -> database.dsn
	if err := k.Load(env.Provider("APP_", ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "APP_")), "_", ".")
	}), nil); err != nil {
		return nil, fmt.Errorf("load env config: %w", err)
	}

	// 4. Unmarshal to Config struct
	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

// MustLoad loads configuration or panics.
func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		panic("failed to load config: " + err.Error())
	}
	return cfg
}
