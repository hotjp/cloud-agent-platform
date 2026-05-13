package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_WithEnvVars(t *testing.T) {
	// Set environment variables to override defaults
	// Env vars use flat naming (no underscores in value part):
	// APP_CATEGORY_VALUE -> category.value (each underscore = path separator)
	os.Setenv("APP_SERVER_PORT", "9090")
	os.Setenv("APP_SERVER_METRICSPORT", "9091")
	os.Setenv("APP_DATABASE_DSN", "postgres://test:test@localhost:5432/testdb?sslmode=disable")
	os.Setenv("APP_REDIS_ADDR", "localhost:6380")
	os.Setenv("APP_REDIS_PASSWORD", "testpass")
	os.Setenv("APP_REDIS_DB", "1")
	os.Setenv("APP_AUTHZ_JWTSECRET", "secret123")
	os.Setenv("APP_AUTHZ_APIKEYHEADER", "X-Custom-API-Key")
	os.Setenv("APP_LLM_ANTHROPICAPIKEY", "sk-ant-test")
	os.Setenv("APP_LLM_ZHIPUAPIKEY", "sk-zhipu-test")
	os.Setenv("APP_SANDBOX_BACKEND", "cubesandbox")
	os.Setenv("APP_SANDBOX_FALLBACKTODOCKER", "false")
	os.Setenv("APP_MINIO_ENDPOINT", "minio.local:9000")
	os.Setenv("APP_MINIO_ACCESSKEY", "minioaccess")
	os.Setenv("APP_MINIO_SECRETKEY", "miniosecret")
	os.Setenv("APP_MINIO_BUCKET", "test-bucket")
	os.Setenv("APP_GIT_HTTPSUSER", "gituser")
	os.Setenv("APP_GIT_HTTPSTOKEN", "gittoken")
	defer func() {
		os.Unsetenv("APP_SERVER_PORT")
		os.Unsetenv("APP_SERVER_METRICSPORT")
		os.Unsetenv("APP_DATABASE_DSN")
		os.Unsetenv("APP_REDIS_ADDR")
		os.Unsetenv("APP_REDIS_PASSWORD")
		os.Unsetenv("APP_REDIS_DB")
		os.Unsetenv("APP_AUTHZ_JWTSECRET")
		os.Unsetenv("APP_AUTHZ_APIKEYHEADER")
		os.Unsetenv("APP_LLM_ANTHROPICAPIKEY")
		os.Unsetenv("APP_LLM_ZHIPUAPIKEY")
		os.Unsetenv("APP_SANDBOX_BACKEND")
		os.Unsetenv("APP_SANDBOX_FALLBACKTODOCKER")
		os.Unsetenv("APP_MINIO_ENDPOINT")
		os.Unsetenv("APP_MINIO_ACCESSKEY")
		os.Unsetenv("APP_MINIO_SECRETKEY")
		os.Unsetenv("APP_MINIO_BUCKET")
		os.Unsetenv("APP_GIT_HTTPSUSER")
		os.Unsetenv("APP_GIT_HTTPSTOKEN")
	}()

	cfg, err := Load("")
	require.NoError(t, err)

	// Server config from env
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, 9091, cfg.Server.MetricsPort)

	// Database config from env
	assert.Equal(t, "postgres://test:test@localhost:5432/testdb?sslmode=disable", cfg.Database.DSN)

	// Redis config from env
	assert.Equal(t, "localhost:6380", cfg.Redis.Addr)
	assert.Equal(t, "testpass", cfg.Redis.Password)
	assert.Equal(t, 1, cfg.Redis.DB)

	// Authz config from env
	assert.Equal(t, "secret123", cfg.Authz.JWTSecret)
	assert.Equal(t, "X-Custom-API-Key", cfg.Authz.APIKeyHeader)

	// LLM config from env
	assert.Equal(t, "sk-ant-test", cfg.LLM.AnthropicAPIKey)
	assert.Equal(t, "sk-zhipu-test", cfg.LLM.ZhipuAPIKey)

	// Sandbox config from env
	assert.Equal(t, "cubesandbox", cfg.Sandbox.Backend)
	assert.False(t, cfg.Sandbox.FallbackToDocker)

	// MinIO config from env
	assert.Equal(t, "minio.local:9000", cfg.MinIO.Endpoint)
	assert.Equal(t, "minioaccess", cfg.MinIO.AccessKey)
	assert.Equal(t, "miniosecret", cfg.MinIO.SecretKey)
	assert.Equal(t, "test-bucket", cfg.MinIO.Bucket)

	// Git config from env
	assert.Equal(t, "gituser", cfg.Git.HTTPSUser)
	assert.Equal(t, "gittoken", cfg.Git.HTTPSToken)
}

func TestLoad_WithDefaults(t *testing.T) {
	// Clear all env vars that might interfere
	envVars := []string{
		"APP_SERVER_PORT", "APP_SERVER_METRICSPORT", "APP_DATABASE_DSN",
		"APP_REDIS_ADDR", "APP_REDIS_PASSWORD", "APP_REDIS_DB",
		"APP_AUTHZ_JWTSECRET", "APP_AUTHZ_APIKEYHEADER",
		"APP_LLM_ANTHROPICAPIKEY", "APP_SANDBOX_BACKEND",
		"APP_MINIO_ENDPOINT", "APP_GIT_HTTPSUSER",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}

	cfg, err := Load("")
	require.NoError(t, err)

	// Check defaults
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 9090, cfg.Server.MetricsPort)
	assert.Equal(t, 6060, cfg.Server.PprofPort)
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, 0, cfg.Redis.DB)
	assert.Equal(t, "docker", cfg.Sandbox.Backend)
	assert.True(t, cfg.Sandbox.FallbackToDocker)
	assert.Equal(t, "cap-artifacts", cfg.MinIO.Bucket)
	assert.False(t, cfg.MinIO.UseSSL)
	assert.True(t, cfg.RateLimit.Enabled)
	assert.Equal(t, 100, cfg.RateLimit.QPS)
	assert.Equal(t, 200, cfg.RateLimit.Burst)
}

func TestLoad_WithYAMLFile(t *testing.T) {
	// Create a temporary YAML config file
	yamlContent := `
server:
  port: 8888
  metricsport: 8889
database:
  dsn: "postgres://yaml:yaml@localhost:5432/yamldb?sslmode=disable"
  maxopen: 50
redis:
  addr: "redis.yaml:6379"
  db: 2
authz:
  jwtsecret: "yaml-secret"
  apikeyheader: "X-YAML-API-Key"
  cachettl: 5m
telemetry:
  servicename: "yaml-service"
  endpoint: "http://yaml-otel:4317"
  samplerate: 0.2
llm:
  anthropicapikey: "sk-ant-yaml-key"
  zhipuapikey: "yaml-zhipu-key"
sandbox:
  backend: "cubesandbox"
  fallbacktodocker: false
minio:
  endpoint: "minio.yaml:9000"
  accesskey: "yaml-key"
  secretkey: "yaml-secret"
  bucket: "yaml-bucket"
  usessl: true
git:
  httpsuser: "yaml-git-user"
  httpstoken: "yaml-git-token"
rate_limit:
  enabled: false
  qps: 50
  burst: 100
plugins:
  search:
    enabled: true
    host: "http://yaml-search:7700"
    apikey: "yaml-search-key"
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(yamlContent)
	require.NoError(t, err)
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	require.NoError(t, err)

	// YAML values should be used
	assert.Equal(t, 8888, cfg.Server.Port)
	assert.Equal(t, 8889, cfg.Server.MetricsPort)
	assert.Equal(t, "postgres://yaml:yaml@localhost:5432/yamldb?sslmode=disable", cfg.Database.DSN)
	assert.Equal(t, 50, cfg.Database.MaxOpen)
	assert.Equal(t, "redis.yaml:6379", cfg.Redis.Addr)
	assert.Equal(t, 2, cfg.Redis.DB)
	assert.Equal(t, "yaml-secret", cfg.Authz.JWTSecret)
	assert.Equal(t, "X-YAML-API-Key", cfg.Authz.APIKeyHeader)
	assert.Equal(t, int64(5*time.Minute), int64(cfg.Authz.CacheTTL))
	assert.Equal(t, "yaml-service", cfg.Telemetry.ServiceName)
	assert.Equal(t, "http://yaml-otel:4317", cfg.Telemetry.Endpoint)
	assert.Equal(t, 0.2, cfg.Telemetry.SampleRate)
	assert.Equal(t, "sk-ant-yaml-key", cfg.LLM.AnthropicAPIKey)
	assert.Equal(t, "yaml-zhipu-key", cfg.LLM.ZhipuAPIKey)
	assert.Equal(t, "cubesandbox", cfg.Sandbox.Backend)
	assert.False(t, cfg.Sandbox.FallbackToDocker)
	assert.Equal(t, "minio.yaml:9000", cfg.MinIO.Endpoint)
	assert.Equal(t, "yaml-key", cfg.MinIO.AccessKey)
	assert.Equal(t, "yaml-secret", cfg.MinIO.SecretKey)
	assert.Equal(t, "yaml-bucket", cfg.MinIO.Bucket)
	assert.True(t, cfg.MinIO.UseSSL)
	assert.Equal(t, "yaml-git-user", cfg.Git.HTTPSUser)
	assert.Equal(t, "yaml-git-token", cfg.Git.HTTPSToken)
	assert.False(t, cfg.RateLimit.Enabled)
	assert.Equal(t, 50, cfg.RateLimit.QPS)
	assert.Equal(t, 100, cfg.RateLimit.Burst)
	assert.True(t, cfg.Plugins.Search.Enabled)
	assert.Equal(t, "http://yaml-search:7700", cfg.Plugins.Search.Host)
	assert.Equal(t, "yaml-search-key", cfg.Plugins.Search.APIKey)
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	// Create a temporary YAML config file
	yamlContent := `
server:
  port: 8888
database:
  dsn: "postgres://yaml:yaml@localhost:5432/yamldb?sslmode=disable"
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(yamlContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Set env vars that should override YAML
	os.Setenv("APP_SERVER_PORT", "9999")
	os.Setenv("APP_AUTHZ_JWTSECRET", "env-secret")
	defer func() {
		os.Unsetenv("APP_SERVER_PORT")
		os.Unsetenv("APP_AUTHZ_JWTSECRET")
	}()

	cfg, err := Load(tmpFile.Name())
	require.NoError(t, err)

	// Env should override YAML
	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, "env-secret", cfg.Authz.JWTSecret)
	// YAML value should still be used for database
	assert.Equal(t, "postgres://yaml:yaml@localhost:5432/yamldb?sslmode=disable", cfg.Database.DSN)
}

func TestMustLoad_NoPanicOnMissingFile(t *testing.T) {
	// With nonexistent YAML path, Load uses defaults and doesn't panic
	assert.NotPanics(t, func() {
		cfg := MustLoad("/nonexistent/path/config.yaml")
		assert.NotNil(t, cfg)
		assert.Equal(t, 8080, cfg.Server.Port) // Default value
	})
}
