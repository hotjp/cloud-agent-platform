# Cloud Agent Platform - 外部 Agent 架构设计

> **创建时间**: 2026-05-16
> **状态**: 草案,讨论中
> **来源**: 创始人与 AI 助手的架构讨论

---

## 〇、代码现状(截至 2026-05-16)

### 项目进度

145/145 LRA task 全部完成(100%)。核心模块已实现,E2E 测试通过。

### 当前执行模型

```
用户提交任务(MCP / REST)
        ↓
Gateway REST handler
        ↓
TaskService.Submit() - 事务写入 Task + OutboxEvent
        ↓
Outbox Poller → Redis Stream
        ↓
Orchestrator.StartTask() - 任务拆解 + 分配
        ↓
OrchestratorAdapter.Execute()
        ├── 有 Git 容器?
        │   ├── 是:GitContainer.Ensure() → 创建/复用项目容器
        │   │         → 创建 cap-worker 容器(挂载 Git 容器的 volume)
        │   │         → 在 cap-worker 里执行 entrypoint.sh
        │   └── 否:直接创建独立 cap-worker 容器执行
        ↓
Git 容器 commit/push 结果
```

### 关键文件和职责

| 文件 | 职责 |
|------|------|
| `cmd/server/main.go` | 启动入口,组装所有组件 |
| `internal/gateway/handler.go` | REST + connect-go 端点,JWT 鉴权 |
| `internal/service/task_service.go` | 任务提交、查询、取消 |
| `internal/orchestrator/orchestrator.go` | 任务拆解、Agent 分配、ReAct 循环编排 |
| `internal/scheduler/backend.go` | Backend 接口(Create/Exec/Destroy) |
| `internal/scheduler/docker_backend.go` | Docker 容器后端实现 |
| `internal/scheduler/orchestrator_adapter.go` | 桥接 Scheduler → Orchestrator,管理 Git 容器 + Worker 容器 |
| `internal/gitcontainer/manager.go` | 按项目创建/复用 Git 容器(cap-git image),HTTP API 做 Git 操作 |
| `internal/worker/sandbox/sandbox.go` | Sandbox 接口定义 |
| `internal/worker/sandbox/docker.go` | Docker 沙箱实现 |
| `internal/worker/sandbox/cube.go` | CubeSandbox 轻量沙箱实现 |
| `plugins/workermanager/workermanager.go` | Worker 池管理(获取/释放/健康检查/空闲清理) |
| `plugins/mcpserver/tools.go` | MCP Server,9 Tools + 4 Resources |
| `plugins/orchestrator/eino_nodes.go` | Eino 图节点(复杂度路由、Agent 匹配) |

### 容器结构

```
Git 容器(cap-git:latest)
  - 每个项目一个,常驻
  - 提供 HTTP API:/init, /commit, /push, /diff, /files, /health
  - Volume: /tmp/cap-git-volumes/{projectID}/

Worker 容器(cap-worker:latest)
  - 每个 Agent 执行创建一个,执行完销毁
  - 挂载 Git 容器的 Volume 到 /workspace
  - entrypoint.sh 内置 LLM 调用循环
  - 环境变量注入:LLM_API_URL, LLM_API_KEY, LLM_MODEL, TASK_ID, RESULT_BRANCH 等
```

### Worker 自建 LLM 循环

cap-worker 的 entrypoint.sh 执行流程:

```
1. 解析环境变量获取任务信息
2. 循环调用 LLM API
3. 解析 LLM 返回的工具调用
4. 执行工具(read_file, write_file, exec_command 等)
5. 将工具结果喂回 LLM
6. 重复直到 LLM 输出完成信号或达到步数上限
7. 退出,Worker 容器销毁
```

**这个循环很基础**,缺乏 Claude Code 级别的上下文管理、错误恢复、代码理解能力。这是引入外部 Agent 的核心动机。

### Backend 接口(已有抽象)

```go
type Backend interface {
    Create(ctx context.Context, spec ContainerSpec) (string, error)
    Exec(ctx context.Context, containerID string, spec ExecSpec) (ExecResult, error)
    Destroy(ctx context.Context, containerID string) error
}
```

目前只有 `DockerBackendAdapter` 一个实现。接口已做好抽象,后续新增 InProcess 等后端不需要改上层代码。

---

## 〇-A、平台定位（✅ 已确定）

> **状态**: 讨论中的核心共识,尚未最终定稿。基于当前讨论的倾向性结论。
> **修订**: 2026-05-16,吸收外部评审(「镜」)反馈,修正三个矛盾点。

### 核心问题

我们要做的是大脑还是手脚?CPU 还是 GPU?编排还是实现?

### 倾向性答案

**我们做手脚为主,做 GPU,做编排。但平台提供默认 LLM 能力。**

```
Cloud Agent Platform = GPU + 默认 LLM 服务(?)
  - 接收任务(显存分配)
  - 拆解任务(并行计算)
  - 分配给 Agent(计算单元)
  - 管理资源(内存、文件锁)
  - 收集结果(回读显存)
  - 提供默认 LLM 调用能力(平台承担成本,收集完整数据)

外部 Agent = CUDA Core
  - 接收指令
  - 执行计算(LLM 推理 + 工具调用)
  - 返回结果
  - 可以使用平台提供的 LLM,也可以自带
```

### 理由

1. **手脚(工具执行)是核心壁垒** - 在容器里安全地读写文件、跑命令、管理 Git、协调并发,这些是第三方 Agent 不替你做的。
2. **LLM 网关是数据飞轮的关键** - 平台做 LLM 流量的中间层,Agent 调平台网关而非直连 Provider,平台天然记录所有 LLM 交互数据。不需要 Agent 同意回传。
3. **外部 Agent 带来能力增量** - Claude Code、Kimi 的能力是自建难以追赶的,但不排斥它们接入。
4. **GPU 类比有局限** - GPU 核心是同质的,Agent 是异构的。类比只用于沟通直觉,工程上不低估适配复杂度。

### 关键修正(来自评审反馈)

| 原表述 | 问题 | 修正后 |
|--------|------|--------|
| "平台不碰 LLM" | ask_llm 工具和数据飞轮都需要 LLM 能力 | 平台提供默认 LLM,Agent 可自带 |
| "拉模式,不推" | 执行流程里实际是平台推送任务描述给 Agent | 初始上下文由平台推送,后续 Agent 按需拉取 |
| "大脑不是壁垒" | 阶段 3 又说要追上第三方,逻辑矛盾 | 当前不自建是成本考量,非能力判断;自建是阶段 3 目标 |

### 对下游设计的影响

| 问题 | 方向 |
|------|------|
| Agent 注册 | 从单一 Agent(OpenClaw)开始,不急着通用适配 |
| 上下文传递 | 初始推送任务描述 + 工具列表,后续 Agent 按需拉取 |
| 成本归属 | 使用平台 LLM → 平台承担成本并收集数据;自带 LLM → Agent 承担成本,平台只收工具数据 |
| 安全边界 | 平台严格控制(细粒度锁、命令白名单、权限隔离) |
| Agent 生命周期 | 平台管理,含心跳/租约/超时接管 |
| 工具接口 | 平台暴露最小必要工具集,含 ask_llm |
| 降级策略 | 平台可换 Agent,但承认用户习惯绑定风险 |

---

## 一、背景与动机

### 现状

当前 Cloud Agent Platform 的 Worker 自建了完整的 Agent 循环:

```
cap-worker 容器内:
  entrypoint.sh → LLM 调用 → 解析工具调用 → 执行工具 → 结果喂回 LLM → 循环
```

这个循环质量远不如成熟的第三方 Agent(Claude Code、Kimi)。

### 问题

1. **自建 Agent 能力弱** - ReAct 循环简单,缺乏 Claude Code 级别的上下文管理、错误恢复、代码理解能力
2. **重复造轮子** - OpenClaw 子 Agent、Claude Code、Kimi 都已经有成熟的 Agent 循环,没必要自己写
3. **平台的核心价值不在 Agent 循环** - 而在调度、编排、状态管理、并发控制这些基础设施

### 决策

**方案 B:平台当调度层,Agent 能力外挂。**

平台不自己跑 LLM 循环,而是把"思考+行动"的职责交给更擅长的外部 Agent。

---

## 二、架构设计

### 角色划分

```
┌──────────────────────────────────────────────┐
│            Cloud Agent Platform               │
│            (调度层 + 工具执行层)               │
│                                               │
│  ┌─────────┐  ┌──────────┐  ┌─────────────┐ │
│  │任务拆解  │  │并发控制   │  │状态追踪      │ │
│  └─────────┘  └──────────┘  └─────────────┘ │
│  ┌─────────┐  ┌──────────┐  ┌─────────────┐ │
│  │Git 管理  │  │容器管理   │  │事件推送      │ │
│  └─────────┘  └──────────┘  └─────────────┘ │
│                                               │
│  ┌─────────────────────────────────────────┐ │
│  │  工具执行层(MCP Server 暴露)            │ │
│  │  read_file / write_file / exec_command  │ │
│  └─────────────────────────────────────────┘ │
│                                               │
│  ┌─────────────────────────────────────────┐ │
│  │  LLM 网关(反代 + 日志 + 路由)          │ │
│  │  Agent 的所有 LLM 调用必须经过网关       │ │
│  │  网关记录完整 input/output → 数据飞轮    │ │
│  └─────────────────────────────────────────┘ │
└──────────────────┬───────────────────────────┘
                   │ MCP / REST API
                   ↓
┌──────────────────────────────────────────────┐
│            外部 Agent                          │
│                                               │
│  OpenClaw 子Agent / Claude Code / Kimi / ...  │
│                                               │
│  负责:                                        │
│  - LLM 调用(思考、推理)                       │
│  - 决定用什么工具、传什么参数                     │
│  - 多轮迭代直到任务完成                          │
└──────────────────────────────────────────────┘
```

### 三个角色

| 角色 | 职责 | 不做什么 |
|------|------|---------|
| **调度层(平台)** | 任务拆解、分配、并发控制、状态追踪、Git 管理、事件推送 | 不思考、不调用 LLM |
| **外部 Agent** | 思考、推理、决策、调用工具、多轮迭代 | 不管理容器、不做并发控制 |
| **工具执行层(平台)** | 在容器内执行 read_file、write_file、exec_command 等操作 | 不决策 |

### 执行流程

```
1. 用户提交任务(MCP / REST)
2. 平台拆解任务 → 分配给外部 Agent
3. 外部 Agent 收到:
   - 任务描述
   - 可用工具列表(平台暴露的 MCP 工具)
   - 上下文信息(项目结构、已有文件等)
4. 外部 Agent 开始工作:
   - 思考 → 调用 read_file → 平台从容器读文件 → 返回
   - 继续思考 → 调用 write_file → 平台写入容器
   - 调用 exec_command → 平台在容器里跑命令
   - 看到结果 → 继续迭代 → 直到完成
5. 外部 Agent 报告完成 → 平台做 Git commit/push
6. 平台更新任务状态 → 通知用户
```

---

## 三、工具访问方式：共享 Volume + 进程隔离

> **修订**: 2026-05-16，从 Sidecar → Cube → 回归务实方案。
> **Cube 调研结论**: 每个 Cube 沙箱是独立 MicroVM，不适合多 Agent 共享文件的场景。留作未来 exec_command 安全执行环境。

### 核心思路

**一个项目 = 一个 Git 容器（Volume），Agent 进程通过共享 Volume 访问文件。**

```
┌─ 项目 Git 容器 ─────────────────────────────────┐
│                                                    │
│  /workspace（Volume，源代码）                       │
│  Git HTTP API（/init, /commit, /push, /diff）      │
│                                                    │
│  平台管理进程：                                     │
│  - 文件锁 / 并发控制                                │
│  - Git 操作（commit/push）                          │
│  - LLM 网关代理                                    │
│  - 审计日志                                        │
└──────────┬─────────────────────────────────────────┘
           │ 共享 Volume 挂载
           │
    ┌──────┴──────┐
    │             │
┌───▼───┐   ┌───▼───┐
│Agent 1│   │Agent 2│   ...
│Worker │   │Worker │
│容器    │   │容器    │
└───────┘   └───────┘
```

**和现有架构的区别：**

| | 现有 | 新架构 |
|---|---|---|
| Worker 镜像 | cap-worker（自建 LLM 循环） | OpenClaw Agent 镜像 |
| 文件访问 | 共享 Volume | 共享 Volume（不变） |
| Git 操作 | Worker 自己 commit/push | 平台统一 commit/push |
| LLM 调用 | Worker 内部直接调 LLM | 走平台 LLM 网关 |
| 工具执行 | Worker 内部直接执行 | 同左 |

**本质上就是换 Worker 镜像 + 加 LLM 网关 + 平台接管 Git 操作。** 容器编排方式不变。

### 为什么不用 Cube

Cube Sandbox 调研结论：
- 每个 Cube 沙箱是独立 MicroVM（轻量虚拟机），不是进程级沙箱
- 文件共享走 virtiofs，不如 Docker Volume 直接
- 仅支持 x86_64 Linux + KVM，macOS 无法开发
- 无 Go SDK
- **结论**：Cube 留作未来 exec_command 安全执行环境的选项，不用于 Agent 编排

---

## 四、与现有代码的关系

### 已有基础设施(可复用)

| 组件 | 当前用途 | 新架构用途 |
|------|---------|----------|
| MCP Server(9 Tools + 4 Resources) | 给用户暴露接口 | 给外部 Agent 暴露工具接口 |
| Git Container Manager | 管理项目容器 | 不变,继续管理 |
| Worker 执行框架 | 自建 LLM 循环 | 退化为工具执行层 |
| Backend 接口 | Docker/Cube 沙箱 | 保留,未来可扩展 |
| Outbox + WebSocket | 事件推送 | 不变 |

### 核心改动

**Worker 从"有脑有手"变成"只有手":**

```
当前 Worker:
  [LLM 循环] → [工具调用] → [容器内执行]

新 Worker:
  等待外部 Agent 调用 → [工具执行] → 返回结果
```

Worker 不再自己调用 LLM,只负责执行工具操作并返回结果。LLM 循环的职责转移到外部 Agent。

---

## 五、设计细节

> 以下问题尚未得出结论,需要进一步讨论。
> **注意**: 评审反馈指出,这些问题中的任何一个都可能影响顶层架构决策。在 POC 验证前不应视为已确定。

### 5.1 Agent 注册机制（✅ 已确定）

- **当前阶段**：单一 Agent（OpenClaw）深度集成，通过配置文件注册
- **后期目标**：通用 Agent 注册机制，声明能力（语言、领域等）
- 通用注册是扩展目标，不是起步方案

### 5.2 上下文传递(修订)

- **初始上下文由平台推送**:任务描述、可用工具列表、项目文件树概览
- **后续按需拉取**:Agent 自己调用 read_file 获取具体文件内容
- 多 Agent 上下文共享:平台维护共享上下文缓存,Agent B 可以复用 Agent A 已读取的文件
- **待验证**:初始上下文的 Token 成本是否可接受

### 5.3 成本归属（✅ 已确定）

- **双模式**：
  - 使用平台 LLM 网关 → 平台承担成本，收集完整 LLM 交互数据（第二级数据）
  - 自带 LLM → Agent 承担成本，平台只收集工具调用数据（第一级数据）
- 这保证了数据飞轮的基础运转，同时给 Agent 选择权

### 5.4 安全边界（✅ 已设计）

#### 文件访问控制

```
Agent 访问范围 = 项目根目录 + 临时目录
禁止访问: /etc, /root, /home, 其他项目目录
```

- 每次工具调用带 project_id，平台校验 Agent 是否有权限操作该项目
- read_file / write_file 的路径必须在项目沙箱内（路径遍历检测）
- 临时文件写入平台分配的 tmp 目录，任务结束自动清理

#### 命令执行控制

**黑名单（始终禁止）：**
```
rm -rf /
sudo *
chmod 777 *
nc -l * / nc * -e *
curl * | sh / wget * | sh
mkfs *
dd if=* of=/dev/*
```

**白名单（默认允许）：**
```
go *, python *, node *, npm *, pip *
git *
make *, cmake *
cat *, head *, tail *, grep *, find *, ls *
echo *, mkdir *, cp *, mv *
docker *（仅 inspect/logs，不含 run/exec）
```

**灰名单（需要 Guardian 审批）：**
```
docker run/exec/build
deploy *, kubectl *
apt/yum/brew install *
curl/wget（下载外部文件）
```

#### 网络访问控制

```yaml
network_policy:
  default: restricted
  restricted:
    allow:
      - LLM API（通过网关）
      - package registry（npm, pip, go proxy）
      - git remote
    deny:
      - 其他所有外部访问
  full:
    allow: all
    require: guardian_approval
```

#### 审计日志

每次工具调用记录：
```json
{
  "timestamp": "2026-05-16T09:00:00Z",
  "agent_id": "openclaw-sub-001",
  "task_id": "task_xxx",
  "tool": "write_file",
  "params": {"path": "src/main.go", "size": 2048},
  "result": "success",
  "duration_ms": 12
}
```

#### 混合模式（方式 2）安全方案

Agent "直接进容器"不等于不受控。实现方式：
- Agent 通过平台分配的 SSH/exec 会话进入容器
- 所有命令经过平台的命令过滤器（黑/白名单）
- 平台实时审计命令流
- 会话有超时，超时后自动断开

---

### 5.5 Agent 生命周期与故障恢复（✅ 已设计）

#### 状态机

```
                    ┌──────────────────────────────┐
                    │                              │
                    ▼                              │
idle → assigned → running → completed → idle(回池)
                │       │
                │       ▼
                │    timed_out → retry → assigned
                │       │
                │       ▼
                │    failed
                ▼
             crashed → recover → assigned
```

#### 心跳协议

```
Agent → 平台: heartbeat(agent_id, task_id, progress, timestamp)
频率: 每 30 秒
超时: 连续 3 次未收到（90 秒）→ 判定失联
```

#### 租约机制

```
Agent 获取任务时获得租约:
  lease_id: 唯一标识
  ttl: 5 分钟（每次心跳续约）
  expires_at: 过期时间

租约过期 → 平台自动释放该 Agent 持有的所有文件锁 → 任务状态回滚到 pending → 可被其他 Agent 认领
```

#### 超时策略

| 级别 | 超时时间 | 触发动作 |
|------|---------|----------|
| 任务级 | 20 分钟 | 任务标记 failed，释放所有资源 |
| 步骤级 | 5 分钟 | 单个工具调用超时，Agent 收到超时错误可重试 |
| 心跳级 | 90 秒 | 判定 Agent 失联，触发租约回收 |

#### 崩溃恢复

```
1. 平台检测到 Agent 失联（心跳超时）
2. 等待租约过期（最多 5 分钟，给 Agent 最后的恢复机会）
3. 释放该 Agent 持有的所有文件锁
4. 回滚该 Agent 的未提交文件变更（git checkout -- .）
5. 任务状态改为 pending
6. 重新分配给其他 Agent
```

#### Agent 间通信

- 不直接通信，通过平台中转
- 平台维护共享上下文缓存（已读取的文件、已执行的结果）
- Agent B 可以看到 Agent A 的产出，但通过平台提供，不直接访问

---

### 5.6 工具接口设计（✅ 已设计）

#### 工具清单（10 个，POC 阶段）

| 工具 | 参数 | 说明 | 安全级别 |
|------|------|------|----------|
| `read_file` | path, [start_line, end_line] | 读取文件内容 | 白名单 |
| `write_file` | path, content | 写入文件 | 白名单 + 文件锁 |
| `edit_file` | path, old_text, new_text | 精确替换 | 白名单 + 文件锁 |
| `list_files` | path, [pattern] | 列出目录 | 白名单 |
| `search_code` | query, [path] | 搜索代码 | 白名单 |
| `exec_command` | cmd, [timeout, workdir] | 执行命令 | 黑/白名单过滤 |
| `git_status` | - | 查看 Git 状态 | 白名单 |
| `git_diff` | [path] | 查看差异 | 白名单 |
| `ask_llm` | prompt, [model, temperature] | 调用 LLM | 走网关，记录数据 |
| `report_progress` | message, [percent] | 报告进度 | 白名单 |

#### ask_llm 接口设计

```go
type AskLLMRequest struct {
    Prompt      string            `json:"prompt"`
    Model       string            `json:"model,omitempty"`       // 可选，默认用任务配置的模型
    Temperature float64           `json:"temperature,omitempty"` // 默认 0.7
    MaxTokens   int               `json:"max_tokens,omitempty"`  // 默认 4096
    Context     []string          `json:"context,omitempty"`     // 附加上下文文件路径
}

type AskLLMResponse struct {
    Content     string            `json:"content"`
    Model       string            `json:"model"`
    TokensUsed  int               `json:"tokens_used"`
    Cost        float64           `json:"cost_yuan"`
}
```

- 请求走 LLM 网关，自动记录 input/output
- 单次调用 token 上限 4096，防止单次消耗过大
- 单个任务 ask_llm 调用次数上限 10 次

#### 不含的工具（POC 阶段不做）

- ~~spawn_sub_agent~~ — 复杂度高，POC 不需要
- ~~git_commit~~ — 由平台在任务完成后统一执行
- ~~git_push~~ — 同上

#### Git 操作由平台控制

Agent 不直接做 git commit/push，而是：
1. Agent 通过 write_file/edit_file 修改文件
2. Agent 完成任务后报告结果
3. 平台统一执行 git add → git commit → git push
4. 这样平台控制了 Git 历史，避免 Agent 写出不合规的 commit message

### 5.7 降级策略（✅ 已确定）

- 外部 Agent 不可用时：
  - 优先级 1：排队等待同类型 Agent 空闲
  - 优先级 2：降级到其他可用 Agent
  - 优先级 3：降级到平台自建的简单 LLM 循环
  - 最终：任务标记为 failed

---

## 六、并发控制设计

> **状态**: 初步设计，以下验证项需在 POC 中确认

### 核心问题

多个外部 Agent 同时操作同一个项目的文件,如何避免冲突?

### 并发原语(需定义)

| 原语 | 粒度 | 语义 | 适用场景 |
|------|------|------|----------|
| 项目级写锁 | 整个项目 | 同一时刻只有一个 Agent 能写 | 重量级任务(全项目重构) |
| 文件级写锁 | 单个文件 | 同一文件同时只有一个 Agent 写 | 日常开发 |
| 乐观读 | 单个文件 | 读取时不加锁,写入时检测版本冲突 | 大部分读操作 |
| 原子批量操作 | 文件组 | 一组操作要么全成功要么全回滚 | 配置文件 + 代码同步修改 |

### 锁的生命周期

```
Agent 获取锁 → 执行操作 → 释放锁
                  ↓ 超时/崩溃
            平台自动释放锁(租约机制)
```

- 锁持有超时:默认 5 分钟
- 心跳续约:Agent 每 30 秒发送心跳续约锁
- 锁超时后自动释放,其他 Agent 可获取

### 待验证

- 5-10 个 Agent 并发下的锁竞争性能
- 文件级锁的粒度是否足够细
- Git 操作(commit/push)与文件锁的协调

---

## 七、演进路径(修订)

> **修订**: 2026-05-16,吸收评审反馈,调整为 POC 驱动。

```
阶段 0(当前):平台自建 Worker 循环
  → 能跑,但 Agent 能力弱
  → 继续保持,作为降级方案

阶段 1(下一步):单一 Agent POC
  → 选择 OpenClaw 子 Agent 做深度集成
  → 验证三个硬指标:
    1. MCP 文件调用的总延迟是否可接受(<2s/次?)
    2. 多 Agent 并发写同一项目的冲突解决
    3. 上下文传递的 Token 成本
  → POC 失败则回退,不强行推进

阶段 2(POC 通过后):OpenClaw 生产化
  → OpenClaw 子 Agent 作为默认 Agent
  → 平台提供默认 LLM 服务
  → 数据飞轮开始运转(第一级 + 第二级数据)

阶段 3(远期):多 Agent 接入 + 自建 Agent
  → 引入其他 Agent(Claude Code、Kimi 等)
  → 用积累的数据训练/优化自建 Agent
  → 混合使用:简单任务用自建,复杂任务用第三方
```

### POC 验证清单

- [ ] OpenClaw 子 Agent 通过 MCP 调用平台工具的延迟测试
- [ ] 2-3 个 Agent 并发读写同一项目的冲突测试
- [ ] 上下文传递 Token 成本估算
- [ ] Agent 崩溃后任务状态恢复测试
- [ ] ask_llm 工具的延迟和成本测试

> **详细测试方案**: [POC-TEST-PLAN.md](POC-TEST-PLAN.md) — 6 组测试，含极限场景，找到系统崩溃点

---

## 八、与 InProcess 优化的关系

> 详见 [PRODUCT-VISION.md 第九章](PRODUCT-VISION.md#九未来架构优化inprocess-执行模式)

InProcess 优化(Agent 在 Git 容器内直接执行)和外部 Agent 架构不冲突:

- InProcess 解决的是**容器开销**问题
- 外部 Agent 解决的是**Agent 能力**问题

两者可以叠加:外部 Agent 通过 MCP 调用工具,平台在 Git 容器内直接执行(InProcess 模式),不走独立 Worker 容器。

---

## 九、数据飞轮与 LLM 交互数据收集

> **状态**: 核心策略已确定，数据 Schema 已设计

### 数据飞轮的初衷

```
用户使用平台 → 产生完整任务链路 → 高质量标注数据 → 训练更好的模型
```

训练模型最值钱的数据是完整的 LLM 交互链路:

```
prompt → Agent 思考 → 工具调用 → 工具返回 → Agent 继续思考 → ... → 最终结果
```

### 当前数据库已有的

| 表 | 内容 | 数据飞轮价值 |
|------|------|------------|
| `tasks` | 任务元数据(目标、状态、分支、标签) | 任务级元数据 |
| `subtasks` | 子任务(描述、Agent 模板、token 消耗、依赖) | 拆解模式 |
| `audit_logs` | 操作日志(action + message + details) | 操作记录,但不完整 |
| `agent_experience` | Agent 执行经验(任务类型、状态、耗时、token、成本) | 统计指标,非原始数据 |
| `decomposition_pattern` | 任务拆解模式(成功率、并行度、子任务结构) | 拆解策略 |
| `outbox_events` | 领域事件(状态变更) | 事件流 |

### 缺什么?

**LLM 交互的完整对话链路。**

- `audit_logs` 只记了 action + message,没有完整的 LLM input/output
- 没有工具调用的参数和返回值
- 没有多轮对话的上下文链路
- `agent_experience` 是统计指标,不是原始对话数据

### "做手脚"定位对数据收集的影响

**有利的一面:**

1. 平台控制所有工具调用 → read_file、write_file、exec_command 全经过平台,参数和返回值天然可记录(训练数据的 "Action" 部分)
2. 多种 Agent 接入 → 数据来源更丰富
3. 用最好的 Agent → 产出的思考链路质量更高

**核心难题已解决:LLM 网关**

> **修订**: 2026-05-16,发现 LLM 网关是数据收集的最优解。

之前认为第二级数据(LLM 交互)"只有自建 Agent 能提供"。实际上只需要一个 **LLM 网关**:

```
Agent → 平台 LLM 网关 → 实际 LLM Provider(OpenAI / Anthropic / 智谱 / MiniMax)
              ↓
         记录所有 input/output(第二级数据)
              ↓
         透传给 Agent
```

**Agent 不需要"同意回传"**,因为它调的就是平台的网关。平台天然能看到所有 LLM 请求和响应。Agent 甚至不需要知道中间有个网关--把 API URL 指向平台就行。

LLM 网关的附加价值:
- **限流熔断** - 防止单个 Agent 吃光 LLM 配额
- **多 Provider 路由** - 主备切换、成本优化、按任务选模型
- **缓存** - 相同 prompt 不重复调 LLM,省成本
- **审计** - 每次调用可追溯
- **成本结算** - 平台统一结算,按用量计费给用户/Agent

**结论:LLM 网关让第二级数据(LLM 交互数据)不再是难题。所有 Agent 都能贡献完整数据,不区分自建还是第三方。**

### 数据收集策略(修订后)

```
第一级:工具调用数据(平台天然拥有)
  - 所有 read_file、write_file、exec_command 的参数和返回值
  - 训练价值:工具使用模式、代码编辑模式

第二级:LLM 交互数据(LLM 网关自动收集)
  - 所有 LLM 的输入 prompt、输出 response
  - Agent 不需要额外配合,调平台网关就行
  - 训练价值:思维链、推理过程

第三级:结果评价数据(用户侧)
  - 用户对任务结果的满意度、修改、接受/拒绝
  - 通过 MCP/REST 接口收集
  - 训练价值:偏好对齐、质量评估
```

### 结论

1. **三级数据全部可收集**,LLM 网关解决了第二级的难题
2. **不需要自建 Agent 也能拿到完整数据**,降低了数据飞轮的门槛
3. **LLM 网关本身就是产品价值** - 限流、路由、缓存、审计、成本管理
4. **数据飞轮可以立即启动**,不需要等到阶段 3

### 数据存储 Schema

#### 第一级：工具调用记录（tool_calls 表）

```sql
CREATE TABLE tool_calls (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL,
    subtask_id VARCHAR(255),
    agent_id VARCHAR(255) NOT NULL,
    tool VARCHAR(50) NOT NULL,           -- read_file, write_file, exec_command ...
    params JSONB NOT NULL,               -- 工具调用参数
    result JSONB,                        -- 工具返回结果（截断到大 64KB）
    result_summary TEXT,                 -- 结果摘要（用于快速检索）
    exit_code INTEGER,                   -- exec_command 退出码
    duration_ms INTEGER,                 -- 执行耗时
    status VARCHAR(20) NOT NULL,         -- success, failed, timeout, blocked
    error_message TEXT,                  -- 错误信息
    created_at BIGINT NOT NULL,
    INDEX(task_id),
    INDEX(agent_id),
    INDEX(tool),
    INDEX(created_at)
);
```

#### 第二级：LLM 交互记录（llm_calls 表）

```sql
CREATE TABLE llm_calls (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL,
    subtask_id VARCHAR(255),
    agent_id VARCHAR(255) NOT NULL,
    gateway_trace_id VARCHAR(255),       -- LLM 网关追踪 ID
    provider VARCHAR(50) NOT NULL,       -- openai, anthropic, zhipu, minimax
    model VARCHAR(100) NOT NULL,         -- gpt-4o, claude-3.5-sonnet, glm-5
    role VARCHAR(20) NOT NULL,           -- system, user, assistant, tool
    -- 输入侧
    prompt_tokens INTEGER NOT NULL,
    prompt_hash VARCHAR(64),             -- prompt 去重用（不存原始 prompt 到这列）
    prompt_text TEXT,                    -- 完整 prompt（可选，大文本存对象存储）
    -- 输出侧
    completion_tokens INTEGER NOT NULL,
    completion_text TEXT,                -- 完整 completion
    tool_calls_json JSONB,               -- 模型输出的工具调用
    finish_reason VARCHAR(30),           -- stop, tool_calls, length
    -- 元数据
    latency_ms INTEGER NOT NULL,
    cost_yuan FLOAT NOT NULL,
    temperature FLOAT,
    created_at BIGINT NOT NULL,
    INDEX(task_id),
    INDEX(agent_id),
    INDEX(provider, model),
    INDEX(created_at)
);
```

**大文本处理策略：**
- prompt/completion < 64KB → 直接存 PostgreSQL
- prompt/completion >= 64KB → 存 MinIO，表中只存引用路径
- 所有数据保留 90 天，之后归档到冷存储

#### 第三级：用户评价记录（task_evaluations 表）

```sql
CREATE TABLE task_evaluations (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL UNIQUE,
    user_id VARCHAR(255) NOT NULL,
    rating INTEGER,                      -- 1-5 分
    accepted BOOLEAN,                    -- 是否接受结果
    feedback TEXT,                        -- 用户反馈
    modifications JSONB,                 -- 用户做了哪些修改（diff）
    created_at BIGINT NOT NULL,
    INDEX(task_id),
    INDEX(user_id)
);
```

#### 训练数据导出视图

```sql
-- 完整任务链路视图（用于导出训练数据）
CREATE VIEW training_task_chain AS
SELECT
    t.id AS task_id,
    t.goal,
    t.status AS task_status,
    tc.tool,
    tc.params,
    tc.result_summary,
    lc.prompt_text,
    lc.completion_text,
    lc.provider,
    lc.model,
    lc.tool_calls_json,
    te.rating,
    te.accepted
FROM tasks t
LEFT JOIN tool_calls tc ON tc.task_id = t.id
LEFT JOIN llm_calls lc ON lc.task_id = t.id
LEFT JOIN task_evaluations te ON te.task_id = t.id
ORDER BY t.created_at, tc.created_at, lc.created_at;
```

---

## 十、外部评审记录

> **评审时间**: 2026-05-16
> **评审者**: Kimi(角色扮演「镜」- 资深架构师)
> **评审对象**: 本文档初版

### 评审结论

顶层方向合理,但论证建立在理想化假设上。核心问题:
1. "平台不碰 LLM"与 ask_llm 工具和数据飞轮需求矛盾
2. GPU 类比掩盖了 Agent 异构性带来的适配复杂度
3. 网络延迟被低估,高频文件访问场景下可能不可接受
4. 并发控制设计空白(只有"文件锁"四个字)
5. 大量硬问题被后置为"待讨论细节",但这些问题可能推翻顶层设计

### 三个核心建议

1. **先做单一 Agent POC** - 冻结通用适配野心,用 OpenClaw 验证三个硬指标
2. **重新定义平台与 LLM 的关系** - 平台提供默认 LLM,Agent 可自带
3. **并发控制原语和安全模型先设计再写代码**

### 本轮吸收情况

| 建议 | 是否吸收 | 说明 |
|------|---------|------|
| 先做单一 Agent POC | ✅ 吸收 | 演进路径调整为 POC 驱动 |
| 平台提供默认 LLM | ✅ 吸收 | 修正平台定位 |
| 并发控制具体设计 | ✅ 吸收 | 新增第六章 |
| 数据飞轮前置 | ✅ 吸收 | 成本归属已确定方向 |
| Agent 故障恢复 | ✅ 吸收 | 5.5 增加心跳/租约/接管 |
| GPU 类比不完美 | ⚠️ 部分吸收 | 加局限性说明,但不影响决策 |
| 强化自建 Agent | ❌ 暂不吸收 | 阶段 3 目标,不矛盾 |
| 容器内 Agent 镜像 | ❌ 暂不吸收 | 记为灵感,当前阶段不做 |

### 后续新增(讨论中产生)

| 洞察 | 来源 | 影响 |
|--------|------|------|
| LLM 网关解决数据收集难题 | 用户 | 第九章全面修订,数据飞轮三级数据全部可收集 |
| LLM 网关本身是产品价值 | 用户 | 新增架构图中的 LLM 网关组件 |

---

*文档持续更新中,后续讨论结果会补充进来。*
