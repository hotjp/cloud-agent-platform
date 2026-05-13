# Cloud Agent Platform - 任务拆分规范

> 完整业务设计见 [Cloud-Agent-Platform.md](Cloud-Agent-Platform.md)，任务拆分方法论见 [TASK-PROMPT.md](TASK-PROMPT.md)。

本文档定义 Cloud Agent Platform 的任务拆分策略。每个 Task = 一个业务模块在某一架构层的完整实现，任务描述采用五段式（目标/契约参考/依赖接口/约定/验收标准）。

> **TODO**: 参照 TASK-PROMPT.md 中的拆分方法论，为以下模块树的每个节点创建具体 Task。

---

## 一、模块树

```
Cloud Agent Platform
│
├── 共享内核 (Shared Kernel)
│   ├── 通用类型 (TaskStatus, SubtaskType, AgentRole 枚举)
│   ├── 错误码定义 (L1_001 ~ L5_802)
│   ├── 领域事件协议 (DomainEvent 结构 + OutboxWriter 接口)
│   └── ULID 生成
│
├── 任务管理 (Task Management)
│   ├── Task ─────── 9态状态机 (pending/decomposing/dispatched/running/reviewing/confirming/completed/failed/cancelled)
│   └── Subtask ──── 依赖 Task (FK: task_id), DAG 依赖执行
│
├── 编排调度 (Orchestration)          [plugins/orchestrator]
│   ├── 复杂度路由 ─── 简单/中等/复杂 三路 Eino Graph
│   ├── Agent 匹配 ─── 能力评分 × 历史成功率 × 成本效率
│   └── DAG 执行器 ─── 按 Subtask.Dependencies 调度并行/串行
│
├── Agent 执行 (Agent Execution)      [plugins/orchestrator + workermanager]
│   ├── ReAct 循环 ─── Thought→Action→Observation，最多15步
│   ├── Worker 沙箱 ── CubeSandbox（主）/ Docker（降级）
│   └── 6个角色 ────── observer/strategist/executor/guardian/tester/researcher
│
├── 上下文管理 (Context Management)
│   ├── 三层存储 ───── Redis（热）/ PostgreSQL（温）/ MinIO（冷）
│   └── 三级压缩 ───── L1规则 → L2 Embedding去重 → L3 LLM智能压缩
│
├── 工具系统 (Tool System)             [plugins/tools]
│   ├── 文件工具 ───── read_file / write_file / edit_file / list_files / search_code
│   ├── Git 工具 ───── git_status / git_diff / git_commit / git_push
│   ├── 命令工具 ───── execute_command（白名单+超时）
│   └── LLM 工具 ───── ask_llm（技术研究，不计任务token）
│
├── 产出物管理 (Artifact Management)
│   └── Artifact ──── MinIO 存储，签名 URL，90天 TTL
│
├── 人工审批 (Human Approval)
│   └── confirming ── Guardian 触发 → WebSocket 推送 → 5分钟超时自动拒绝
│
└── 监控查询 (Monitoring)
    ├── WebSocket ──── 实时状态推送（任务级/Agent级/平台级事件）
    └── MCP Server ─── 9 Tools + 4 Resources [plugins/mcpserver]
```

---

## 二、模块间依赖

```
TaskManagement ◄──── OrchestrationScheduling
                              │
                              ▼
                     AgentExecution ──► ContextManagement
                              │
                              ▼
                          ToolSystem
                              │
                              ▼
                       ArtifactManagement
                              │
                     HumanApproval (Guardian 触发)
```

---

## 三、任务清单概览（模块 × 层矩阵）

| 模块 | L2-Domain | L1-Storage | L4-Service | Plugin | L5-Gateway |
|------|-----------|-----------|------------|--------|-----------|
| 共享内核 | T03,T04,T05 | - | T05 | - | - |
| 基础设施 | - | T10,T11,T12,T13 | - | - | - |
| 任务管理 | T20 | T21 | T22 | - | T23 |
| 编排调度 | - | - | - | T30 | - |
| Agent执行 | - | - | - | T31(ReAct),T32(Worker) | - |
| Git集成 | - | - | - | T33 | - |
| WebSocket | - | - | - | - | T40 |
| Eino全图 | - | - | - | T41 | - |
| 6Agent角色 | - | - | - | T42 | - |
| Agent匹配 | - | - | - | T43 | - |
| 上下文管理 | T44 | T45 | T46 | - | - |
| 工具系统 | - | - | - | T47 | - |
| 人工审批 | T48 | - | T48 | - | - |
| MCP Server | - | - | - | T49 | - |
| Worker池 | - | - | - | T60 | - |
| LLM路由 | - | - | - | T61 | - |
| 上下文压缩 | - | - | - | T62 | - |
| 冷存储 | - | T63 | - | - | - |
| Metrics | - | - | - | - | T64 |
| 链路追踪 | - | - | - | - | T65 |

---

## 四、阶段规划

### Phase 0：骨架 + 最小闭环（T00-T33）

目标：curl 提交任务 → 单 Agent 执行 → Git push 结果 → WebSocket 推送状态

| Task | 模块 | 层 | 说明 |
|------|------|----|------|
| T00 | 基础设施 | - | 项目初始化（go mod, 目录结构, Taskfile） |
| T01 | 基础设施 | - | 配置系统（koanf, 所有 Config struct） |
| T02 | 基础设施 | - | Protobuf 定义 + 代码生成（TaskService 8 RPC） |
| T03 | 共享内核 | L2 | 通用类型 + ULID（TaskStatus, SubtaskType, AgentRole 枚举） |
| T04 | 共享内核 | L2 | 错误码（L1_001 ~ L5_802） |
| T05 | 共享内核 | L2+interfaces | 领域事件 + 状态机框架 + 所有仓库接口 |
| T10 | 基础设施 | L1 | PostgreSQL 连接 + 迁移框架 + 事务管理 |
| T11 | 基础设施 | L1 | DB 迁移脚本（tasks/subtasks/audit_logs/outbox_events 4张表） |
| T12 | 基础设施 | L1 | Redis 客户端 + 缓存层 |
| T13 | 基础设施 | L1 | Outbox 系统（写入器 + 轮询器 + Redis Stream 发布） |
| T20 | 任务管理 | L2 | Task + Subtask 领域模型（9态状态机，所有转换规则） |
| T21 | 任务管理 | L1 | ent Schema（Task/Subtask/AuditLog）+ Repository 实现 |
| T22 | 任务管理 | L4 | TaskService（Submit/Get/List/Cancel/Decide 5个方法） |
| T23 | 任务管理 | L5 | Gateway（connect-go + JWT + sentinel-go 限流） |
| T30 | 编排调度 | Plugin | 最小编排（单 Agent 路径：pending→running→completed） |
| T31 | Agent执行 | Plugin | ReAct Agent（LLM 调用循环 + read_file + write_file） |
| T32 | Worker | Plugin | **双沙箱并行实现**：Docker + CubeSandbox 同时实现 `SandboxBackend` 接口，config 切换（`sandbox.backend: docker\|cubesandbox`） |
| T33 | Git集成 | Plugin | go-git（clone + commit + push 到 result branch） |
| T40 | 监控 | L5 | WebSocket Hub（房间管理 + 所有事件类型） |

### Phase 1：多 Agent 协作（T41-T49）

目标：任务自动拆解 → 多 Agent 协作 → Guardian 审查 → 人工审批

| Task | 模块 | 层 | 说明 |
|------|------|----|------|
| T41 | 编排调度 | Plugin | Eino Graph 全图（3路路由：简单/中等/复杂） |
| T42 | Agent执行 | Plugin | 6个 Agent 角色（prompt + 工具集分配） |
| T43 | 编排调度 | Plugin | Agent 匹配算法（能力评分 + 历史 + 成本） |
| T44 | 上下文管理 | L2 | 上下文领域模型（TaskContext, FileState, ConversationTurn） |
| T45 | 上下文管理 | L1 | Redis 热层（分布式锁 + Redlock + 看门狗） |
| T46 | 上下文管理 | L4 | 上下文传递（full/summary/delta 三模式） |
| T47 | 工具系统 | Plugin | 全工具集（文件/Git/命令/LLM + 角色权限表） |
| T48 | 人工审批 | L2+L4 | Guardian 触发 + confirming 状态 + 超时 + WebSocket 推送 |
| T49 | MCP | Plugin | MCP Server（9 Tools + 4 Resources，SSE 传输） |

### Phase 2：Worker 层 + 生产化（T60-T65）

| Task | 模块 | 层 | 说明 |
|------|------|----|------|
| T60 | Worker | Plugin | Worker 池生产化（预热/扩缩容/健康检查，复用 T32 SandboxBackend 接口） |
| T61 | LLM | Plugin | LLM 路由插件（Claude/GLM 自适应升降级） |
| T62 | 上下文 | Plugin | 上下文压缩引擎（**L1规则 + L3 LLM，L2 Embedding 暂跳过**） |
| T63 | 产出物 | L1 | MinIO 冷存储（上传/下载/签名URL/90天TTL） |
| T64 | 监控 | L5 | 业务 Metrics（`cap_*` Prometheus 指标全集） |
| T65 | 监控 | L5 | 链路追踪（OTel Spans：submit/decompose/execute/llm/tool） |

---

## 五、依赖关系总图

```
Phase 0 (部分可并行):
  T00 ──► T01 ∥ T02 ∥ T03
  T03 ──► T04 ──► T05
  T01 ──► T10 ──► T11 ∥ T13
  T01 ──► T12
  T05 + T10 + T12 ──► T13
  T05 ──► T20 ──► T21 ──► T22 ──► T23
  T22 + T13 ──► T30 ──► T31
  T01 ──► T32
  T01 ──► T33
  T23 ──► T40

Phase 1:
  T30 ──► T41 ──► T42 ──► T43
  T05 ──► T44 ──► T45 ──► T46
  T05 ──► T47
  T42 + T47 + T45 ──► T48
  T22 ──► T49

Phase 2:
  T32 ──► T60
  T31 ──► T61
  T46 ──► T62
  T12 ──► T63
  T23 ──► T64 ∥ T65
```

### 可并行任务组

| 阶段 | 可并行的任务 | 前提条件 |
|------|-------------|----------|
| Phase 0 | T01 ∥ T02 ∥ T03 | T00 完成 |
| Phase 0 | T10 ∥ T12 | T01 完成 |
| Phase 0 | T11 ∥ T13 | T10 完成 |
| Phase 0 | T32 ∥ T33 ∥ T40 | 各自前置完成 |
| Phase 1 | T41 ∥ T44 ∥ T47 | T05 + T30 完成 |
| Phase 2 | T60 ∥ T61 ∥ T62 ∥ T63 ∥ T64 ∥ T65 | Phase 1 完成 |

