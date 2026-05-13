// Package main is the entry point for Cloud Agent Platform server.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-agent-platform/cap/internal/config"
	"github.com/cloud-agent-platform/cap/internal/gateway"
	"github.com/cloud-agent-platform/cap/internal/service"
	"github.com/cloud-agent-platform/cap/internal/storage"
)

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
	logger.Info("Configuration loaded",
		zap.Int("port", cfg.Server.Port),
		zap.String("service_name", cfg.Telemetry.ServiceName),
	)

	// Initialize layers: Storage → Service → Gateway
	store, err := storage.New(cfg, logger)
	if err != nil {
		logger.Error("Failed to create storage", zap.Error(err))
		os.Exit(1)
	}
	defer store.Close(ctx)

	serviceSvc := service.NewTaskService(service.TaskServiceInput{
		Logger: logger,
	})

	logger.Info("Server starting",
		zap.String("version", "0.1.0"),
		zap.Int("port", cfg.Server.Port),
	)

	// Initialize and start gateway
	gwCfg := gateway.Config{
		Port:        cfg.Server.Port,
		ReadTimeout: cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		JWTSecret:   cfg.Authz.JWTSecret,
	}
	gatewaySvc := gateway.New(gwCfg, serviceSvc, logger)
	if err := gatewaySvc.Start(ctx); err != nil {
		logger.Error("Failed to start gateway", zap.Error(err))
		os.Exit(1)
	}

	// Wait for shutdown
	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := gatewaySvc.Stop(shutdownCtx); err != nil {
		logger.Error("Failed to stop gateway", zap.Error(err))
	}

	logger.Info("Server stopped")
}
