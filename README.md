# Cloud Agent Platform (CAP)

> 提交一个开发任务 → 平台自动调度容器化 Agent → LLM 生成代码 → 结果推送到 Git 分支

## 这是什么

CAP 是一个用 Go 写的云端多 Agent 编排平台。它接收自然语言描述的开发任务，在 Docker 容器中启动 Worker，调用 LLM 生成代码，并将结果提交到 Git 仓库。

**核心流程：**
```
用户提交任务 → 平台调度 → Worker 容器启动 → LLM 生成代码 → Git Container 提交 → Artifact 上报
```

**验证状态：** E2E 全链路已跑通（submit → clone → LLM → diff → commit → artifact），压力测试 10/10 通过。

---

## 快速开始

### 前置依赖

- Go 1.23+
- Docker Desktop（或 Docker Engine）
- PostgreSQL 15+、Redis 7+
- 一个 LLM API Key（目前支持 MiniMax）

### 1. 配置

编辑 `config.yaml`：

```yaml
llm:
  minimax_apikey: "your-minimax-api-key"
  minimax_endpoint: "https://api.minimaxi.com/v1"
  minimax_model: "MiniMax-M2.7-highspeed"
  worker_model: "minimax"  # 或 "zhipu" 作为 fallback

database:
  dsn: "postgres://user:password@localhost:5432/cloud_agent_platform?sslmode=disable"

redis:
  addr: "localhost:6379"
```

### 2. 启动数据库

```bash
# 用 Docker 快速启动 PostgreSQL + Redis
docker run -d --name cap-postgres \
  -e POSTGRES_USER=user -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=cloud_agent_platform \
  -p 5432:5432 postgres:15

docker run -d --name cap-redis -p 6379:6379 redis:7
```

### 3. 构建 & 启动

```bash
# 构建 Worker 镜像（Agent 运行环境）
docker build -t cap-worker:latest -f images/cap-worker/Dockerfile images/cap-worker/

# 构建 Git Container 镜像（Git 操作服务）
docker build -t cap-git:latest -f images/cap-git/Dockerfile images/cap-git/

# 编译 & 启动平台服务
go build -o cap-server ./cmd/server/
./cap-server
```

服务默认监听 `http://localhost:18080`。

### 4. 提交任务

```bash
# 生成 JWT Token（开发环境）
TOKEN=$(go run ./cmd/tools/ jwtgen --subject user-001 --client test-client)

# 提交一个代码生成任务
curl -X POST http://localhost:18080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "goal": "Create a Python FastAPI REST API for a todo app with CRUD endpoints",
    "repository": {
      "url": "https://github.com/your-org/your-repo.git",
      "branch": "main"
    }
  }'
```

---

## 架构

### 五层 + 插件

```
L5-Gateway  →  L3-Authz  →  L4-Service  →  L2-Domain  →  L1-Storage
                                                              ↓
                                                         插件层（接口倒置）
```

| 层 | 职责 | 关键代码 |
|---|---|---|
| L5-Gateway | HTTP/REST 路由、JWT 中间件 | `internal/gateway/` |
| L3-Authz | 鉴权、权限（⚠️ 当前是 stub） | `internal/authz/` |
| L4-Service | 业务编排、状态机驱动 | `internal/service/` |
| L2-Domain | Task/Subtask 聚合根、状态定义 | `internal/domain/` |
| L1-Storage | ent ORM + PostgreSQL + Redis | `internal/storage/` |
| Scheduler | Worker 调度（独立模块） | `internal/scheduler/` |
| Git Container | 按 project 隔离的 Git 操作服务 | `internal/gitcontainer/` |

### 插件层

| 插件 | 作用 |
|---|---|
| `plugins/orchestrator/` | 任务编排、状态流转 |
| `plugins/llmrouter/` | 多模型路由（Claude / GLM / MiniMax），自适应切换 |
| `plugins/workermanager/` | Worker 容器生命周期 |
| `plugins/mcpserver/` | MCP Server（9 Tools + 7 Resources），stdio JSON-RPC |

### 目录结构

```
cmd/server/main.go            # 入口，DI 组装
internal/
  gateway/                    # L5: REST handler + JWT 中间件
  authz/                      # L3: 鉴权（stub）
  service/                    # L4: 业务逻辑
  domain/                     # L2: 聚合根 + 状态机
  storage/                    # L1: ent + PostgreSQL
  scheduler/                  # Worker 调度（coldstart 模式）
  gitcontainer/               # Git Container 管理（per-project）
  mcp/                        # MCP client（调用平台 REST API）
  config/                     # koanf 配置加载
plugins/
  orchestrator/               # 任务编排引擎
  llmrouter/                  # LLM 路由 + 熔断
  workermanager/              # Docker backend
  mcpserver/                  # MCP Server 定义
images/
  cap-worker/                 # Worker 镜像（Alpine + entrypoint.sh）
  cap-git/                    # Git Container 镜像（Go HTTP API）
scripts/
  stress-test.sh              # 压力测试脚本
  smoke-test.sh               # 冒烟测试脚本
```

---

## 核心概念

### Task 状态机（9 个状态）

```
pending → decomposing → dispatched → running → reviewing → confirming → completed
                                                                       → failed
                                                                       → cancelled
```

### Git Container 架构

每个项目（repo）对应一个独立的 Git Container：

```
用户提交 Task → 调度器分配 Worker Container
                    ↓
        查找/创建 Project 对应的 Git Container
                    ↓
    Worker 和 Git Container 通过 Docker Volume 共享文件
                    ↓
    Worker 写文件 → Git Container commit → Artifact 上报平台
```

**优点：**
- 同一项目的多个 Task 共享 Git Container，共享代码上下文
- 不同项目完全隔离
- Worker 无需安装 git，只负责写文件

### MCP 集成

CAP 内置 MCP Server，AI 助手（Claude Code、OpenClaw 等）可直接调用：

| MCP Tool | 对应 REST API | 说明 |
|---|---|---|
| `task_submit` | POST /api/v1/tasks | 提交任务 |
| `task_status` | GET /api/v1/tasks/{id} | 查询状态 |
| `task_list` | GET /api/v1/tasks | 列出任务 |
| `task_cancel` | POST /api/v1/tasks/{id}/cancel | 取消任务 |
| `task_decide` | POST /tasks/{id}/subtasks/{sid}/decision | 人工审批 |
| `task_diff` | GET /api/v1/tasks/{id}/diff | 获取 diff |
| `task_wait` | - | 阻塞等待完成 |
| `agent_templates` | GET /api/v1/agent-templates | Agent 模板 |
| `platform_status` | GET /api/v1/platform/status | 平台状态 |

---

## 技术栈

| 类别 | 选型 | 说明 |
|---|---|---|
| 语言 | Go 1.23 | 单二进制部署 |
| API 协议 | connect-go | gRPC + HTTP 双模 |
| ORM | ent | Schema 驱动，自动生成 |
| 数据库 | PostgreSQL 15 | 主存储 |
| 缓存 | Redis 7 | 热数据 + 分布式锁 |
| 配置 | koanf | YAML + 环境变量 |
| 日志 | zap | 结构化日志 |
| LLM | MiniMax M2.7 / 智谱 GLM | 通过 llmrouter 多模型路由 |
| ID | oklog/ulid | 全局唯一，时间有序 |
| 容器 | Docker | Worker + Git Container |
| 测试 | testify + miniredis | 单元 + 集成 + E2E |

---

## E2E 测试验证

| 测试场景 | 结果 | 说明 |
|---|---|---|
| 单任务代码生成 | ✅ | Python FastAPI 3 文件，13KB+ 真实代码 |
| 文档生成 | ✅ | API.md 9.7KB，专业 REST API 文档 |
| 并行任务（同 repo） | ✅ | 2 Agent 并行，共享 Git Container，2 次独立 commit |
| 跨项目隔离 | ✅ | 不同 repo → 独立 Git Container |
| 压力测试（10 任务） | ✅ | 5 同 repo + 4 不同 repo + 1 空 project，全部 COMPLETED |
| Agent 间文件共享 | ✅ | Agent B 可读取 Agent A 写入的文件 |

详细测试场景见 [docs/E2E-TEST-SCENARIOS.md](docs/E2E-TEST-SCENARIOS.md)。

---

## 当前限制 & 已知问题

| 问题 | 优先级 | 状态 |
|---|---|---|
| L3-Authz 是 stub（无真实验签） | P0 | 待实现 |
| MCP 硬编码 token，无法区分用户 | P1 | 待实现 |
| Worker 容器执行后未自动清理 | P1 | 待实现 |
| 任务分解（decompose）未实现 | P2 | 待实现 |
| Git push 需要 GIT_TOKEN 配置 | P2 | 已预留接口 |
| Dashboard + WebSocket | P3 | 待实现 |

---

## 开发

```bash
# 编译
go build ./...

# 运行测试
go test ./...

# 运行某个包的测试
go test ./internal/scheduler/ -v

# 压力测试
./scripts/stress-test.sh

# 冒烟测试
./scripts/smoke-test.sh
```

### 代码规范

- 五层架构严格分层，不允许跨层直接调用
- `ent/` 目录是自动生成的，不要手动修改
- `internal/scheduler/` 是独立模块，不依赖 orchestrator/service/gateway
- 所有外部依赖通过接口倒置（插件层）

---

## 文档索引

| 文档 | 内容 |
|---|---|
| [docs/Cloud-Agent-Platform.md](docs/Cloud-Agent-Platform.md) | 完整业务设计（1754 行） |
| [docs/architecture.md](docs/architecture.md) | 框架技术规范 |
| [docs/E2E-TEST-SCENARIOS.md](docs/E2E-TEST-SCENARIOS.md) | E2E 测试场景 |
| [docs/PRODUCT-VISION.md](docs/PRODUCT-VISION.md) | 产品愿景 |
| [CLAUDE.md](CLAUDE.md) | Claude Code 开发约束 |
| [docs/TASK-BREAKDOWN.md](docs/TASK-BREAKDOWN.md) | 任务拆分 |

---

## License

MIT
