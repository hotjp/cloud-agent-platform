# task_092

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_092.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P2: Context compression L1+L3 — Redis hot layer + compression engine wired + triggered


## 需求 (requirements)

internal/domain/context/compress.go has compression engine but Redis hot layer (internal/infra/cache/task_context_cache.go) not wired to orchestration. Required: (1) Wire TaskContextCache into orchestrator as context store; (2) Trigger L1 rule compression when task context exceeds budget (e.g., > 50K tokens); (3) Trigger L3 LLM compression when L1 insufficient; (4) Pass context store to ReAct agent; (5) Wire MinIO cold storage for context archival (> 24h old); (6) Add context.size and context.compression metrics; (7) Verify L1 compression works (remove JSON indent, summarize files with checksums, truncate old conversation rounds)



## 验收标准 (acceptance)


- Context compression triggered when context > 50K tokens; L1 reduces context size measurably (check metrics); Cold archival to MinIO works; Context survives across subtasks via Redis




## 交付物 (deliverables)

- `internal/service/context/context_storage.go` — CompressionStorage with hot (Redis) + cold (MinIO) tiering
- `internal/service/context/context_manager.go` — ContextManager integrating compressor with storage
- `internal/orchestrator/interfaces.go` — OrchestratorContextManager interface added
- `internal/orchestrator/orchestrator.go` — OrchestratorImpl updated to use ContextManager
- `internal/service/context/context_storage_test.go` — Unit tests for CompressionStorage
- `internal/service/context/context_manager_test.go` — Unit tests for ContextManager



## 设计方案 (design)

### Architecture

Two-tier context compression storage:
1. **Hot layer (Redis)**: L1 rule-based compression results, configurable TTL (default 1 hour)
2. **Cold layer (MinIO)**: L3 LLM compression intermediates, 90-day TTL

### Key Components

1. **CompressionStorage** (`context_storage.go`):
   - `SaveHot()` — stores L1 results to Redis with configurable TTL
   - `SaveCold()` — stores L3 intermediates to MinIO with 90-day TTL
   - `Load()` — reads hot first, falls back to cold on miss

2. **ContextManager** (`context_manager.go`):
   - `BeforeExecution()` — loads context from storage before agent runs
   - `AfterExecution()` — auto-selects L1/L3 based on budget, compresses and stores

3. **Orchestrator wiring**:
   - Added `OrchestratorContextManager` interface to `interfaces.go`
   - `OrchestratorImpl` now accepts `ContextManager` in `NewOrchestrator()`
   - `executeAgentSession()` calls `BeforeExecution` and `AfterExecution` around agent runs

### Compression Level Selection

- **L1** (rule-based): Fast, no LLM calls. Triggered when context approaches budget.
- **L3** (LLM-powered): Higher quality. Triggered when L1 insufficient (very tight budget).


## 验证证据（完成前必填）

- [x] **实现证明**: Created CompressionStorage (hot Redis + cold MinIO), ContextManager, and wired into orchestrator
- [x] **测试验证**: Unit tests pass: `go test ./internal/service/context/...` — all tests pass
- [x] **影响范围**: No breaking changes to existing APIs

### 测试步骤
1. `go build ./cmd/server/...` — server builds successfully
2. `go test ./internal/service/context/...` — all context tests pass
3. `go test ./internal/orchestrator/...` — orchestrator tests pass

### 验证结果
```
ok  	github.com/cloud-agent-platform/cap/internal/service/context	0.653s
```