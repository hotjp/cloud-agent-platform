// Package main is the entry point for Cloud Agent Platform server.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/internal/config"
	"github.com/cloud-agent-platform/cap/internal/gateway"
	"github.com/cloud-agent-platform/cap/internal/gateway/ws"
	"github.com/cloud-agent-platform/cap/internal/infra/cache"
	"github.com/cloud-agent-platform/cap/internal/infra/outbox"
	"github.com/cloud-agent-platform/cap/internal/infra/persistence"
	"github.com/cloud-agent-platform/cap/internal/observability/metrics"
	"github.com/cloud-agent-platform/cap/internal/observability/tracing"
	"github.com/cloud-agent-platform/cap/internal/service"
	"github.com/cloud-agent-platform/cap/internal/storage"
	"github.com/cloud-agent-platform/cap/plugins/gitclient"
	"github.com/cloud-agent-platform/cap/plugins/llmrouter"
	"github.com/cloud-agent-platform/cap/plugins/mcpserver"
	"github.com/cloud-agent-platform/cap/plugins/workermanager"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// transactionManagerAdapter wraps *storage.Storage to implement service.TransactionManager.
// This adapter is needed because storage.TransactionManager and service.TransactionManager
// are separate interface types in different packages, even though structurally identical.
// The adapter delegates to the underlying storage instance.
type transactionManagerAdapter struct {
	s *storage.Storage
}

func (a *transactionManagerAdapter) Commit(ctx context.Context) error {
	return a.s.Commit(ctx)
}

func (a *transactionManagerAdapter) Rollback(ctx context.Context) error {
	return a.s.Rollback(ctx)
}

func (a *transactionManagerAdapter) BeginTx(ctx context.Context) (service.TransactionManager, error) {
	tx, err := a.s.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	// tx is storage.TransactionManager, get the underlying *ent.Tx
	entTx := tx.Tx()
	// entTx is *ent.Tx which has Commit/Rollback methods
	// Wrap it in our adapter
	return &entTxnAdapter{tx: entTx}, nil
}

// entTxnAdapter wraps *ent.Tx to implement service.TransactionManager.
// ent.Tx embeds *internalTx which has Commit/Rollback (no ctx params).
type entTxnAdapter struct {
	tx *ent.Tx
}

func (a *entTxnAdapter) Commit(ctx context.Context) error {
	// ent.Tx.Commit has no parameters (inherited from internalTx)
	return a.tx.Commit()
}

func (a *entTxnAdapter) Rollback(ctx context.Context) error {
	return a.tx.Rollback()
}

func (a *entTxnAdapter) BeginTx(ctx context.Context) (service.TransactionManager, error) {
	// For ent, nested transactions aren't directly supported via ent.Tx
	// The ent client.BeginTx would create a new top-level transaction
	// For now, just return a simple adapter that wraps the current ent.Tx
	// This is a limitation - nested transactions will not truly be nested
	return &simpleEntTxnAdapter{tx: a.tx}, nil
}

// simpleEntTxnAdapter wraps *ent.Tx for cases where we can't do true nesting.
type simpleEntTxnAdapter struct {
	tx *ent.Tx
}

func (a *simpleEntTxnAdapter) Commit(ctx context.Context) error {
	return a.tx.Commit()
}

func (a *simpleEntTxnAdapter) Rollback(ctx context.Context) error {
	return a.tx.Rollback()
}

func (a *simpleEntTxnAdapter) BeginTx(ctx context.Context) (service.TransactionManager, error) {
	// Can't truly nest ent transactions, just return self
	return a, nil
}

func main() {
	// Initialize logger (zap)
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	}()

	// Load configuration
	cfg := config.MustLoad("config.yaml")

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("Configuration validation failed", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("Configuration loaded",
		zap.Int("port", cfg.Server.Port),
		zap.String("service_name", cfg.Telemetry.ServiceName),
	)

	// Initialize storage (L1-Storage)
	store, err := storage.New(cfg, logger)
	if err != nil {
		logger.Error("Failed to create storage", zap.Error(err))
		os.Exit(1)
	}
	defer store.Close(ctx)

	// Initialize Redis client for outbox forwarder
	redisClient, err := cache.NewRedisClient(&cfg.Redis, logger)
	if err != nil {
		logger.Error("Failed to create Redis client", zap.Error(err))
		os.Exit(1)
	}
	defer redisClient.Close()

	// Initialize MinIO storage for cleanup worker
	minioStorage, err := storage.NewMinIOStorage(&cfg.MinIO, logger, storage.MinIOOptions{})
	if err != nil {
		logger.Error("Failed to create MinIO storage", zap.Error(err))
		os.Exit(1)
	}

	// Wire repositories and outbox writer
	taskRepo := persistence.NewTaskRepository(store.Client(), logger)
	subtaskRepo := persistence.NewSubtaskRepository(store.Client(), logger)
	outboxWriter := outbox.NewOutboxWriter(store.Client(), logger)

	// Wire metrics and tracing
	metricsInstance := metrics.NewMetrics()
	metricsRecorder := metrics.NewRecorder(metricsInstance)
	tracer := tracing.NewSpanHelper()

	// Create TaskService with full DI
	// Note: Storage uses adapter to implement service.TransactionManager
	txAdapter := &transactionManagerAdapter{s: store}
	serviceSvc := service.NewTaskService(service.TaskServiceInput{
		TaskRepo:     taskRepo,
		SubtaskRepo:  subtaskRepo,
		OutboxWriter: outboxWriter,
		Storage:      txAdapter,
		Logger:       logger,
		Metrics:      metricsRecorder,
		Tracer:       tracer,
		// Orchestrator is nil for now - orchestration is triggered via outbox events
		// TODO: Create and inject OrchestratorImpl once event consumer is implemented
	})

	// Initialize tracing (observability)
	tracerConfig := tracing.TelemetryConfig{
		ServiceName: cfg.Telemetry.ServiceName,
		Endpoint:    cfg.Telemetry.Endpoint,
		SampleRate:  cfg.Telemetry.SampleRate,
	}
	tracerShutdown, err := tracing.InitTracer(ctx, tracerConfig, logger)
	if err != nil {
		logger.Error("Failed to initialize tracer", zap.Error(err))
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := tracerShutdown(shutdownCtx); err != nil {
			logger.Error("Failed to shutdown tracer", zap.Error(err))
		}
	}()

	// Initialize metrics provider
	metricsProvider, err := metrics.NewProvider(ctx, metrics.Config{ServiceName: cfg.Telemetry.ServiceName})
	if err != nil {
		logger.Error("Failed to create metrics provider", zap.Error(err))
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := metricsProvider.Shutdown(shutdownCtx); err != nil {
			logger.Error("Failed to shutdown metrics provider", zap.Error(err))
		}
	}()

	// Start outbox poller background goroutine
	redisForwarder := outbox.NewRedisStreamForwarder(redisClient, logger)
	pollerConfig := outbox.DefaultPollerConfig()
	poller, err := outbox.NewOutboxPoller(store.Client(), store.DB(), redisForwarder, logger, pollerConfig)
	if err != nil {
		logger.Error("Failed to create outbox poller", zap.Error(err))
		os.Exit(1)
	}
	poller.Start(ctx)
	defer poller.Stop()

	// Start cleanup worker background goroutine
	cleanupConfig := storage.DefaultCleanupConfig()
	cleanupWorker, err := storage.NewCleanupWorker(minioStorage, logger, cleanupConfig)
	if err != nil {
		logger.Error("Failed to create cleanup worker", zap.Error(err))
		os.Exit(1)
	}
	cleanupWorker.Start(ctx)
	defer cleanupWorker.Stop()

	// -------------------------------------------------------------------------
	// Initialize Plugins
	// -------------------------------------------------------------------------

	// Initialize LLM Router with provider configurations
	llmRouterCfg := llmrouter.DefaultConfig()
	// Override with API keys from config if provided
	if cfg.LLM.AnthropicAPIKey != "" {
		if providerCfg, ok := llmRouterCfg.ProviderConfigs[llmrouter.ModelClaudeSonnet]; ok {
			providerCfg.APIKey = cfg.LLM.AnthropicAPIKey
			llmRouterCfg.ProviderConfigs[llmrouter.ModelClaudeSonnet] = providerCfg
		}
		if providerCfg, ok := llmRouterCfg.ProviderConfigs[llmrouter.ModelClaudeHaiku]; ok {
			providerCfg.APIKey = cfg.LLM.AnthropicAPIKey
			llmRouterCfg.ProviderConfigs[llmrouter.ModelClaudeHaiku] = providerCfg
		}
	}
	if cfg.LLM.ZhipuAPIKey != "" {
		if providerCfg, ok := llmRouterCfg.ProviderConfigs[llmrouter.ModelGLM5]; ok {
			providerCfg.APIKey = cfg.LLM.ZhipuAPIKey
			llmRouterCfg.ProviderConfigs[llmrouter.ModelGLM5] = providerCfg
		}
		if providerCfg, ok := llmRouterCfg.ProviderConfigs[llmrouter.ModelGLM5Air]; ok {
			providerCfg.APIKey = cfg.LLM.ZhipuAPIKey
			llmRouterCfg.ProviderConfigs[llmrouter.ModelGLM5Air] = providerCfg
		}
	}
	llmRouter := llmrouter.NewFromConfig(llmRouterCfg, logger, metricsRecorder)
	if err := llmRouter.Initialize(); err != nil {
		logger.Error("Failed to initialize LLM router", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("LLM router initialized",
		zap.Bool("enabled", llmRouter.Enabled()),
	)

	// Initialize Git Client
	var gitClient *gitclient.GitClient
	if cfg.Git.HTTPSUser != "" && cfg.Git.HTTPSToken != "" {
		gitClient = gitclient.NewWithAuth(logger, &gitclient.AuthMethod{
			Username: cfg.Git.HTTPSUser,
			Password: cfg.Git.HTTPSToken,
		})
	} else {
		gitClient = gitclient.New(logger)
	}
	// gitClient is available for agent execution and MCP tools
	_ = gitClient
	logger.Info("Git client initialized",
		zap.Bool("has_auth", cfg.Git.HTTPSUser != ""),
	)

	// Initialize Docker Backend for Worker Manager
	dockerBackendCfg := workermanager.DefaultDockerBackendConfig()
	dockerBackend, err := workermanager.NewDockerBackend(dockerBackendCfg, logger)
	if err != nil {
		logger.Error("Failed to create Docker backend", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("Docker backend initialized")

	// Initialize Worker Manager
	wmCfg := workermanager.DefaultConfig()
	// Use default image from docker backend config
	wmCfg.DefaultSandboxOpts.Image = dockerBackendCfg.DefaultImage
	wm, err := workermanager.New(wmCfg, dockerBackend, logger)
	if err != nil {
		logger.Error("Failed to create worker manager", zap.Error(err))
		os.Exit(1)
	}
	if err := wm.Start(); err != nil {
		logger.Error("Failed to start worker manager", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("Worker manager started",
		zap.Int("max_workers", wmCfg.MaxWorkers),
	)

	// Initialize MCP Server
	mcpServer := mcpserver.NewMCPServer(logger)
	logger.Info("MCP server initialized",
		zap.Int("tools_count", len(mcpServer.Tools())),
		zap.Int("resources_count", len(mcpServer.Resources())),
	)

	// Start metrics server on separate port
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.MetricsPort),
		Handler: promhttp.Handler(),
	}
	go func() {
		logger.Info("Starting metrics server", zap.Int("port", cfg.Server.MetricsPort))
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Metrics server error", zap.Error(err))
		}
	}()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Failed to shutdown metrics server", zap.Error(err))
		}
	}()

	// Determine CORS origins from config
	corsOrigins := cfg.Gateway.CORSOrigins
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"*"}
	}

	// Initialize and start gateway
	gwCfg := gateway.Config{
		Port:         cfg.Server.Port,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		JWTSecret:    cfg.Authz.JWTSecret,
		CORSOrigins:  corsOrigins,
		WSHubConfig:  ws.DefaultHubConfig(redisClient, logger),
	}
	gatewaySvc := gateway.New(gwCfg, serviceSvc, logger, store.DB(), redisClient, minioStorage, metricsRecorder)

	logger.Info("Server starting",
		zap.String("version", "0.1.0"),
		zap.Int("port", cfg.Server.Port),
	)

	if err := gatewaySvc.Start(ctx); err != nil {
		logger.Error("Failed to start gateway", zap.Error(err))
		os.Exit(1)
	}

	// Wait for shutdown
	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Shutdown plugins in reverse order of initialization

	// Stop Worker Manager first (depends on Docker backend)
	if wm != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := wm.Stop(shutdownCtx); err != nil {
			logger.Error("Failed to stop worker manager", zap.Error(err))
		}
	}

	// Shutdown LLM Router
	if llmRouter != nil {
		if err := llmRouter.Shutdown(); err != nil {
			logger.Error("Failed to shutdown LLM router", zap.Error(err))
		}
	}

	if err := gatewaySvc.Stop(shutdownCtx); err != nil {
		logger.Error("Failed to stop gateway", zap.Error(err))
	}

	logger.Info("Server stopped")
}
