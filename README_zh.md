# Cloud Agent Platform

中文 | **[English](README.md)**

云端多 Agent 协作执行平台，用于自动化代码开发。

> 用户提交一个开发任务，平台自动拆解成多个子任务，分配给不同角色的 Agent 并行/串行执行，最终产出代码改动并推送到 Git 分支。

## 是什么

Cloud Agent Platform（CAP）是一个生产级 Go 后端，通过编排多个 AI Agent 来自主完成软件开发任务。像一个"项目经理 + 专业工程师团队"：平台负责任务拆解和调度，Agent（observer / strategist / executor / guardian / tester）负责分析、编码、审查和测试。

## 三种使用方式

| 方式 | 场景 | 说明 |
|------|------|------|
| **MCP 协议**（推荐） | Claude Code / Kimi CLI | 通过 MCP Tools 与平台原生交互 |
| **REST API** | CI/CD 脚本 | HTTP/JSON 用于自动化流水线 |
| **WebSocket** | 实时监控 | 任务状态和 Agent 日志实时推送 |

## 技术栈

| 类别 | 选型 | 用途 |
|---|---|---|
| API | `connect-go` | gRPC + HTTP 双模 |
| Agent 编排 | `eino` (cloudwego) | Graph + ADK + MCP，生产验证 |
| ORM | `ent` (pgx) | Schema 驱动，代码生成 |
| 配置 | `koanf` | YAML + 环境变量，显式依赖注入 |
| 日志 | `zap` | 高性能结构化日志 |
| JSON | `sonic` | 比标准库快 10-20x |
| 限流熔断 | `sentinel-go` | QPS/并发/系统自适应 |
| Git | `go-git` | 纯 Go，无需 git 二进制 |
| ID | `oklog/ulid` | 全局唯一、按时间排序 |
| 缓存/锁 | `go-redis/v9` | 热上下文 + 分布式锁 |
| 冷存储 | MinIO | 产出物归档 |
| Worker 沙箱 | CubeSandbox / Docker | 60ms 启动 / 硬件级隔离 |
| 可观测性 | OpenTelemetry + Prometheus | 链路追踪 + Metrics |
| 数据库迁移 | `golang-migrate` | 版本化 SQL 迁移 |
| 测试 | testify + gomock + dockertest + miniredis | 单元 / 集成 / E2E |

## 架构

```
L5-Gateway → L3-Authz → L4-Service → L2-Domain → L1-Storage
```

```
cmd/server/main.go           # 入口，依赖注入组装
internal/
  gateway/                   # L5: Connect handler + WebSocket Hub
  authz/                     # L3: API Key + RBAC + sentinel-go 限流
  service/                   # L4: 业务编排（含 interfaces.go）
  domain/                    # L2: Task/Subtask 状态机（零外部依赖）
  storage/                   # L1: ent + PostgreSQL + Redis + MinIO
plugins/
  orchestrator/              # Eino 编排图（任务拆解 + Agent 调度）
  llmrouter/                 # LLM 多模型路由（Claude / GLM）
  workermanager/             # Worker 生命周期（CubeSandbox / Docker）
  mcpserver/                 # MCP Server（9 Tools + 4 Resources）
  gitclient/                 # go-git（clone/commit/push）
  tools/                     # Agent 工具集（文件/Git/命令/LLM）
api/cap/v1/                  # TaskService Protobuf 定义
```

## 任务状态机（9 态）

```
pending → decomposing → dispatched → running → reviewing → confirming → completed
                                                                       → failed
                                                                       → cancelled
```

## 接口

所有 RPC 同时暴露为 REST + MCP Tools：

| RPC | MCP Tool | 说明 |
|-----|----------|------|
| SubmitTask | task_submit | 提交开发任务 |
| GetTask | task_status | 查询任务状态 |
| ListTasks | task_list | 列出任务 |
| CancelTask | task_cancel | 取消任务 |
| DecideTask | task_decide | 批准/拒绝人工确认 |
| GetDiff | task_diff | 获取代码 diff |
| - | task_wait | 阻塞等待任务完成 |
| ListAgentTemplates | agent_templates | 列出可用 Agent 角色 |
| GetPlatformStatus | platform_status | 平台状态 |

## Agent 角色

| 角色 | 职责 | 默认模型 |
|------|------|----------|
| observer | 代码分析、依赖识别 | Claude Sonnet |
| strategist | 策略规划、方案设计 | Claude Sonnet |
| executor | 代码编写和修改 | Claude Sonnet |
| guardian | 安全审查、约束检查 | GLM-5.1 |
| tester | 测试编写和执行 | GLM-5.1 |
| researcher | 技术研究、最佳实践 | Claude Sonnet |

## 文档

| 文档 | 用途 |
|---|---|
| [docs/Cloud-Agent-Platform.md](docs/Cloud-Agent-Platform.md) | 完整技术设计（业务 + API + Schema + 实现参考） |
| [CLAUDE.md](CLAUDE.md) | 架构约束 + 编码规则（始终加载） |
| [docs/architecture.md](docs/architecture.md) | 框架技术规范（配置/日志/遥测/测试） |
| [docs/TASK-BREAKDOWN.md](docs/TASK-BREAKDOWN.md) | 任务定义（含完整上下文） |
| [lra.md](lra.md) | LRA 命令参考 |

## 任务管理（LRA）

```bash
lra ready              # 查看可认领任务
lra claim <id>         # 原子认领
lra show <id>          # 查看任务详情
# 实现 → 测试 → 提交
lra set <id> completed
lra check <id>         # 质量门控
lra set <id> truly_completed
```

## License

MIT
