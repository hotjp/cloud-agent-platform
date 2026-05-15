# task_089

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_089.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P3: Smart task decomposition — Optimize subtask generation using historical data


## 需求 (requirements)

Improve the task decomposition algorithm based on historical data: (1) Track decomposition patterns: which task characteristics (size, file count, complexity) lead to which subtask structures; (2) Add ent schema: decomposition_patterns(table: task_fingerprint, subtask_count, avg_parallelism, avg_dependencies, success_rate, avg_duration); (3) Modify eino_nodes.go AnalyzerNode to query historical patterns before decomposing; (4) For similar tasks (same language, same type, similar size): reuse proven decomposition structures; (5) Cost estimator improvement: use historical avg_cost per task_fingerprint to give more accurate estimates; (6) Add metrics: decomposition_pattern_hits, decomposition_custom_count



## 验收标准 (acceptance)


- New tasks query historical patterns before decomposing; Similar tasks reuse proven decomposition patterns; Cost estimates improve over time as more data accumulates




## 交付物 (deliverables)

- `ent/schema/decomposition_pattern.go` - New ent schema for tracking decomposition patterns
- `plugins/orchestrator/nodes.go` - Enhanced AnalyzerNode with historical pattern query
- `internal/observability/metrics/metrics.go` - Added decomposition pattern metrics
- `ent/schema/schema_test.go` - Updated test to include new tables

## 设计方案 (design)

1. **DecompositionPattern Schema**: Tracks task fingerprints (hash of language+type+size_bucket), subtask structures, success rates, avg_cost, avg_duration
2. **AnalyzerNode Enhancement**: Queries historical patterns by fingerprint before decomposing; reuses proven patterns when available
3. **Metrics**: `cap_decomposition_pattern_hits` (when pattern reused), `cap_decomposition_custom_count` (when custom decomposition created)
4. **Pattern Reuse**: When similar task fingerprint found, subtask structure is reused from history


## 验证证据（完成前必填）

- [x] **实现证明**: 
  - Created `ent/schema/decomposition_pattern.go` with task_fingerprint, subtask_structure, success_rate, avg_cost, avg_duration_ms fields
  - Enhanced `AnalyzerNode` in `nodes.go` to compute fingerprint and query historical patterns
  - Added `DecompositionPatternRepo` interface to Dependencies
  - Added `Fingerprint` and `HistoryPattern` fields to TaskContext
  - Added `reuseHistoricalPattern` method to MediumDecomposerNode
  - Added metrics `DecompositionPatternHits` and `DecompositionCustomCount`
- [x] **测试验证**: 
  - `go build ./...` - zero errors
  - `go vet ./plugins/orchestrator/... ./internal/observability/metrics/... ./ent/...` - zero issues
  - `go test ./plugins/orchestrator/...` - PASS
  - `go test ./ent/schema/...` - PASS
- [x] **影响范围**: New ent schema added; AnalyzerNode enhanced; no breaking changes to existing interfaces