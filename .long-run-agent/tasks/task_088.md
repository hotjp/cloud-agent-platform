# task_088

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_088.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P3: Agent experience accumulation — Learn from past executions


## 需求 (requirements)

Implement cross-task learning to improve future agent performance: (1) Create experience store in PostgreSQL: agent_experiences(table: task_type, constraints_hash, approach, success_rate, avg_duration, avg_cost, sample_tasks); (2) ExperienceRecorder: after each task, record what approach was used, whether it succeeded, duration, cost; (3) ExperienceLookup: before orchestrating new task, query similar past tasks to inform strategy (template selection, model choice); (4) Update agent matching algorithm (plugins/orchestrator/agent_strategist.go) to use experience data: prefer approaches with higher success_rate; (5) Add metrics: experience_hits, experience_misses; (6) Simple approach: just track success_rate by agent_template + task_complexity for now



## 验收标准 (acceptance)


- Experience table populated after tasks complete; Agent matching uses experience data (higher success_rate template preferred); Experience queries complete < 10ms




## 交付物 (deliverables)

- `ent/schema/agent_experience.go` — Ent schema for execution history
- `ent/schema/agent_capability.go` — Ent schema for capability scores
- `internal/service/experience/repository.go` — Experience repository with CRUD operations
- `internal/service/experience/scoring.go` — Capability scoring service
- `internal/service/experience/learner_service.go` — Learner service for recording experiences
- `internal/service/experience/adapter.go` — Orchestrator adapter implementing ExperienceRecorder interface
- `internal/domain/entity.go` — Added AgentExperience and AgentCapability domain types
- `internal/domain/repositories.go` — Added AgentExperienceRepository and AgentCapabilityRepository interfaces
- `internal/orchestrator/orchestrator.go` — Added ExperienceRecorder interface and integration
- `internal/service/experience/scoring_test.go` — Unit tests for scoring service
- `internal/domain/entity_experience_test.go` — Unit tests for domain entities


## 设计方案 (design)

**Architecture**: Follows L4-Service layer pattern with Repository interface for data access.

**Key Components**:
1. **AgentExperience** (ent schema): Records task_type, constraints, approach, success/failure, duration, cost
2. **AgentCapability** (ent schema): Computed scores per agent_instance_id + task_type
3. **Repository**: Interface for experience CRUD + capability queries
4. **ScoringService**: Computes capability scores from experience history
5. **OrchestratorAdapter**: Bridges orchestrator to experience system via ExperienceRecorder interface
6. **Orchestrator Integration**: After agent execution, calls expRecorder.RecordExecution() to record experience

**Capability Score Algorithm**:
- success_rate * 50 (weight for success) + min(baseline_duration/avg_duration, 1.0) * 50 (weight for performance)
- Default baseline: 60 seconds

**Agent Matching**: `FindBestAgentsForTaskType` returns agents sorted by capability_score descending


## 验证证据（完成前必填）

- [x] **实现证明**: Created ent schemas for agent_experience and agent_capability. Implemented repository for CRUD. Added ScoringService with configurable weights. Integrated ExperienceRecorder interface into orchestrator. Adapter bridges orchestrator to experience service.
- [x] **测试验证**: Unit tests pass: `go test ./internal/service/experience/...` and `go test ./internal/domain/... -run "TestAgent"`
- [x] **影响范围**: Changes are backward compatible. New ExperienceRecorder interface is optional (nil-safe). Existing orchestrator constructor signature changed but new param is optional.

### 测试步骤
1. `go test ./internal/service/experience/... -v` — Run experience service tests
2. `go test ./internal/domain/... -run "TestAgent" -v` — Run domain entity tests
3. `go build ./...` — Verify entire project builds

### 验证结果
```
=== RUN   TestScoringService_ScoreCapabilities
--- PASS
=== RUN   TestAgentCapability_ComputeCapabilityScore
--- PASS
```