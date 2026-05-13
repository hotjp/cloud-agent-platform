// Package main is the entry point for the MCP server.
// It runs as a standalone process communicating with AI agents via stdio.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Parse flags
	serverURL := flag.String("server-url", "http://localhost:8080", "Cloud Agent Platform server URL")
	token := flag.String("token", "", "JWT token for authentication")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Initialize logger
	level := zapcore.InfoLevel
	switch *logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(os.Stderr),
		level,
	))

	// Create platform client
	client := mcp.NewPlatformClient(*serverURL, *token, logger)

	// Create tool executor
	executor := mcp.NewToolExecutor(client, logger)

	// Create MCP server
	server := mcp.NewServer(executor, logger)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down",
			zap.String("layer", "MCP"),
			zap.String("signal", sig.String()),
		)
		cancel()
	}()

	logger.Info("MCP server starting",
		zap.String("layer", "MCP"),
		zap.String("server_url", *serverURL),
	)

	if err := server.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("MCP server error",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		os.Exit(1)
	}

	logger.Info("MCP server stopped",
		zap.String("layer", "MCP"),
	)
}
