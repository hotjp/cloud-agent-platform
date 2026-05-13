// Package config implements configuration management using koanf.
// Supports YAML files + environment variable overrides (APP_ prefix).
package config

import (
	"fmt"
	"time"
)

// Config holds all configuration for the application.
type Config struct {
	Server    ServerConfig    `koanf:"server"`
	Database  DatabaseConfig  `koanf:"database"`
	Redis     RedisConfig     `koanf:"redis"`
	Authz     AuthzConfig     `koanf:"authz"`
	Telemetry TelemetryConfig `koanf:"telemetry"`
	LLM       LLMConfig       `koanf:"llm"`
	Sandbox   SandboxConfig   `koanf:"sandbox"`
	MinIO     MinIOConfig     `koanf:"minio"`
	Git       GitConfig       `koanf:"git"`
	RateLimit RateLimitConfig `koanf:"rate_limit"`
	Approval  ApprovalConfig   `koanf:"approval"`
	Plugins   PluginsConfig   `koanf:"plugins"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port         int           `koanf:"port"`
	MetricsPort  int           `koanf:"metricsport"`
	PprofPort    int           `koanf:"pprofport"`
	ReadTimeout  time.Duration `koanf:"readtimeout"`
	WriteTimeout time.Duration `koanf:"writetimeout"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	DSN         string        `koanf:"dsn"`
	MaxOpen     int           `koanf:"maxopen"`
	MaxIdle     int           `koanf:"maxidle"`
	MaxLifetime time.Duration `koanf:"maxlifetime"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr     string `koanf:"addr"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}

// AuthzConfig holds authorization settings.
type AuthzConfig struct {
	JWTSecret    string        `koanf:"jwtsecret"`
	APIKeyHeader string        `koanf:"apikeyheader"`
	CacheTTL     time.Duration `koanf:"cachettl"`
}

// TelemetryConfig holds OpenTelemetry settings.
type TelemetryConfig struct {
	ServiceName string  `koanf:"servicename"`
	Endpoint    string  `koanf:"endpoint"`
	SampleRate  float64 `koanf:"samplerate"`
}

// LLMConfig holds LLM provider API keys and settings.
type LLMConfig struct {
	AnthropicAPIKey string `koanf:"anthropicapikey"`
	ZhipuAPIKey     string `koanf:"zhipuapikey"`
}

// SandboxConfig holds Worker sandbox configuration.
type SandboxConfig struct {
	Backend          string `koanf:"backend"`
	FallbackToDocker bool   `koanf:"fallbacktodocker"`
}

// MinIOConfig holds MinIO object storage settings.
type MinIOConfig struct {
	Endpoint  string `koanf:"endpoint"`
	AccessKey string `koanf:"accesskey"`
	SecretKey string `koanf:"secretkey"`
	Bucket    string `koanf:"bucket"`
	UseSSL    bool   `koanf:"usessl"`
}

// GitConfig holds Git operation settings.
type GitConfig struct {
	HTTPSUser  string `koanf:"httpsuser"`
	HTTPSToken string `koanf:"httpstoken"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	Enabled bool `koanf:"enabled"`
	QPS     int  `koanf:"qps"`
	Burst   int  `koanf:"burst"`
}

// PluginsConfig holds plugin configuration.
type PluginsConfig struct {
	Search SearchPluginConfig `koanf:"search"`
}

// SearchPluginConfig holds search plugin settings.
type SearchPluginConfig struct {
	Enabled bool   `koanf:"enabled"`
	Host    string `koanf:"host"`
	APIKey  string `koanf:"apikey"`
}

// ApprovalConfig holds approval/guardian settings.
type ApprovalConfig struct {
	// DefaultTimeout is the default timeout for approval requests (default: 5 minutes).
	DefaultTimeout time.Duration `koanf:"default_timeout"`
	// MaxTimeout is the maximum allowed timeout for approval requests.
	MaxTimeout time.Duration `koanf:"max_timeout"`
	// HighRiskCostThreshold is the estimated cost threshold (in yuan) above which
	// an operation is considered high-risk and requires approval.
	HighRiskCostThreshold float64 `koanf:"high_risk_cost_threshold"`
}

// DefaultApprovalConfig returns the default approval configuration.
func DefaultApprovalConfig() ApprovalConfig {
	return ApprovalConfig{
		DefaultTimeout:       5 * time.Minute,
		MaxTimeout:           30 * time.Minute,
		HighRiskCostThreshold: 1.0, // 1 yuan
	}
}

// Validate checks that required configuration fields are set.
func (c *Config) Validate() error {
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}
	if c.Authz.JWTSecret == "" {
		return fmt.Errorf("authz.jwtsecret is required")
	}
	if c.Approval.DefaultTimeout <= 0 {
		return fmt.Errorf("approval.default_timeout must be positive")
	}
	if c.Approval.MaxTimeout <= 0 {
		return fmt.Errorf("approval.max_timeout must be positive")
	}
	if c.Approval.DefaultTimeout > c.Approval.MaxTimeout {
		return fmt.Errorf("approval.default_timeout must not exceed approval.max_timeout")
	}
	return nil
}
