// Package gateway implements L5-Gateway layer: protocol adaptation, middleware chain,
// and request routing via connect-go.
package gateway

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/cloud-agent-platform/cap/api/cap/v1"
	"github.com/cloud-agent-platform/cap/api/cap/v1/capv1connect"
	"github.com/cloud-agent-platform/cap/internal/service"
	"go.uber.org/zap"
)

// Gateway handles HTTP/gRPC protocol adaptation and middleware.
type Gateway struct {
	server       *http.Server
	handler      *TaskServiceHandler
	logger       *zap.Logger
	metrics      *MetricsCollector
	authConfig   AuthConfig
	corsOrigins  []string
}

// Config holds gateway configuration.
type Config struct {
	Port        int
	ReadTimeout time.Duration
	WriteTimeout time.Duration
	JWTSecret   string
	CORSOrigins []string
}

// New creates a new Gateway instance.
func New(cfg Config, svc *service.TaskService, logger *zap.Logger) *Gateway {
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.CORSOrigins == nil {
		cfg.CORSOrigins = []string{"*"}
	}

	metrics := NewMetricsCollector()
	handler := NewTaskServiceHandler(svc, logger)

	g := &Gateway{
		logger:      logger,
		metrics:     metrics,
		authConfig:  AuthConfig{JWTSecret: cfg.JWTSecret},
		corsOrigins: cfg.CORSOrigins,
		handler:     handler,
	}

	// Build the handler chain: Recover → RequestID → Metrics → Logging → CORS → Auth → Routing
	mux := http.NewServeMux()

	// Register the connect-go handler
	path, connectHandler := capv1connect.NewTaskServiceHandler(handler)
	mux.Handle(path, connectHandler)

	// Register REST API adapter
	rest := NewRESTAdapter(svc, logger)
	mux.HandleFunc("/api/v1/tasks", rest.handleTasks)
	mux.HandleFunc("/api/v1/tasks/", rest.handleTaskOperations)
	mux.HandleFunc("/api/v1/agent-templates", rest.ListAgents)
	mux.HandleFunc("/api/v1/agent-templates/", rest.ListAgents)
	mux.HandleFunc("/api/v1/sessions", rest.ListSessions)
	mux.HandleFunc("/api/v1/status", rest.PlatformStatus)
	mux.HandleFunc("/api/v1/dashboard/stats", rest.DashboardStats)

	// Health check endpoints
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/readyz", readyzHandler)

	// Build the middleware chain
	wrappedHandler := Chain(mux,
		Recover(logger),
		RequestID(),
		Metrics(metrics),
		Logging(logger),
		CORS(g.corsOrigins),
		Auth(g.authConfig),
	)

	g.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      wrappedHandler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	return g
}

// Start starts the gateway server.
func (g *Gateway) Start(ctx context.Context) error {
	g.logger.Info("starting gateway",
		zap.String("layer", "L5"),
		zap.String("addr", g.server.Addr),
	)

	errCh := make(chan error, 1)
	go func() {
		if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		return nil
	}
}

// Stop gracefully stops the gateway server.
func (g *Gateway) Stop(ctx context.Context) error {
	g.logger.Info("stopping gateway", zap.String("layer", "L5"))

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := g.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	g.logger.Info("gateway stopped", zap.String("layer", "L5"))
	return nil
}

// Handler returns the HTTP handler for testing purposes.
func (g *Gateway) Handler() http.Handler {
	return g.server.Handler
}

// MetricsCollector returns the metrics collector for testing purposes.
func (g *Gateway) MetricsCollector() *MetricsCollector {
	return g.metrics
}

// healthzHandler handles the health check endpoint.
func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// readyzHandler handles the readiness check endpoint.
func readyzHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Check database and Redis connectivity
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// UnimplementedTaskServiceHandler returns CodeUnimplemented from all methods.
type UnimplementedTaskServiceHandler struct{}

func (UnimplementedTaskServiceHandler) Submit(context.Context, *connect.Request[v1.SubmitTaskRequest]) (*connect.Response[v1.SubmitTaskResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (UnimplementedTaskServiceHandler) Get(context.Context, *connect.Request[v1.GetTaskRequest]) (*connect.Response[v1.GetTaskResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (UnimplementedTaskServiceHandler) List(context.Context, *connect.Request[v1.ListTasksRequest]) (*connect.Response[v1.ListTasksResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (UnimplementedTaskServiceHandler) Cancel(context.Context, *connect.Request[v1.CancelTaskRequest]) (*connect.Response[v1.CancelTaskResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (UnimplementedTaskServiceHandler) Decide(context.Context, *connect.Request[v1.DecideRequest]) (*connect.Response[v1.DecideResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (UnimplementedTaskServiceHandler) GetArtifact(context.Context, *connect.Request[v1.GetArtifactRequest]) (*connect.Response[v1.GetArtifactResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (UnimplementedTaskServiceHandler) GetDiff(context.Context, *connect.Request[v1.GetDiffRequest]) (*connect.Response[v1.GetDiffResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (UnimplementedTaskServiceHandler) Wait(context.Context, *connect.Request[v1.WaitTaskRequest]) (*connect.Response[v1.WaitTaskResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
