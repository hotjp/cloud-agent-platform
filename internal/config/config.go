// Package config implements configuration management using koanf.
// Supports YAML files + environment variable overrides (APP_ prefix).
package config

import "time"

// Config holds all configuration for the application.
type Config struct {
	Server     ServerConfig     `koanf:"server"`
	Database   DatabaseConfig   `koanf:"database"`
	Redis      RedisConfig      `koanf:"redis"`
	Authz      AuthzConfig      `koanf:"authz"`
	Telemetry  TelemetryConfig  `koanf:"telemetry"`
	Plugins    PluginsConfig    `koanf:"plugins"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port         int           `koanf:"port"`
	MetricsPort  int           `koanf:"metrics_port"`
	PprofPort    int           `koanf:"pprof_port"`
	ReadTimeout  time.Duration `koanf:"read_timeout"`
	WriteTimeout time.Duration `koanf:"write_timeout"`
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	DSN         string        `koanf:"dsn"`
	MaxOpen     int           `koanf:"max_open"`
	MaxIdle     int           `koanf:"max_idle"`
	MaxLifetime time.Duration `koanf:"max_lifetime"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr     string `koanf:"addr"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}

// AuthzConfig holds authorization settings.
type AuthzConfig struct {
	JWTSecret    string        `koanf:"jwt_secret"`
	APIKeyHeader string        `koanf:"api_key_header"`
	CacheTTL     time.Duration `koanf:"cache_ttl"`
}

// TelemetryConfig holds OpenTelemetry settings.
type TelemetryConfig struct {
	ServiceName string  `koanf:"service_name"`
	Endpoint    string  `koanf:"endpoint"`
	SampleRate  float64 `koanf:"sample_rate"`
}

// PluginsConfig holds plugin configuration.
type PluginsConfig struct {
	Search SearchPluginConfig `koanf:"search"`
}

// SearchPluginConfig holds search plugin settings.
type SearchPluginConfig struct {
	Enabled bool   `koanf:"enabled"`
	Host    string `koanf:"host"`
	APIKey  string `koanf:"api_key"`
}
