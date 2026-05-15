# task_072

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_072.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P0: main.go DI Assembly — Wire all components together


## 需求 (requirements)

Fix cmd/server/main.go dependency injection. Current bug: service.NewTaskService(TaskServiceInput{Logger: logger}) only sets Logger, all other fields (TaskRepo/SubtaskRepo/OutboxWriter/Storage/Metrics/Tracer) are nil — will panic on Submit(). Required: (1) Call storage.New(cfg, logger) to get *Storage; (2) Wire TaskRepo=infra/persistence.NewTaskRepository(client), SubtaskRepo=NewSubtaskRepository(client), OutboxWriter=infra/outbox.NewOutboxWriter(client); (3) Wire metrics.Recorder and tracing.NewSpanHelper from observability package; (4) Pass all to TaskServiceInput; (5) Start OutboxPoller as background goroutine (poller.Start(ctx)); (6) Start CleanupWorker; (7) Call cfg.Validate(); (8) Pass CORSOrigins from config.Gateway to gateway.Config; (9) Wire observability shutdown on exit



## 验收标准 (acceptance)


- go build ./... passes; service.Submit() can be called without panic; OutboxPoller goroutine is running; docker-compose up server starts and connects to PG/Redis




## 交付物 (deliverables)

- `cmd/server/main.go` — Fixed DI assembly with all components wired
- `internal/config/config.go` — Added GatewayConfig for CORSOrigins
- `internal/storage/storage.go` — Added DB() method for OutboxPoller



## 设计方案 (design)

1. Added `GatewayConfig` to `internal/config/config.go` with `CORSOrigins []string` field
2. Added `DB()` method to `Storage` in `internal/storage/storage.go` to expose `*sql.DB` for OutboxPoller
3. Rewrote `cmd/server/main.go` with full DI assembly:
   - Created `transactionManagerAdapter` and `entTxnAdapter` to bridge `storage.TransactionManager` → `service.TransactionManager`
   - Wire all repos, outbox writer, metrics, tracer into `TaskServiceInput`
   - Start OutboxPoller and CleanupWorker as background goroutines
   - Initialize tracing with proper shutdown
   - Initialize metrics provider and start metrics server
   - Pass CORSOrigins from config to gateway


## 验证证据（完成前必填）

- [x] **实现证明**: cmd/server/main.go完全重写，包含所有组件的DI连接
- [x] **测试验证**: go build ./... 和 go vet ./... 均通过，相关包测试通过
- [x] **影响范围**: 无负面影响，所有依赖包测试正常

### 测试步骤
1. `go build ./...` - 编译通过
2. `go vet ./...` - 静态分析通过
3. `go test ./internal/storage/... ./internal/config/... ./internal/service/...` - 相关包测试通过

### 验证结果
```
go build ./...  # 通过
go vet ./...    # 通过
go test ./internal/storage/... ./internal/config/... ./internal/service/...  # 全部通过
```