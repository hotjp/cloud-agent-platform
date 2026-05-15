// Package main is the entry point for Cloud Agent Platform server.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	_ "github.com/lib/pq"

	"github.com/cloud-agent-platform/cap/ent"
	"github.com/cloud-agent-platform/cap/ent/outboxevent"
	"github.com/cloud-agent-platform/cap/internal/config"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/gateway"
	"github.com/cloud-agent-platform/cap/internal/infra/outbox"
	"github.com/cloud-agent-platform/cap/internal/infra/persistence"
	"github.com/cloud-agent-platform/cap/internal/observability/metrics"
	"github.com/cloud-agent-platform/cap/internal/observability/tracing"
	"github.com/cloud-agent-platform/cap/internal/orchestrator"
	"github.com/cloud-agent-platform/cap/internal/scheduler"
	"github.com/cloud-agent-platform/cap/internal/service"
	"github.com/cloud-agent-platform/cap/internal/storage"
	"github.com/cloud-agent-platform/cap/plugins/workermanager"
)

// --- Transaction Manager Adapters ---

type storageTMAdapter struct{ *storage.Storage }

func (a *storageTMAdapter) BeginTx(ctx context.Context) (service.TransactionManager, error) {
	tm, err := a.Storage.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	return &svcTMWrapper{tm: tm}, nil
}
func (a *storageTMAdapter) Commit(ctx context.Context) error   { return a.Storage.Commit(ctx) }
func (a *storageTMAdapter) Rollback(ctx context.Context) error { return a.Storage.Rollback(ctx) }

type svcTMWrapper struct{ tm storage.TransactionManager }

func (w *svcTMWrapper) BeginTx(context.Context) (service.TransactionManager, error) { return w, nil }
func (w *svcTMWrapper) Commit(ctx context.Context) error                             { return w.tm.Commit(ctx) }
func (w *svcTMWrapper) Rollback(ctx context.Context) error                           { return w.tm.Rollback(ctx) }
func (w *svcTMWrapper) Tx() *ent.Tx                                                 { return w.tm.Tx() }

type orchTMAdapter struct{ tm service.TransactionManager }

func (a *orchTMAdapter) BeginTx(ctx context.Context) (orchestrator.TransactionManager, error) {
	tm, err := a.tm.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	return &orchTMWrapper{tm: tm}, nil
}
func (a *orchTMAdapter) Commit(ctx context.Context) error   { return a.tm.Commit(ctx) }
func (a *orchTMAdapter) Rollback(ctx context.Context) error { return a.tm.Rollback(ctx) }

type orchTMWrapper struct{ tm service.TransactionManager }

func (w *orchTMWrapper) BeginTx(context.Context) (orchestrator.TransactionManager, error) {
	return w, nil
}
func (w *orchTMWrapper) Commit(ctx context.Context) error   { return w.tm.Commit(ctx) }
func (w *orchTMWrapper) Rollback(ctx context.Context) error { return w.tm.Rollback(ctx) }
func (w *orchTMWrapper) Tx() *ent.Tx {
	if txer, ok := w.tm.(interface{ Tx() *ent.Tx }); ok {
		return txer.Tx()
	}
	return nil
}

// --- Stub Agent Runner (fallback when Docker is unavailable) ---

type stubAgentRunner struct{}

func (s *stubAgentRunner) Run(ctx context.Context, subtask *domain.Subtask, task *domain.Task) (*orchestrator.AgentResult, error) {
	return &orchestrator.AgentResult{
		Summary:          fmt.Sprintf("Stub agent completed: %s", task.Goal),
		TokensUsed:       150,
		ExecutionDuration: 2 * time.Second,
	}, nil
}
func (s *stubAgentRunner) Type() string { return "stub" }

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("shutting down", zap.String("signal", sig.String()))
		cancel()
	}()

	cfg := config.MustLoad("config.yaml")
	logger.Info("Configuration loaded", zap.Int("port", cfg.Server.Port))

	// L1-Storage
	store, err := storage.New(cfg, logger)
	if err != nil {
		logger.Error("Failed to create storage", zap.Error(err))
		os.Exit(1)
	}
	defer store.Close(ctx)

	taskRepo := persistence.NewTaskRepository(store.Client(), logger)
	subtaskRepo := persistence.NewSubtaskRepository(store.Client(), logger)
	outboxWriter := outbox.NewOutboxWriter(store.Client(), logger)
	svcTM := &storageTMAdapter{store}

	// --- Scheduler Module (independent) ---
	var workerExecutor orchestrator.WorkerExecutor

	dockerBackend, dockerErr := workermanager.NewDockerBackend(
		workermanager.DefaultDockerBackendConfig(), logger,
	)
	if dockerErr != nil {
		logger.Warn("Docker backend unavailable, using stub agent runner",
			zap.Error(dockerErr),
		)
	} else {
		sched, schedErr := scheduler.New(scheduler.DefaultConfig(),
			scheduler.NewDockerBackend(dockerBackend, logger), logger,
		)
		if schedErr != nil {
			logger.Warn("Scheduler creation failed, using stub agent runner",
				zap.Error(schedErr),
			)
		} else {
			if err := sched.Start(ctx); err != nil {
				logger.Warn("Scheduler start failed", zap.Error(err))
			} else {
				logger.Info("Scheduler started (Docker backend)")
				adapterCfg := scheduler.DefaultAdapterConfig()
				adapterCfg.LLMAPIKey = cfg.LLM.ZhipuAPIKey
				if adapterCfg.LLMAPIKey == "" {
					logger.Warn("LLM API key not configured, cap-worker will fail on LLM calls")
				}
				workerExecutor = scheduler.NewOrchestratorAdapter(sched, adapterCfg)
				// Graceful shutdown: stop scheduler on context cancel
				go func() {
					<-ctx.Done()
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer stopCancel()
					sched.Stop(stopCtx)
				}()
			}
		}
	}

	// L4-Orchestrator
	var agentRunner orchestrator.AgentRunner
	if workerExecutor == nil {
		agentRunner = &stubAgentRunner{}
	}

	orch := orchestrator.NewOrchestrator(
		orchestrator.DefaultConfig(),
		taskRepo,
		subtaskRepo,
		outboxWriter,
		&orchTMAdapter{tm: svcTM},
		agentRunner,
		logger,
		workerExecutor,
		nil, // Guardian — TODO
	)

	// L4-Service
	serviceSvc := service.NewTaskService(service.TaskServiceInput{
		TaskRepo:     taskRepo,
		SubtaskRepo:  subtaskRepo,
		OutboxWriter: outboxWriter,
		Storage:      svcTM,
		Logger:       logger,
		Metrics:      metrics.NewRecorder(metrics.NewMetrics()),
		Tracer:       tracing.NewSpanHelper(),
	})

	// Outbox processor
	go startOutboxProcessor(ctx, store, taskRepo, orch, logger)

	// L5-Gateway
	gwCfg := gateway.Config{
		Port:         cfg.Server.Port,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		JWTSecret:    cfg.Authz.JWTSecret,
	}
	gatewaySvc := gateway.New(gwCfg, serviceSvc, logger)
	if err := gatewaySvc.Start(ctx); err != nil {
		logger.Error("Failed to start gateway", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("Server started", zap.Int("port", cfg.Server.Port))
	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	gatewaySvc.Stop(shutdownCtx)
	logger.Info("Server stopped")
}

// --- Outbox Processor ---

func startOutboxProcessor(ctx context.Context, store *storage.Storage, taskRepo domain.TaskRepository, orch orchestrator.Orchestrator, logger *zap.Logger) {
	logger.Info("outbox processor started")
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("outbox processor stopped")
			return
		case <-ticker.C:
			processOutbox(ctx, store, taskRepo, orch, logger)
		}
	}
}

func processOutbox(ctx context.Context, store *storage.Storage, taskRepo domain.TaskRepository, orch orchestrator.Orchestrator, logger *zap.Logger) {
	events, err := store.Client().OutboxEvent.Query().
		Where(outboxevent.StatusEQ("pending")).
		Order(ent.Asc(outboxevent.FieldCreatedAt)).
		Limit(10).
		All(ctx)
	if err != nil {
		return
	}
	for _, evt := range events {
		if evt.EventType == "TaskSubmittedV1" {
			taskID := evt.AggregateID
			if taskID == "" {
				var payload map[string]any
				if err := json.Unmarshal(evt.Payload, &payload); err == nil {
					if id, ok := payload["task_id"].(string); ok {
						taskID = id
					}
				}
			}
			if taskID == "" {
				continue
			}
			task, err := taskRepo.GetByID(ctx, taskID)
			if err != nil {
				logger.Error("task not found", zap.String("task_id", taskID))
				continue
			}
			if err := orch.StartTask(ctx, task); err != nil {
				logger.Error("orchestrator start failed", zap.String("task_id", taskID), zap.Error(err))
			} else {
				logger.Info("orchestrator started task", zap.String("task_id", taskID))
			}
		}
		store.Client().OutboxEvent.UpdateOneID(evt.ID).
			SetStatus("processed").
			SetProcessedAt(time.Now().UnixNano()).
			Save(ctx)
	}
}
