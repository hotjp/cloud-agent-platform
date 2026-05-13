# Cloud Agent Platform

**[中文](README_zh.md)** | English

Cloud-based multi-agent collaboration platform for automated code development.

> Users submit a development task. The platform automatically decomposes it into subtasks, assigns them to different role agents for parallel/sequential execution, and produces code changes pushed to a Git branch.

## What

Cloud Agent Platform (CAP) is a production-grade Go backend that orchestrates multiple AI agents to complete software development tasks autonomously. Think of it as a "project manager + specialist engineers" team: the platform handles task decomposition and scheduling, while agents (observer / strategist / executor / guardian / tester) handle analysis, coding, review, and testing.

## Three Usage Modes

| Mode | Use Case | Description |
|------|----------|-------------|
| **MCP Protocol** (recommended) | Claude Code / Kimi CLI | Native agent integration via MCP Tools |
| **REST API** | CI/CD scripts | HTTP/JSON for automation pipelines |
| **WebSocket** | Real-time monitoring | Live task status + agent logs for dashboard |

## Tech Stack

| Category | Choice | Purpose |
|---|---|---|
| API | `connect-go` | gRPC + HTTP dual-mode |
| Agent orchestration | `eino` (cloudwego) | Graph + ADK + MCP, production-validated |
| ORM | `ent` (pgx) | Schema-driven, code generation |
| Config | `koanf` | YAML + env, explicit DI |
| Logging | `zap` | High-performance structured logging |
| JSON | `sonic` | 10-20x faster than stdlib |
| Rate limiting | `sentinel-go` | QPS/concurrency/adaptive |
| Git | `go-git` | Pure Go, no binary dependency |
| ID | `oklog/ulid` | Globally unique, time-sortable |
| Cache/Lock | `go-redis/v9` | Hot context + distributed lock |
| Cold storage | MinIO | Artifact archive |
| Worker sandbox | CubeSandbox / Docker | 60ms startup / hardware isolation |
| Observability | OpenTelemetry + Prometheus | Tracing + metrics |
| DB Migration | `golang-migrate` | Versioned SQL migration |
| Testing | testify + gomock + dockertest + miniredis | Unit / integration / E2E |

## Architecture

```
L5-Gateway  →  L3-Authz  →  L4-Service  →  L2-Domain  →  L1-Storage
```

```
cmd/server/main.go           # Entry point, DI assembly
internal/
  gateway/                   # L5: Connect handler + WebSocket Hub
  authz/                     # L3: API Key + RBAC + sentinel-go
  service/                   # L4: Business orchestration (interfaces.go)
  domain/                    # L2: Task/Subtask state machines (zero deps)
  storage/                   # L1: ent + PostgreSQL + Redis + MinIO
plugins/
  orchestrator/              # Eino graph (task decomposition + scheduling)
  llmrouter/                 # Multi-model LLM routing (Claude / GLM)
  workermanager/             # Worker lifecycle (CubeSandbox / Docker)
  mcpserver/                 # MCP Server (9 Tools + 4 Resources)
  gitclient/                 # go-git (clone/commit/push)
  tools/                     # Agent tool set (file/git/cmd/llm)
api/cap/v1/                  # TaskService Protobuf definitions
```

## Task State Machine (9 states)

```
pending → decomposing → dispatched → running → reviewing → confirming → completed
                                                                       → failed
                                                                       → cancelled
```

## API

All RPCs are exposed simultaneously as REST + MCP Tools:

| RPC | MCP Tool | Description |
|-----|----------|-------------|
| SubmitTask | task_submit | Submit a development task |
| GetTask | task_status | Query task status |
| ListTasks | task_list | List tasks |
| CancelTask | task_cancel | Cancel a task |
| DecideTask | task_decide | Approve/reject human confirmation |
| GetDiff | task_diff | Get code diff |
| - | task_wait | Block until task completes |
| ListAgentTemplates | agent_templates | List available agent roles |
| GetPlatformStatus | platform_status | Platform status |

## Agent Roles

| Role | Responsibility | Default Model |
|------|---------------|---------------|
| observer | Code analysis, dependency mapping | Claude Sonnet |
| strategist | Strategy planning, solution design | Claude Sonnet |
| executor | Code writing and modification | Claude Sonnet |
| guardian | Security review, constraint checking | GLM-5.1 |
| tester | Test writing and execution | GLM-5.1 |
| researcher | Technical research, best practices | Claude Sonnet |

## Documentation

| Document | Purpose |
|---|---|
| [docs/Cloud-Agent-Platform.md](docs/Cloud-Agent-Platform.md) | Full technical design (business + API + schema + implementation) |
| [CLAUDE.md](CLAUDE.md) | Architecture constraints + coding rules (always loaded) |
| [docs/architecture.md](docs/architecture.md) | Framework technical specs (config/logging/telemetry/testing) |
| [docs/TASK-BREAKDOWN.md](docs/TASK-BREAKDOWN.md) | Task definitions with full context |
| [lra.md](lra.md) | LRA command reference |

## Task Management with LRA

```bash
lra ready              # Find available tasks
lra claim <id>         # Claim atomically
lra show <id>          # Read task details
# Implement → Test → Commit
lra set <id> completed
lra check <id>         # Quality gates
lra set <id> truly_completed
```

## License

MIT
