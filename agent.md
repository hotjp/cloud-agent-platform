# Cloud Agent Platform - Go Production Stack

## 技术栈

### 核心框架
| 组件 | 库 | 用途 |
|---|---|---|
| API 协议 | `connect-go` | Connect (gRPC + HTTP 双模) |
| Protobuf | `buf` + `protoc-gen-go` | API 定义与代码生成 |
| API 文档 | `buf` → TS Client | 不 Swagger，用 buf 生成 TS 客户端给前端/Agent |
| ORM | `ent` | 数据库模型、迁移、查询（底层 pgx） |
| ID 生成 | `oklog/ulid` | 全局唯一、按时间排序的 ID |
| Agent 编排 | `eino` (github.com/cloudwego/eino) | Graph + ADK + MCP，字节跳动开源，生产验证 |

### 存储
| 组件 | 库 | 用途 |
|---|---|---|
| PostgreSQL | `pgx/v5` | 主数据库驱动（ent 底层使用） |
| Redis Sentinel | `go-redis/v9` | 热上下文 + 分布式锁 + 事件流 |
| MinIO | `minio-go` | 冷存储 + 产出物归档 |

### 可观测性
| 组件 | 库 | 用途 |
|---|---|---|
| 日志 | `zap` | 高性能结构化日志，与 OTel 集成成熟 |
| 链路追踪 | `opentelemetry-go` | OTLP 导出，概率采样 |
| Metrics | `opentelemetry-go` + Prometheus | 请求延迟、错误率、业务指标 |

### 基础设施
| 组件 | 库 | 用途 |
|---|---|---|
| 配置 | `koanf` | 多源配置加载（YAML + 环境变量），显式依赖注入 |
| 数据库迁移 | `golang-migrate` | Schema 版本管理 |
| HTTP 框架 | `net/http` (标准库) | Connect 底层使用 |
| JSON 序列化 | `sonic` | 比标准库快 10-20x，drop-in 替换 |
| 限流熔断 | `sentinel-go` | QPS/并发/系统自适应，阿里云开源 |
| Git 操作 | `go-git` | 纯 Go 实现，Worker 不需要 git 二进制 |
| Worker 沙箱 | CubeSandbox / Docker | 60ms 启动 / 硬件级隔离，降级到 Docker |

### 测试
| 组件 | 库 | 用途 |
|---|---|---|
| 断言 | `testify` | 断言 + suite |
| Mock | `gomock` | 接口 mock 生成 |
| 集成测试 | `dockertest` | PostgreSQL / Redis 独立容器，自动管理生命周期 |
| Redis Mock | `miniredis` | 单元测试 Redis mock |

---

## 架构概览：5层核心 + N插件

```
依赖方向：L5-Gateway → L3-Authz → L4-Service → L2-Domain → L1-Storage
```

### 核心设计原则
- 核心层定义接口，插件层实现接口，通过依赖注入连接
- **禁止核心层 import 插件层具体实现**
- L2-Domain 零外部依赖（纯 Go struct + 标准库）

### 分层职责
| 层 | 职责 | 关键约束 |
|---|---|---|
| L5-Gateway | TLS终止、协议适配、中间件(Recover/Metrics/CORS)、WebSocket Hub | JWT仅解密不验证，调用L3 |
| L3-Authz | API Key + RBAC、sentinel-go 限流、身份验证 | 所有RPC必须通过L3才能到L4（Fail Fast） |
| L4-Service | 输入校验、事务边界、任务编排、上下文管理、工具调度 | 不重复验证权限，通过interface依赖插件 |
| L2-Domain | Task/Subtask 状态机、事件收集(Outbox)、业务不变量 | 纯Go struct，零外部依赖 |
| L1-Storage | Ent实现、事务管理、Outbox轮询、事件转发Redis | Outbox同库同事务 |

### 插件层（接口倒置）
- 接口定义在 L4-Service（`interfaces.go`），实现在 `plugins/` 目录
- CAP 核心插件：Eino 编排器、LLM 路由、Worker 管理器、MCP Server、go-git、工具集

---

## 项目结构

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
  llmrouter/                 # LLM 多模型路由（Claude/GLM 自适应）
  workermanager/             # Worker 生命周期（CubeSandbox / Docker）
  mcpserver/                 # MCP Server（9 Tools + 4 Resources）
  gitclient/                 # go-git（clone/commit/push）
  tools/                     # Agent 工具集（文件/Git/命令/LLM）
api/cap/v1/                  # TaskService Protobuf 定义
```

---

## 代码生成规则

### 错误码格式
`L{层号}{3位序号}`，范围：L1=[001,199], L2=[200,399], L3=[400,599], L4=[600,799], L5=[800,999]

### 领域事件
- 格式：`{Aggregate}{Action}V{Version}`
- 必须包含：event_id(ULID), aggregate_type, aggregate_id, event_type, payload, occurred_at, idempotency_key, version
- 通过 Outbox 模式发布（事务内写入，后台轮询转发 Redis Stream）

### 状态机
- 声明式定义（states, transitions, guards, actions）
- 每次转换自动 increment_version（乐观锁）

### 配置管理
- 使用 `koanf` 加载，禁止全局单例
- 配置结构体显式定义，通过构造函数注入
- 支持 YAML 文件 + 环境变量覆盖（`APP_` 前缀）

### 日志规范
- 使用 `zap`，禁止 fmt.Println / log.Println
- 必带字段：trace_id, span_id, layer, task_id（如有）
- 敏感字段自动脱敏（password, token, api_key）

### 测试策略
- **单元测试**：零外部依赖，gomock + testify + miniredis
- **集成测试**：dockertest，每测试独立容器 + 独立 schema
- **E2E测试**：命名空间隔离

### 可观测性
- Tracing：OpenTelemetry OTLP，概率采样
- Metrics：:9090/metrics，Prometheus 格式（`cap_*` 前缀）
- Logging：zap JSON Handler
- Health：/healthz（存活）+ /readyz（就绪，检查 DB + Redis）
- pprof：独立端口 :6060，仅内网访问

---

## Agent 工作流

本项目使用 **LRA** (Long-Running Agent) 管理任务。

### 文档阅读顺序

```
1. 本文件 (agent.md)                ← 架构约束 & 编码规则（必读）
2. TASK-BREAKDOWN.md               ← 认领任务，获取自包含上下文
3. docs/Cloud-Agent-Platform.md   ← 业务细节（实体/模型/API/DDL/编排），任务引用时查阅
4. docs/architecture.md           ← 技术细节（配置/日志/可观测性），任务引用时查阅
```

### 任务流程

```bash
lra ready                              # 查看可认领任务
lra claim <id>                         # 原子性认领
lra show <id>                          # 查看任务详情
    ↓
阅读 TASK-BREAKDOWN.md §TaskID         # 自包含上下文（目标/契约/依赖/约定/验收标准）
    ↓
实现 → 测试 → 提交
    ↓
lra set <id> completed
lra check <id>                         # 运行 Constitution 质量门控
lra set <id> truly_completed           # 完成
```

### LRA 命令参考

详细指南见 [lra.md](lra.md)

```bash
lra list                # 列出所有任务
lra ready               # 列出可认领任务
lra show <id>           # 查看任务详情
lra claim <id>          # 认领任务
lra set <id> <status>   # 更新状态
lra check <id>          # 运行质量检查
lra checkpoint <id> --note "进度"  # 保存检查点
```

### Session 结束 Checklist

结束 session 前必须：
1. `lra checkpoint <id> --note "当前进度"` 保存所有进行中的任务
2. `lra set <id> completed/optimizing` 更新状态
3. Git 提交并推送

### 禁止规则
- ❌ 不要创建 markdown TODO 列表
- ❌ 不要使用 LRA 以外的追踪系统
- ❌ 不要跳过 `lra ready` 直接问"我该做什么"
- ❌ 不要编辑 task 文件（用 `lra set` 命令）

---

## 详细规范
完整技术规范见 [docs/architecture.md](docs/architecture.md)，业务设计见 [docs/Cloud-Agent-Platform.md](docs/Cloud-Agent-Platform.md)

