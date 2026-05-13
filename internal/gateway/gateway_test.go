package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/service"
	"go.uber.org/zap/zaptest"
)

func TestGatewayNew(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{
		Logger: logger,
	})

	cfg := Config{
		Port:         8080,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:    "test-secret",
		CORSOrigins:   []string{"*"},
	}

	g := New(cfg, svc, logger)
	if g == nil {
		t.Error("expected non-nil Gateway")
	}
}

func TestGatewayStart(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{
		Logger: logger,
	})

	cfg := Config{
		Port:         8081,
		ReadTimeout:  1 * time.Second,
		WriteTimeout:  1 * time.Second,
		JWTSecret:    "test-secret",
		CORSOrigins:   []string{"*"},
	}

	g := New(cfg, svc, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := g.Start(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGatewayStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{
		Logger: logger,
	})

	cfg := Config{
		Port:         8082,
		ReadTimeout:  1 * time.Second,
		WriteTimeout:  1 * time.Second,
		JWTSecret:    "test-secret",
		CORSOrigins:   []string{"*"},
	}

	g := New(cfg, svc, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = g.Start(ctx) // ignore error from cancelled context

	err := g.Stop(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
