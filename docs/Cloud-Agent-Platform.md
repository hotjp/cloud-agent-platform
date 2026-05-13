# Cloud Agent Platform — 最终技术方案

> **版本**: v3.0  
> **日期**: 2026-05-13  
> **定位**: 面向Agent的完整开发指南，覆盖业务设计 + 技术选型 + 实现参考  
> **前置阅读**: 本文档自包含，无需参考外部材料  

---

## 目录

- [一、业务定义](#一业务定义)
- [二、技术选型总览](#二技术选型总览)
- [三、业务模块设计](#三业务模块设计)
- [四、Schema定义](#四schema定义)
- [五、核心接口契约](#五核心接口契约)
- [六、Agent编排引擎](#六agent编排引擎)
- [七、工具系统设计](#七工具系统设计)
- [八、上下文与压缩](#八上下文与压缩)
- [九、Worker沙箱](#九worker沙箱)
- [十、监控与可观测性](#十监控与可观测性)
- [十一、安全设计](#十一安全设计)
- [十二、开发计划](#十二开发计划)
- [十三、关键代码参考](#十三关键代码参考)
- [附录：术语表](#附录术语表)

---

## 一、业务定义

### 1.1 一句话描述

Cloud Agent Platform 是一个**云端多Agent协作执行平台**。用户提交一个开发任务，平台自动拆解成多个子任务，分配给不同角色的Agent并行/串行执行，最终产出代码改动并推送到Git分支。

### 1.2 类比理解

像一个"项目经理（平台）+ 多个专业工程师（Agent）"的协作团队：
- **项目经理**负责任务拆解和进度跟踪
- **工程师们**各自负责分析、编码、审查、测试
- 最终结果汇总交付给用户

### 1.3 三种使用方式

| 方式 | 场景 | 说明 |
|------|------|------|
| **MCP协议**（推荐） | Claude Code / Kimi CLI / OpenClaw | Agent通过MCP Tools与平台交互，原生支持 |
| **REST API** | 脚本/CI/CD集成 | HTTP/JSON调用，用于自动化流水线 |
| **WebSocket** | 实时监控 | 推送任务状态变更和Agent日志，用于前端Dashboard |

---

## 二、技术选型总览

### 2.1 选型原则

1. **Schema驱动开发** — 业务定义（ent Schema + Protobuf）优先，代码由工具生成
2. **AI友好** — 选择让AI生成代码准确率更高的技术模式
3. **确定性生成** — 65%的代码由工具确定性生成，AI只写业务和Schema
4. **内部使用** — 不追求通用化，贴着业务设计

### 2.2 技术栈全景

```
┌──────────────────────────────────────────────────────────────────────┐
│ 前端 (TypeScript/React)                                               │
│ ├── FlowGram              编排可视化 + 执行监控                       │
│ ├── Dashboard             任务管理 + Agent日志 + 成本分析             │
│ └── buf generate TS       API Client（从Protobuf自动生成）           │
├──────────────────────────────────────────────────────────────────────┤
│ 后端 (Go)                                                             │
│ ├── vibe-go               微服务骨架（五层架构 + 插件倒置）            │
│ ├── Eino                  Agent编排引擎（Graph + ADK + MCP）         │
│ ├── connect-go            RPC路由（HTTP/JSON + gRPC双协议）          │
│ ├── ent                   ORM（Schema驱动 + 代码生成）               │
│ ├── buf                   Protobuf管理 + 跨语言代码生成               │
│ ├── sonic                 JSON序列化（高性能）                       │
│ ├── go-redis              Redis客户端                                │
│ ├── pgx                   PostgreSQL驱动                             │
│ ├── go-git                Git操作（纯Go）                            │
│ ├── zap                   结构化日志                                 │
│ ├── koanf                 配置管理                                   │
│ ├── sentinel-go           限流熔断                                   │
│ ├── OpenTelemetry         链路追踪                                   │
│ └── golang-migrate        数据库迁移                                 │
├──────────────────────────────────────────────────────────────────────┤
│ 基础设施                                                              │
│ ├── PostgreSQL 15         任务持久化 + 队列（Outbox模式）            │
│ ├── Redis Sentinel        热上下文 + 分布式锁 + 事件流               │
│ ├── MinIO                 冷存储 + 产出物归档                        │
│ └── CubeSandbox           Worker安全沙箱（腾讯云开源MicroVM）       │
├──────────────────────────────────────────────────────────────────────┤
│ 外部服务                                                              │
│ ├── Anthropic API         Claude Sonnet / Haiku                      │
│ ├── Zhipu API             GLM-5.1 / GLM-5.1-Air                     │
│ └── GitHub/GitLab API     代码仓库操作                               │
└──────────────────────────────────────────────────────────────────────┘
```

### 2.3 核心选型决策及理由

| 决策 | 选型 | 理由 |
|------|------|------|
| **后端语言** | Go | goroutine高并发、单二进制部署、Eino生态 |
| **前端语言** | TypeScript/React | FlowGram是TS/React、生态成熟 |
| **API协议** | connect-go | 一套代码同时支持HTTP/JSON+gRPC，前端直接fetch |
| **ORM** | ent | Schema驱动，AI写Go代码比SQL准确率高；ent generate自动生成CRUD |
| **接口定义** | Protobuf + buf | 一份契约前后端共享，buf generate产出Go+TS代码 |
| **Agent编排** | Eino | 字节跳动开源，Graph+ADK+MCP内置，生产验证 |
| **Worker沙箱** | CubeSandbox | 60ms启动/<5MB内存/硬件级隔离/专为Agent设计 |
| **消息队列** | PostgreSQL Outbox | vibe-go内置Outbox模式，事务内写入+后台转发 |
| **序列化** | sonic | 比标准库快10-20x，drop-in替换 |
| **限流熔断** | sentinel-go | 功能全（QPS/并发/系统自适应），阿里云开源 |
| **JSON处理** | sonic | 10-20x性能提升，零成本替换 |
| **Git操作** | go-git | 纯Go实现，Worker不需要git二进制 |
| **配置管理** | koanf | 轻量、多源叠加、支持热更新 |
| **日志** | zap | 高性能结构化日志，与OTel集成成熟 |
| **测试** | testify+gomock+dockertest | dockertest自动管理Redis/PG测试容器 |
| **构建工具** | Task | 替代Makefile，YAML配置，跨平台 |
| **ID生成** | ULID | 排序友好、无冲突、比UUID更短 |

### 2.4 放弃的方案

| 方案 | 放弃理由 |
|------|----------|
| Node.js + TypeScript（后端） | 并发模型不如goroutine，Eino是Go生态 |
| Fastify | 被vibe-go（基于Hertz）替代 |
| Kitex（对外API） | connect-go双协议更适合前端直接调用；Kitex保留给内部高性能场景 |
| sqlc | AI写Go代码(ent Schema)比写SQL(sqlc)准确率高 |
| Redis Streams（消息队列） | Outbox模式更可靠，同库事务保证 |
| Docker（Worker沙箱） | CubeSandbox硬件级隔离更安全，60ms启动 |
| GORM | ent Schema驱动更适合AI协作模式 |
| viper | koanf更轻量，启动更快 |

---

## 三、业务模块设计

平台由 **8个核心业务模块** 组成。

### 3.1 模块总览

| # | 模块 | 职责 | 复杂度 |
|---|------|------|--------|
| 1 | **任务管理** | 接收、查询、取消任务；管理生命周期 | 中 |
| 2 | **编排调度** | 拆解任务、匹配Agent角色、调度执行顺序 | 高 |
| 3 | **Agent执行** | 在沙箱内运行Agent，调用LLM完成代码操作 | 高 |
| 4 | **上下文管理** | Agent间信息共享、传递、压缩、持久化 | 高 |
| 5 | **工具系统** | Agent可调用的工具集（文件/Git/命令等） | 中 |
| 6 | **产出物管理** | 收集、存储、分发Agent产出（diff/报告/日志） | 低 |
| 7 | **人工审批** | 高风险操作的人工确认流程 | 中 |
| 8 | **监控查询** | 实时状态推送、日志查看、统计分析 | 低 |

### 3.2 任务管理模块

#### 核心概念

- **Task**：用户提交的完整请求，包含目标、约束、验收标准
- **Subtask**：Task拆解后的执行单元，一个Task对应1-N个Subtask
- **状态机**：Task和Subtask共享同一套状态定义

#### 状态定义

```
pending       — 排队中
decomposing   — 正在拆解
dispatched    — 已分发给Agent
running       — 执行中
reviewing     — Agent产出审查中
confirming    — 等待用户确认
completed     — 已完成（终态）
failed        — 失败（终态，可重试）
cancelled     — 已取消（终态）
```

#### 状态转换规则

```
pending ──[开始拆解]──▶ decomposing
  │
  └──[用户取消]──▶ cancelled

decomposing ──[拆解完成]──▶ dispatched
  │
  ├──[拆解失败]──▶ failed
  └──[用户取消]──▶ cancelled

dispatched ──[Agent开始执行]──▶ running
  │
  ├──[无可分配Agent]──▶ failed
  └──[用户取消]──▶ cancelled

running ──[Agent完成]──▶ reviewing
  │
  ├──[Agent执行出错]──▶ failed
  ├──[需要用户确认]──▶ confirming
  └──[用户取消]──▶ cancelled

reviewing ──[审查通过]──▶ completed
  │
  ├──[审查不通过]──▶ running    // 重新执行
  └──[审查不通过且无法修复]──▶ failed

confirming ──[用户批准]──▶ running
  │
  ├──[用户拒绝]──▶ failed
  ├──[超时未响应(默认5分钟)]──▶ failed  // 默认拒绝
  └──[用户取消]──▶ cancelled
```

**关键规则**:
1. 终态（completed/failed/cancelled）不可再转换
2. confirming状态有超时机制，默认5分钟，超时自动转为failed
3. failed的任务可以"重试"——创建新任务，复制原任务的goal+constraints

#### 核心数据结构

```go
// Task — 完整任务
type Task struct {
    ID                    string        // ULID，如 "task_a1b2c3d4"
    Goal                  string        // 任务目标
    Status                TaskStatus    // 当前状态
    Priority              int           // 0-9，默认5
    RepositoryURL         string        // Git仓库地址
    BaseBranch            string        // 基于哪个分支
    ResultBranch          string        // 结果分支：{base}/agent/{task-id}
    Constraints           []string      // 约束条件
    VerificationCriteria  []string      // 验收标准
    AgentHint             *AgentHint    // 用户指定的Agent偏好
    Progress              float64       // 0-100
    TokensUsed            int           // 累计token消耗
    EstimatedCost         float64       // 预估费用
    AgentsUsed            int           // 使用的Agent数
    ClientID              string        // 提交者
    Tags                  []string      // 标签
    CreatedAt             time.Time
    StartedAt             *time.Time
    CompletedAt           *time.Time
}

// Subtask — 子任务（执行单元）
type Subtask struct {
    ID              string        // 如 "sub_001"
    TaskID          string        // 所属任务
    Type            SubtaskType   // analysis/coding/review/testing/research
    Description     string        // 子任务描述
    AgentTemplate   string        // 使用的Agent角色ID
    AgentInstance   *string       // 实际运行的Agent实例ID
    Status          TaskStatus    // 子任务状态
    Summary         *string       // 执行摘要
    Artifacts       []ArtifactRef // 产出物引用
    TokensUsed      int           // 该子任务的token消耗
    Dependencies    []string      // 依赖的其他subtask ID（DAG）
    StartedAt       *time.Time
    CompletedAt     *time.Time
}

// SubtaskType — 子任务类型
type SubtaskType string
const (
    SubtaskAnalysis  SubtaskType = "analysis"
    SubtaskCoding    SubtaskType = "coding"
    SubtaskReview    SubtaskType = "review"
    SubtaskTesting   SubtaskType = "testing"
    SubtaskResearch  SubtaskType = "research"
)

// AgentHint — 用户对Agent的偏好
type AgentHint struct {
    Templates []string // 建议使用的Agent角色
    Model     *string  // 覆盖默认模型
    MaxAgents int      // 最大并发Agent数
}
```

#### 对外接口

```protobuf
syntax = "proto3";
package cap.v1;

service TaskService {
    rpc SubmitTask(SubmitTaskRequest) returns (SubmitTaskResponse);
    rpc GetTask(GetTaskRequest) returns (Task);
    rpc ListTasks(ListTasksRequest) returns (ListTasksResponse);
    rpc CancelTask(CancelTaskRequest) returns (CancelTaskResponse);
    rpc DecideTask(DecideTaskRequest) returns (DecideTaskResponse);
    rpc GetDiff(GetDiffRequest) returns (DiffResponse);
    rpc ListAgentTemplates(ListAgentTemplatesRequest) returns (ListAgentTemplatesResponse);
    rpc GetPlatformStatus(GetPlatformStatusRequest) returns (PlatformStatus);
}

// MCP工具映射（同一套接口同时暴露为MCP Tools）
// task_submit    → SubmitTask
// task_status    → GetTask
// task_list      → ListTasks
// task_cancel    → CancelTask
// task_decide    → DecideTask
// task_diff      → GetDiff
// task_wait      → GetTask（轮询直到完成）
// agent_templates → ListAgentTemplates
// platform_status → GetPlatformStatus
```

### 3.3 编排调度模块

#### 核心职责

一个Task提交后，编排模块决定：
1. 这个任务需不需要拆解？
2. 拆成几个子任务？什么类型？
3. 子任务之间谁先谁后、谁跟谁并行？
4. 每个子任务分配什么角色的Agent？用哪个LLM模型？

#### 拆解策略矩阵

| 复杂度 | 判断依据 | 处理方式 | 执行路径 |
|--------|---------|---------|----------|
| **简单** | 单文件修改、<100行、无依赖 | 不拆解，直接交给Executor | pending→running→completed |
| **中等** | 多文件联动、3-10个文件、模块间有依赖 | 按文件/模块拆解，保留DAG | 多个Subtask按依赖顺序执行 |
| **复杂** | 架构级变更、需设计决策、多角色协作 | 按专业角色流水线执行 | Observer→Strategist→Executor→Guardian→Tester |

#### Agent角色定义

| 角色ID | 名称 | 职责 | 默认模型 | 能力特点 |
|--------|------|------|----------|----------|
| observer | 分析观察者 | 分析代码结构、识别依赖、评估影响 | Claude Sonnet | 分析5、研究4 |
| strategist | 策略规划师 | 制定修改策略、设计实现方案 | Claude Sonnet | 分析5、编码4 |
| executor | 代码执行者 | 执行具体代码编写和修改 | Claude Sonnet | 编码5 |
| guardian | 安全审查员 | 审查安全性、检查约束条件 | GLM-5.1 | 审查5 |
| tester | 测试工程师 | 编写和执行测试 | GLM-5.1 | 测试5 |
| researcher | 深度研究员 | 技术研究、调研最佳实践 | Claude Sonnet | 研究5 |

#### 匹配算法

```
输入：子任务（类型+描述+要求能力）
输出：最佳Agent角色+LLM模型

步骤：
1. 过滤：筛选capabilities满足子任务最低要求的模板
2. 排序：综合得分 = 能力匹配度×0.5 + 历史成功率×0.3 + 成本效率×0.2
3. 选择：取排名第一的模板
4. 模型路由：根据任务复杂度选择具体模型
   - 简单任务→便宜模型（GLM-5.1）
   - 复杂任务→高质量模型（Claude Sonnet）
   - 用户可通过agentHint覆盖
```

### 3.4 Agent执行模块

#### ReAct执行循环

```
1. 接收：goal + context + tools + constraints
2. 思考（Thought）：LLM分析当前状态，决定下一步
3. 行动（Action）：调用工具或输出结果
4. 观察（Observation）：获取工具返回值
5. 重复2-4，直到：
   - 目标完成 → 返回结果
   - 达到最大步数（默认15步）→ 返回部分结果
   - 无法恢复的错误 → 返回错误
6. 上报：产出物+摘要+token消耗
```

#### 沙箱环境

| 维度 | 限制 | 说明 |
|------|------|------|
| CPU | 1核 | |
| 内存 | 2GB | |
| 磁盘 | /workspace + /tmp(512MB) | 其余只读 |
| 网络 | 白名单域名 | 仅LLM API + Git仓库 |
| 进程数 | 最多50个 | |
| 执行时间 | 默认30分钟超时 | 超时强制销毁 |
| 安全 | 禁止sudo/禁止内网访问/禁止特权操作 | seccomp+AppArmor+Cubesandbox |

#### 多模型路由

| 子任务类型 | 默认模型 | 理由 |
|-----------|---------|------|
| analysis | Claude Sonnet | 需要深度推理 |
| coding | Claude Sonnet | 代码质量要求高 |
| review | GLM-5.1 | 审查不需要创意，性价比高 |
| testing | GLM-5.1 | 测试生成偏模板化 |
| research | Claude Sonnet | 需要综合多源信息 |

自适应优化：连续3次成功率<80%降级，连续5次成功率>95%且成本低升级为主选。

### 3.5 上下文管理模块

#### 三层存储

| 层 | 存储 | 访问速度 | 内容 | TTL |
|----|------|---------|------|-----|
| **热层** | Redis | 毫秒级 | 活跃上下文、Agent状态、分布式锁 | 2-24小时 |
| **温层** | PostgreSQL | 秒级 | 任务元数据、压缩快照、审计日志 | 永久 |
| **冷层** | MinIO | 分钟级 | 完整上下文归档、产出物 | 90天 |

#### 上下文压缩（两级策略）

> **实现范围**：当前版本实现 L1 + L3，**L2 Embedding去重暂跳过**（需要独立 embedding 服务，依赖成本不划算，L1+L3 组合已覆盖大部分场景）。

```
Level 1: 规则压缩（总是执行，零成本）
  - 删除系统提示和工具描述重复内容
  - 去掉JSON缩进和HTML标签
  - 文件内容→签名摘要（path+checksum+summary）
  - 删除超过N轮的历史对话
  效果：减少20-40%

Level 2: Embedding去重 ⚠️ 暂跳过
  - 依赖独立 embedding 模型服务，当前不引入
  - 后续有需要时再评估接入

Level 3: LLM智能压缩（高成本，按需触发）
  - 仅在L1后仍超预算时触发
  - 输入完整上下文+保留规则
  - 输出压缩后的上下文
  - 必须保留：goal, constraints, user_decisions, error_log
  效果：减少30-50%

硬截断（最后手段）：
  - L1+L3后仍超预算→强制截断
  - 记录丢失信息类别（审计用）
```

#### 上下文传递策略

| 模式 | 适用场景 | 行为 |
|------|---------|------|
| full | 上下文<5K tokens | 完整传递 |
| summary | 5K-20K tokens | LLM生成摘要，丢弃过程 |
| delta | >20K tokens | 只传递变更部分 |

典型传递路径：
- Observer→Strategist：full（分析结果重要）
- Strategist→Executor：summary（策略是核心）
- Executor→Guardian：delta（只需要diff）
- Guardian→Tester：summary（审查结论+改动摘要）

### 3.6 工具系统

#### 工具定义规范

每个工具有明确的输入Schema、输出格式、安全约束。

#### 核心工具集

**文件操作**

| 工具 | 功能 | 输入 | 输出 | 安全约束 |
|------|------|------|------|----------|
| read_file | 读取文件 | path, offset?, limit? | content, totalLines | 禁止访问/workspace外 |
| write_file | 写入文件 | path, content | bytesWritten | 单次最多10MB |
| edit_file | 局部修改 | path, oldString, newString | replaced, count | 精确匹配替换 |
| list_files | 列出目录 | path, recursive? | files[] | |
| search_code | 代码搜索 | pattern, path?, fileType? | results[] | 支持正则 |

**Git操作**

| 工具 | 功能 | 安全约束 |
|------|------|----------|
| git_status | 查看Git状态 | |
| git_diff | 查看diff | |
| git_commit | 提交改动 | 禁止push到main/master |
| git_push | 推送分支 | 禁止force push（除非显式允许） |

**命令执行**

| 工具 | 功能 | 安全约束 |
|------|------|----------|
| execute_command | 执行shell命令 | 禁止sudo/su/rm -rf/网络扫描；curl只能访问白名单；最长60秒 |

**LLM工具**

| 工具 | 功能 | 说明 |
|------|------|------|
| ask_llm | 向LLM提问 | 用于技术研究，不计入任务token预算 |

#### 角色工具集分配

| 角色 | 可用工具 |
|------|----------|
| observer | read_file, list_files, search_code, git_status, ask_llm |
| strategist | read_file, list_files, search_code, ask_llm |
| executor | read_file, write_file, edit_file, list_files, search_code, git_status, git_diff, git_commit, execute_command |
| guardian | read_file, git_diff, search_code |
| tester | read_file, write_file, edit_file, execute_command, git_status, git_diff |
| researcher | read_file, list_files, search_code, execute_command, ask_llm |

### 3.7 产出物管理模块

```go
// Artifact — 产出物
type Artifact struct {
    ID        string      // 唯一标识
    TaskID    string
    SubtaskID string
    Type      ArtifactType // analysis/diff/test_result/report/log
    Summary   string      // 人类可读摘要
    URL       string      // 下载地址（签名URL，有效期1小时）
    Size      int64       // 字节数
    CreatedAt time.Time
}

// TaskResult — 任务最终结果
type TaskResult struct {
    GitCommit     string        // commit hash
    GitBranch     string        // 结果分支名
    Summary       string        // 任务总结
    Changes       []FileChange  // 改动文件列表
    TestResults   *TestResult   // 测试结果
    QualityScore  *float64      // 质量评分0-100
}

type FileChange struct {
    Path      string // 文件路径
    Action    string // added/modified/deleted
    Additions int    // 新增行数
    Deletions int    // 删除行数
}
```

### 3.8 人工审批模块

#### 触发条件

**自动触发**（Guardian Agent检测到）：
- 修改安全相关代码（认证/授权/加密）
- 删除超过50行代码
- 修改配置文件（.env, config文件）
- 添加外部依赖
- 修改数据库schema相关代码

**手动触发**（用户设置）：
- 提交时设置requireApproval: true
- 超过指定金额（estimatedCost > 1元）

#### 确认流程

```
1. Guardian检测到高风险操作
2. Agent暂停执行，状态变为confirming
3. 平台通过WebSocket推送confirmation_required事件
4. 用户查看diff预览
5. 用户决策：approve/reject/modify
   - approve → Agent继续执行
   - reject → 子任务失败
   - modify → Agent根据建议重新执行
6. 超时（默认5分钟）→ 自动拒绝
```

### 3.9 监控查询模块

#### WebSocket事件类型

```typescript
type WSEvent =
  // 任务级
  | { type: 'task.status_changed'; taskId; previousStatus; currentStatus; progress }
  | { type: 'task.subtask_completed'; taskId; subtaskId; summary; duration }
  | { type: 'task.completed'; taskId; result }
  | { type: 'task.failed'; taskId; error; canRetry }
  | { type: 'task.confirmation_required'; taskId; preview: { description; diff; affectedFiles } }
  
  // Agent级
  | { type: 'agent.log'; taskId; subtaskId; agentId; level; message }
  | { type: 'agent.tool_called'; taskId; subtaskId; toolName; input }
  | { type: 'agent.llm_request'; taskId; subtaskId; model; tokens }
  
  // 平台级
  | { type: 'platform.worker_pool'; total; idle; busy; pending }
```

#### 业务指标

```
任务指标：
  cap.tasks.submitted      Counter   任务提交总数
  cap.tasks.completed      Counter   任务完成总数
  cap.tasks.failed         Counter   任务失败总数
  cap.tasks.duration       Histogram 任务完成耗时
  cap.tasks.queue_wait     Histogram 队列等待时间
  cap.tasks.active         Gauge     当前活跃任务数

Agent指标：
  cap.agents.active        Gauge     当前活跃Agent数
  cap.agents.idle          Gauge     空闲Agent数
  cap.agent.execution_time Histogram Agent执行耗时
  cap.agent.token_usage    Counter   Token消耗
  cap.agent.cost           Counter   费用（元）

LLM指标：
  cap.llm.requests         Counter   LLM API调用次数
  cap.llm.errors           Counter   LLM API错误次数
  cap.llm.latency          Histogram LLM API延迟
  cap.llm.tokens_input     Counter   输入tokens
  cap.llm.tokens_output    Counter   输出tokens

上下文指标：
  cap.context.size         Histogram 上下文大小（tokens）
  cap.context.compression  Histogram 压缩率
```

---

## 四、Schema定义

### 4.1 数据库Schema（ent）

```go
// ent/schema/task.go
package schema

import (
    "time"
    "entgo.io/ent"
    "entgo.io/ent/schema/edge"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type Task struct {
    ent.Schema
}

func (Task) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").MaxLen(64).Unique().Immutable(),
        field.String("goal").NotEmpty(),
        field.Enum("status").
            Values("pending", "decomposing", "dispatched", "running",
                "reviewing", "confirming", "completed", "failed", "cancelled").
            Default("pending"),
        field.Int("priority").Range(0, 9).Default(5),
        field.String("repository_url").NotEmpty(),
        field.String("base_branch").NotEmpty(),
        field.String("result_branch"),
        field.JSON("constraints", []string{}).Optional(),
        field.JSON("verification_criteria", []string{}).Optional(),
        field.JSON("agent_hint", map[string]interface{}{}).Optional(),
        field.Int("tokens_used").Default(0),
        field.Float("estimated_cost").Default(0),
        field.Int("agents_used").Default(0),
        field.Float("progress").Default(0),
        field.String("client_id").NotEmpty(),
        field.JSON("tags", []string{}).Optional(),
        field.Time("created_at").Default(time.Now).Immutable(),
        field.Time("started_at").Optional(),
        field.Time("completed_at").Optional(),
    }
}

func (Task) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("subtasks", Subtask.Type),
        edge.To("audit_logs", AuditLog.Type),
    }
}

func (Task) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("status"),
        index.Fields("client_id"),
        index.Fields("created_at"),
    }
}
```

```go
// ent/schema/subtask.go
package schema

import "entgo.io/ent"
import "entgo.io/ent/schema/edge"
import "entgo.io/ent/schema/field"

type Subtask struct {
    ent.Schema
}

func (Subtask) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").MaxLen(64).Unique().Immutable(),
        field.String("task_id"),
        field.Enum("type").Values("analysis", "coding", "review", "testing", "research"),
        field.Text("description"),
        field.String("agent_template").NotEmpty(),
        field.String("agent_instance").Optional(),
        field.Enum("status").
            Values("pending", "decomposing", "dispatched", "running",
                "reviewing", "confirming", "completed", "failed", "cancelled").
            Default("pending"),
        field.Text("summary").Optional(),
        field.JSON("artifacts", []map[string]interface{}{}).Optional(),
        field.Int("tokens_used").Default(0),
        field.JSON("dependencies", []string{}).Optional(),
        field.Time("started_at").Optional(),
        field.Time("completed_at").Optional(),
    }
}

func (Subtask) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("task", Task.Type).Ref("subtasks").Unique().Required(),
    }
}
```

```go
// ent/schema/audit_log.go
package schema

import (
    "time"
    "entgo.io/ent"
    "entgo.io/ent/schema/edge"
    "entgo.io/ent/schema/field"
)

type AuditLog struct {
    ent.Schema
}

func (AuditLog) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").MaxLen(64).Unique().Immutable(),
        field.String("task_id"),
        field.String("subtask_id").Optional(),
        field.String("agent_template").Optional(),
        field.String("action").NotEmpty(),
        field.Enum("level").Values("info", "warning", "error", "critical").Default("info"),
        field.Text("message"),
        field.JSON("details", map[string]interface{}{}).Optional(),
        field.Time("timestamp").Default(time.Now).Immutable(),
    }
}

func (AuditLog) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("task", Task.Type).Ref("audit_logs").Unique().Required(),
    }
}
```

### 4.2 Protobuf接口定义

```protobuf
// proto/cap/v1/task.proto
syntax = "proto3";
package cap.v1;
option go_package = "gen/cap/v1;capv1";

service TaskService {
    rpc SubmitTask(SubmitTaskRequest) returns (SubmitTaskResponse);
    rpc GetTask(GetTaskRequest) returns (Task);
    rpc ListTasks(ListTasksRequest) returns (ListTasksResponse);
    rpc CancelTask(CancelTaskRequest) returns (CancelTaskResponse);
    rpc DecideTask(DecideTaskRequest) returns (DecideTaskResponse);
    rpc GetDiff(GetDiffRequest) returns (DiffResponse);
    rpc ListAgentTemplates(ListAgentTemplatesRequest) returns (ListAgentTemplatesResponse);
    rpc GetPlatformStatus(GetPlatformStatusRequest) returns (PlatformStatus);
}

message SubmitTaskRequest {
    string goal = 1;
    string repository_url = 2;
    string base_branch = 3;
    repeated string constraints = 4;
    repeated string verification_criteria = 5;
    int32 priority = 6;
    int32 timeout = 7;
    AgentHint agent_hint = 8;
    repeated string tags = 9;
}

message SubmitTaskResponse {
    string task_id = 1;
    string status = 2;
    string result_branch = 3;
    int64 created_at = 4;
}

message GetTaskRequest {
    string task_id = 1;
}

message Task {
    string task_id = 1;
    string goal = 2;
    string status = 3;
    int32 priority = 4;
    float progress = 5;
    int64 tokens_used = 6;
    float estimated_cost = 7;
    int32 agents_used = 8;
    int64 created_at = 9;
    repeated Subtask subtasks = 10;
}

message Subtask {
    string subtask_id = 1;
    string type = 2;
    string description = 3;
    string agent_template = 4;
    string status = 5;
    string summary = 6;
    int64 tokens_used = 7;
    int64 started_at = 8;
    int64 completed_at = 9;
}

message ListTasksRequest {
    string status = 1;
    string client_id = 2;
    int32 page = 3;
    int32 page_size = 4;
}

message ListTasksResponse {
    repeated Task tasks = 1;
    int32 total = 2;
}

message CancelTaskRequest {
    string task_id = 1;
}

message CancelTaskResponse {
    bool success = 1;
    string previous_status = 2;
}

message DecideTaskRequest {
    string task_id = 1;
    string decision = 2;  // approve/reject/modify
    string feedback = 3;
}

message DecideTaskResponse {
    string task_id = 1;
    string status = 2;
}

message GetDiffRequest {
    string task_id = 1;
}

message DiffResponse {
    string diff = 1;
    int32 files_changed = 2;
    int32 additions = 3;
    int32 deletions = 4;
}

message AgentHint {
    repeated string templates = 1;
    string model = 2;
    int32 max_agents = 3;
}

message ListAgentTemplatesRequest {}

message AgentTemplate {
    string template_id = 1;
    string name = 2;
    string description = 3;
    string default_model = 4;
}

message ListAgentTemplatesResponse {
    repeated AgentTemplate templates = 1;
}

message GetPlatformStatusRequest {}

message PlatformStatus {
    int32 total_workers = 1;
    int32 idle_workers = 2;
    int32 busy_workers = 3;
    int32 pending_tasks = 4;
    int32 running_tasks = 5;
    float queue_wait_avg = 6;
}
```

---

## 五、核心接口契约

### 5.1 REST API

所有RPC方法同时通过connect-go暴露为REST端点。

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/tasks` | 提交任务 |
| GET | `/api/v1/tasks/:id` | 查询任务 |
| GET | `/api/v1/tasks` | 列出任务（支持status/client_id过滤） |
| POST | `/api/v1/tasks/:id/cancel` | 取消任务 |
| POST | `/api/v1/tasks/:id/decide` | 用户决策 |
| GET | `/api/v1/tasks/:id/diff` | 获取代码改动diff |
| GET | `/api/v1/agent-templates` | 列出Agent角色模板 |
| GET | `/api/v1/platform/status` | 平台状态 |

### 5.2 WebSocket

**连接**: `wss://api.host/api/v1/ws`

**认证**: 首条消息发送 `{ "type": "auth", "token": "jwt-token" }`

**订阅**: `/{taskId}` 按任务ID分房间

### 5.3 MCP协议

**传输**: stdio（本地）/ SSE（生产）

**Tools**:

| Tool | 方法 | 说明 |
|------|------|------|
| task_submit | SubmitTask | 提交任务 |
| task_status | GetTask | 查询任务状态 |
| task_list | ListTasks | 列出任务 |
| task_cancel | CancelTask | 取消任务 |
| task_decide | DecideTask | 对确认请求做决策 |
| task_diff | GetDiff | 获取代码diff |
| task_wait | GetTask(轮询) | 阻塞等待任务完成 |
| agent_templates | ListAgentTemplates | 列出可用Agent角色 |
| platform_status | GetPlatformStatus | 查看平台状态 |

**Resources**:

| URI | 内容 |
|-----|------|
| `cap://tasks/{taskId}/log` | 任务执行日志 |
| `cap://tasks/{taskId}/timeline` | 决策时间线 |
| `cap://tasks/{taskId}/artifact/{id}` | 产出物文件 |
| `cap://platform/status` | 平台实时状态 |

---

## 六、Agent编排引擎

### 6.1 编排引擎职责

编排引擎是平台的"大脑"，使用Eino实现：

1. **任务分析**：判断复杂度（简单/中等/复杂）
2. **任务拆解**：根据复杂度生成Subtask列表和依赖DAG
3. **Agent匹配**：为每个Subtask选择最佳Agent角色和LLM模型
4. **执行调度**：按依赖关系调度，支持串行和并行
5. **状态推进**：通过Eino Callback驱动任务状态机

### 6.2 编排图设计

```go
// 任务编排图的构建
func BuildTaskGraph(deps *Dependencies) (*compose.Graph, error) {
    g := compose.NewGraph[TaskContext, TaskResult]()
    
    // Phase 1: 任务分析
    g.AddNode("analyzer", NewAnalyzerNode(deps.ModelRouter))
    
    // Phase 2: 路由决策
    g.AddNode("router", &ComplexityRouter{})
    g.AddEdge("analyzer", "router")
    
    // 分支1: 简单任务
    g.AddNode("simple_executor", NewExecutorNode(deps))
    g.AddEdge("router", "simple_executor", 
        compose.WithEdgeCondition(func(ctx TaskContext) bool {
            return ctx.Complexity == "simple"
        }))
    
    // 分支2: 中等任务（并行执行）
    g.AddNode("medium_decomposer", NewModuleDecomposer(deps))
    g.AddNode("medium_executors", NewParallelExecutors(deps))
    g.AddEdge("router", "medium_decomposer",
        compose.WithEdgeCondition(func(ctx TaskContext) bool {
            return ctx.Complexity == "medium"
        }))
    g.AddEdge("medium_decomposer", "medium_executors")
    
    // 分支3: 复杂任务（角色流水线）
    g.AddNode("observer", NewAgentNode("observer", deps))
    g.AddNode("strategist", NewAgentNode("strategist", deps))
    g.AddNode("executor", NewAgentNode("executor", deps))
    g.AddNode("guardian", NewGuardianNode(deps))  // 人工确认
    g.AddNode("tester", NewAgentNode("tester", deps))
    
    g.AddEdge("router", "observer",
        compose.WithEdgeCondition(func(ctx TaskContext) bool {
            return ctx.Complexity == "complex"
        }))
    g.AddEdge("observer", "strategist", compose.WithContextPassing("summary"))
    g.AddEdge("strategist", "executor", compose.WithContextPassing("full"))
    g.AddEdge("executor", "guardian", compose.WithContextPassing("delta"))
    g.AddEdge("guardian", "tester", compose.WithContextPassing("summary"))
    
    // Phase 3: 结果合并
    g.AddNode("merger", &ResultMerger{})
    g.AddEdge("simple_executor", "merger")
    g.AddEdge("medium_executors", "merger")
    g.AddEdge("tester", "merger")
    
    g.SetEntryPoint("analyzer")
    return g.Compile(context.Background())
}
```

### 6.3 回调与状态推送

```go
// Eino Callback → WebSocket 状态推送
type OrchestratorCallback struct {
    wsHub    *WebSocketHub
    taskRepo *ent.TaskClient
}

func (cb *OrchestratorCallback) OnStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) {
    cb.wsHub.Broadcast(info.TaskID, WSEvent{
        Type: "task.status_changed",
        Payload: map[string]interface{}{
            "taskId": info.TaskID,
            "currentStatus": "running",
            "progress": 0,
        },
    })
}

func (cb *OrchestratorCallback) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) {
    cb.wsHub.Broadcast(info.TaskID, WSEvent{
        Type: "task.completed",
        Payload: map[string]interface{}{"taskId": info.TaskID},
    })
    cb.wsHub.CloseRoom(info.TaskID)
}

func (cb *OrchestratorCallback) OnStream(ctx context.Context, info *callbacks.RunInfo, chunk *schema.Message) {
    cb.wsHub.Broadcast(info.TaskID, WSEvent{
        Type: "agent.log",
        Payload: map[string]interface{}{
            "taskId":    info.TaskID,
            "subtaskId": info.SubtaskID,
            "agentId":   info.AgentID,
            "message":   chunk.Content,
        },
    })
}
```

---

## 七、工具系统设计

### 7.1 工具接口定义

```go
// Tool — Agent可调用的工具
type Tool interface {
    Info() ToolInfo
    Run(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
}

type ToolInfo struct {
    Name        string
    Description string
    InputSchema map[string]interface{}  // JSON Schema
}
```

### 7.2 核心工具实现

```go
// ReadFileTool — 文件读取
type ReadFileTool struct {
    Workspace string // /workspace
}

func (t *ReadFileTool) Info() ToolInfo {
    return ToolInfo{
        Name:        "read_file",
        Description: "读取指定文件的内容",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "path":   map[string]interface{}{"type": "string", "description": "文件路径（相对workspace）"},
                "offset": map[string]interface{}{"type": "integer", "description": "起始行号（可选）"},
                "limit":  map[string]interface{}{"type": "integer", "description": "读取行数（默认100）"},
            },
            "required": []string{"path"},
        },
    }
}

func (t *ReadFileTool) Run(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
    path := input["path"].(string)
    
    // 安全检查：防止目录遍历
    fullPath := filepath.Join(t.Workspace, path)
    if !strings.HasPrefix(fullPath, t.Workspace) {
        return nil, fmt.Errorf("path escape detected: %s", path)
    }
    
    content, err := os.ReadFile(fullPath)
    if err != nil {
        return nil, err
    }
    
    // 大小限制：最多10MB
    if len(content) > 10*1024*1024 {
        return nil, fmt.Errorf("file too large: %d bytes (max 10MB)", len(content))
    }
    
    return map[string]interface{}{
        "content":    string(content),
        "totalLines": strings.Count(string(content), "\n") + 1,
    }, nil
}

// ExecuteCommandTool — 命令执行
type ExecuteCommandTool struct {
    Workspace     string
    AllowedHosts []string // 网络白名单
    MaxTimeout    time.Duration
}

func (t *ExecuteCommandTool) Run(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
    command := input["command"].(string)
    cwd := t.Workspace
    if v, ok := input["cwd"].(string); ok && v != "" {
        cwd = filepath.Join(t.Workspace, v)
    }
    
    // 安全检查
    if err := t.validateCommand(command); err != nil {
        return nil, err
    }
    
    ctx, cancel := context.WithTimeout(ctx, t.MaxTimeout)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, "sh", "-c", command)
    cmd.Dir = cwd
    
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    err := cmd.Run()
    
    return map[string]interface{}{
        "stdout":   stdout.String(),
        "stderr":   stderr.String(),
        "exitCode": cmd.ProcessState.ExitCode(),
    }, nil
}

func (t *ExecuteCommandTool) validateCommand(command string) error {
    forbidden := []string{"sudo", "su ", "rm -rf /", "chmod +s", ">/dev/sda", ":(){:|:&};:"}
    for _, f := range forbidden {
        if strings.Contains(command, f) {
            return fmt.Errorf("forbidden command pattern: %s", f)
        }
    }
    return nil
}
```

---

## 八、上下文与压缩

### 8.1 上下文存储实现

```go
// ContextStore — 上下文管理接口
type ContextStore interface {
    Get(ctx context.Context, taskID string) (*TaskContext, error)
    Update(ctx context.Context, taskID string, updater func(*TaskContext) error) error
    Archive(ctx context.Context, taskID string) error
}

// RedisContextStore — 热层实现
type RedisContextStore struct {
    client *redis.Client
    budget int // 默认token预算(50K)
}

func (s *RedisContextStore) Get(ctx context.Context, taskID string) (*TaskContext, error) {
    key := fmt.Sprintf("cap:context:%s", taskID)
    data, err := s.client.Get(ctx, key).Bytes()
    if err == redis.Nil {
        return nil, fmt.Errorf("context not found: %s", taskID)
    }
    if err != nil {
        return nil, err
    }
    
    var tc TaskContext
    if err := sonic.Unmarshal(data, &tc); err != nil {
        return nil, err
    }
    return &tc, nil
}

func (s *RedisContextStore) Update(ctx context.Context, taskID string, updater func(*TaskContext) error) error {
    key := fmt.Sprintf("cap:context:%s", taskID)
    lockKey := fmt.Sprintf("cap:lock:context:%s", taskID)
    
    // 分布式锁（Redlock + 看门狗）
    lock, err := s.locker.Obtain(ctx, lockKey, 10*time.Second, nil)
    if err != nil {
        return fmt.Errorf("failed to acquire lock: %w", err)
    }
    defer lock.Release(ctx)
    
    // 读取当前值
    tc, err := s.Get(ctx, taskID)
    if err != nil {
        return err
    }
    
    // 应用更新
    if err := updater(tc); err != nil {
        return err
    }
    
    // 检查预算，超预算触发压缩
    if tc.TokenUsed > s.budget {
        compressed, err := s.compress(ctx, tc)
        if err != nil {
            return err
        }
        tc = compressed
    }
    
    // 写入（带TTL）
    data, _ := sonic.Marshal(tc)
    return s.client.Set(ctx, key, data, 24*time.Hour).Err()
}
```

### 8.2 压缩引擎

> L2 Embedding去重暂跳过，当前实现 L1 规则压缩 + L3 LLM智能压缩两级。

```go
// CompressionEngine — 两级压缩（L2 Embedding 暂跳过）
type CompressionEngine struct {
    l1 *RuleCompressor  // 规则压缩（零成本）
    l3 *LLMCompressor   // LLM智能压缩（高成本）
    // l2 *EmbeddingDeduplicator  // 暂跳过：需要独立 embedding 服务
}

func (e *CompressionEngine) Compress(ctx context.Context, tc *TaskContext, budget int) (*TaskContext, error) {
    tokens := estimateTokens(tc)
    if tokens <= budget {
        return tc, nil // 不需要压缩
    }
    
    // Level 1: 规则压缩
    tc = e.l1.Compress(tc)
    tokens = estimateTokens(tc)
    if tokens <= budget {
        return tc, nil
    }
    
    // Level 3: LLM智能压缩（L1后仍超预算才触发）
    tc, err := e.l3.Compress(ctx, tc, budget)
    if err != nil {
        // L3失败→硬截断
        return e.hardTruncate(tc, budget), nil
    }
    
    return tc, nil
}

// RuleCompressor — Level 1（纯代码实现，零成本）
type RuleCompressor struct{}

func (r *RuleCompressor) Compress(tc *TaskContext) *TaskContext {
    // 1. 格式化压缩（去掉无用空白）
    tc = r.minifyFormat(tc)
    
    // 2. 文件内容→签名摘要
    tc = r.summarizeFiles(tc)
    
    // 3. 删除冗余对话轮次
    tc = r.truncateConversation(tc, 20)
    
    // 4. 删除可压缩字段（按优先级）
    tc = r.dropCompressible(tc)
    
    return tc
}

func (r *RuleCompressor) summarizeFiles(tc *TaskContext) *TaskContext {
    // 将完整文件内容替换为签名摘要
    for i, fs := range tc.FileStates {
        if len(fs.Content) > 1000 {
            tc.FileStates[i].Content = "" // 清空内容
            tc.FileStates[i].Summary = fmt.Sprintf("// %s (%d lines, sha256:%s)",
                fs.Path, fs.LineCount, fs.Checksum[:8])
            tc.FileStates[i].FullContentURL = fmt.Sprintf("cap://tasks/%s/artifact/%s",
                tc.TaskID, fs.ArtifactID)
        }
    }
    return tc
}
```

---

## 九、Worker沙箱

### 9.1 沙箱策略

> **并行实现策略**：Docker 和 CubeSandbox 同时实现，通过配置项 `sandbox.backend` 切换，方便对比验证。Docker 作为基线保障稳定性，CubeSandbox 验证高性能路径。

| 方案 | 状态 | 启动时间 | 内存占用 | 说明 |
|------|------|---------|----------|------|
| **Docker** + seccomp + AppArmor | ✅ 并行实现 | 1-3s | 2GB | 成熟稳定，作为对照基线 |
| **CubeSandbox** | ✅ 并行实现 | <60ms | <5MB | 高性能路径，MicroVM 硬件隔离 |
| gVisor / Kata | 备选 | 2-10s | 2GB | 如 CubeSandbox 不可用的降级 |

配置切换：

```yaml
sandbox:
  backend: "docker"       # "docker" | "cubesandbox"
  fallback_to_docker: true  # cubesandbox 失败时自动降级
```

### 9.2 双沙箱并行实现

```go
// SandboxBackend — 沙箱后端接口（两种实现共用）
type SandboxBackend interface {
    Create(ctx context.Context, spec WorkerSpec) (*Worker, error)
    Destroy(ctx context.Context, worker *Worker) error
    IsAvailable(ctx context.Context) bool
}

// WorkerManager — 按配置选择后端，支持自动降级
type WorkerManager struct {
    primary  SandboxBackend  // 配置指定的主后端
    fallback SandboxBackend  // Docker降级后端（永远可用）
    pool     *WorkerPool
}

func (m *WorkerManager) AcquireWorker(ctx context.Context, spec WorkerSpec) (*Worker, error) {
    // 1. 尝试主后端
    if m.primary.IsAvailable(ctx) {
        worker, err := m.primary.Create(ctx, spec)
        if err == nil {
            return worker, nil
        }
        slog.Warn("primary sandbox failed, falling back to docker",
            zap.Error(err), zap.String("backend", spec.Backend))
    }
    // 2. 降级到 Docker（fallback 永远是 DockerBackend）
    return m.fallback.Create(ctx, spec)
}

// DockerBackend — Docker实现
type DockerBackend struct {
    client *docker.Client
}

func (b *DockerBackend) Create(ctx context.Context, spec WorkerSpec) (*Worker, error) {
    container, err := b.client.ContainerCreate(ctx, &container.Config{
        Image: "cap-worker:latest",
        Cmd:   []string{"/app/worker"},
        Env:   spec.EnvVars,
    }, &container.HostConfig{
        SecurityOpt:    []string{"no-new-privileges:true", "seccomp:./seccomp-worker.json"},
        ReadonlyRootfs: true,
        Tmpfs:          map[string]string{"/tmp": "noexec,nosuid,size=512M"},
        CapDrop:        []string{"ALL"},
        Resources:      container.Resources{Memory: 2 << 30, CPUQuota: 100000},
    }, nil, nil, "")
    if err != nil {
        return nil, err
    }
    return &Worker{ID: container.ID, Type: "docker"}, nil
}

// CubeSandboxBackend — CubeSandbox实现
type CubeSandboxBackend struct {
    client *cubesandbox.Client
}

func (b *CubeSandboxBackend) Create(ctx context.Context, spec WorkerSpec) (*Worker, error) {
    sandbox, err := b.client.CreateSandbox(ctx, &cubesandbox.SandboxConfig{
        CPU:    1,
        Memory: 2 * 1024 * 1024 * 1024,
        Network: cubesandbox.NetworkConfig{
            AllowHosts: []string{"api.anthropic.com", "open.bigmodel.cn", "github.com"},
        },
        ReadOnlyRoot: true,
    })
    if err != nil {
        return nil, err
    }
    return &Worker{ID: sandbox.ID, Type: "cubesandbox"}, nil
}

func (b *CubeSandboxBackend) IsAvailable(ctx context.Context) bool {
    return b.client != nil && b.client.Ping(ctx) == nil
}
```

---

## 十、监控与可观测性

### 10.1 日志规范

```go
// 结构化日志，每条日志包含：
{
    "timestamp": "2026-05-13T10:00:00Z",
    "level": "info",
    "request_id": "req_abc123",      // 请求追踪ID
    "task_id": "task_def456",        // 任务ID（如有）
    "agent_id": "agent_ghi789",      // Agent实例ID（如有）
    "subtask_id": "sub_jkl012",      // 子任务ID（如有）
    "message": "Agent execution started",
    "metadata": {
        "template": "executor",
        "model": "claude-sonnet-4"
    }
}
```

### 10.2 链路追踪

使用OpenTelemetry，追踪路径：`API → Coordinator → Worker → LLM API`

关键Span：
- `task.submit` — 任务提交
- `task.decompose` — 任务拆解
- `subtask.execute` — 子任务执行
- `agent.react_loop` — ReAct循环
- `llm.request` — LLM API调用
- `tool.call` — 工具调用

### 10.3 业务指标

```go
// Prometheus指标定义
var (
    TasksSubmitted = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "cap_tasks_submitted_total",
        Help: "Total number of tasks submitted",
    })
    TasksCompleted = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "cap_tasks_completed_total",
        Help: "Total number of tasks completed",
    }, []string{"status"}) // status=success/failed
    TaskDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name:    "cap_task_duration_seconds",
        Help:    "Task completion duration",
        Buckets: prometheus.DefBuckets,
    })
    AgentCost = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "cap_agent_cost_yuan",
        Help: "Agent execution cost in CNY",
    }, []string{"model", "template"})
)
```

---

## 十一、安全设计

### 11.1 安全分层

| 层 | 措施 | 说明 |
|----|------|------|
| 认证 | JWT + API Key | 24h过期 + 刷新机制 |
| 授权 | RBAC | 6个权限角色（submit/query/cancel/decide/admin/manage） |
| 限流 | sentinel-go | 全局100req/s + 按client_id 10req/s + 按任务5并发 |
| 传输 | HTTPS/WSS | TLS 1.3 |
| 沙箱 | CubeSandbox + seccomp + AppArmor | 硬件级隔离 |
| 网络安全 | 出站白名单 | 仅LLM API + Git仓库 |
| 代码安全 | 工具权限控制 | 按角色分配工具集 |
| 数据安全 | 敏感字段脱敏 | API Key/Git Token不返回给客户端 |
| 审计 | 全操作审计日志 | 不可篡改，独立存储 |

### 11.2 Git安全

- **双容器隔离**：Agent容器（不可信，无Git权限）+ Git容器（可信，持有凭据）
- **短期令牌**：GitHub App安装令牌，TTL 1小时，Vault管理
- **权限最小化**：仅Contents read/write，禁止删除仓库

### 11.3 沙箱安全加固

```yaml
# seccomp-worker.json — 禁止的系统调用
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "syscalls": [
    {
      "names": ["mount", "umount2", "pivot_root", "open_by_handle_at",
                "ptrace", "process_vm_writev", "process_vm_readv",
                "socket"],  // 仅允许AF_INET/AF_INET6
      "action": "SCMP_ACT_KILL"
    }
  ]
}
```

---

## 十二、开发计划

### Phase 0：骨架 + 最小闭环（2周）

目标：提交任务 → **单Agent执行** → 返回结果 → Git push

> **本阶段策略**：
> - Worker 沙箱同时实现 Docker + CubeSandbox，通过配置切换验证
> - 编排保持单 Agent 最简路径，不引入多角色和 DAG
> - 不实现上下文压缩，上下文直接全量传递

| # | 任务 | 说明 |
|---|------|------|
| 1 | 项目初始化 | Fork vibe-go，配置Eino依赖，模块重命名 |
| 2 | ent Schema定义 | Task/Subtask/AuditLog三个核心表 |
| 3 | Protobuf接口 | TaskService 8个RPC方法 |
| 4 | L5-Gateway | connect-go路由 + JWT + sentinel-go限流 |
| 5 | L1-Storage | ent + Redis + Outbox轮询 |
| 6 | 最小编排链路 | Submit → pending → running → completed（单Agent直通） |
| 7 | ReAct Agent | LLM调用 + 工具调用（read_file + write_file） |
| 8 | **双沙箱并行实现** | Docker + CubeSandbox 同时实现，config切换，对比验证 |
| 9 | WebSocket推送 | task.status_changed + agent.log |
| 10 | go-git集成 | clone + commit + push |

验收：curl提交"修改README"任务，分别用 Docker 和 CubeSandbox 后端各跑一次，结果一致且Git push成功，WebSocket推送完整状态变更。

### Phase 1：多Agent协作（3周）

| # | 任务 | 说明 |
|---|------|------|
| 1 | Eino Graph编排 | 任务拆解 + 条件分支 + 并行执行 |
| 2 | 6个Agent角色 | Observer/Strategist/Executor/Guardian/Tester/Researcher |
| 3 | Agent匹配算法 | 能力评分 + 历史数据 + 成本优化 |
| 4 | SequentialAgent | Observer → Strategist → Executor → Guardian → Tester |
| 5 | ParallelAgent | 多个分析维度并行 |
| 6 | Guardian审查 | 安全审查 + 人工确认触发 |
| 7 | 上下文传递 | full/summary/delta三种模式 |
| 8 | 人工审批流程 | confirmation_required → decide → 继续/失败 |
| 9 | 子任务DAG执行 | 按依赖关系调度 |
| 10 | MCP Server | 所有接口暴露为MCP Tools |

验收：提交"实现用户登录"任务，自动拆解为3+子任务，多Agent协作完成，Guardian审查通过。

### Phase 2：Worker层 + 生产化（3周）

| # | 任务 | 说明 |
|---|------|------|
| 1 | Worker池管理 | 预热 + 动态扩缩容 + 健康检查（Docker + CubeSandbox各自的池策略） |
| 2 | LLM路由插件 | 多模型自适应路由（Claude/GLM升降级） |
| 3 | **上下文压缩引擎** | L1规则 + L3 LLM（**L2 Embedding暂跳过**） |
| 4 | 冷热分层存储 | Redis → PostgreSQL → MinIO |
| 5 | 业务指标看板 | Prometheus + Grafana |
| 6 | 告警 + 故障预案 | Redis故障/LLM限流/Worker泄漏 |
| 7 | 链路追踪 | OpenTelemetry + Jaeger |

### Phase 3：高级特性（4周，可选）

| # | 任务 | 说明 |
|---|------|------|
| 1 | FlowGram对接 | 编排可视化 + 执行监控 |
| 2 | Dashboard前端 | 任务列表 + Agent日志 + 成本分析 |
| 3 | Agent经验积累 | 跨任务学习 |
| 4 | 智能任务拆解 | 基于历史数据优化 |
| 5 | 成本优化 | 模型降级策略 |
| 6 | K8s部署 | 生产部署方案 |
| 7 | L2 Embedding压缩 | 评估 embedding 服务方案后决定是否接入 |

---

## 十三、关键代码参考

### 13.1 服务启动

```go
// main.go — Coordinator服务启动
package main

import (
    "context"
    "github.com/vibe-go/vibe"
    "github.com/cloudwego/eino/compose"
    "cap/plugins/orchestrator"
    "cap/plugins/llmrouter"
    "cap/plugins/workermanager"
    "cap/gen/cap/v1/capv1connect"
)

func main() {
    // vibe-go初始化
    app := vibe.New(
        vibe.WithName("coordinator"),
        vibe.WithPort(3002),
        vibe.WithMiddleware(
            middleware.Recovery(),
            middleware.RequestID(),
            middleware.Logger(),
            middleware.JWT(jwtConfig),
            middleware.RateLimit(rateLimitConfig),
            middleware.Tracing(otelConfig),
        ),
        vibe.WithWebSocketHub(wsConfig),
    )
    
    // 插件初始化
    deps := &orchestrator.Dependencies{
        ModelRouter:    llmrouter.New(),
        WorkerManager:  workermanager.New(),
        TaskRepo:       ent.NewTaskClient(),
        WSHub:          app.WebSocketHub(),
    }
    
    // Eino编排图编译
    graph, err := orchestrator.BuildTaskGraph(deps)
    if err != nil {
        panic(err)
    }
    
    // connect-go注册
    handler := &TaskServiceHandler{Graph: graph, Deps: deps}
    path, h := capv1connect.NewTaskServiceHandler(handler)
    app.Mount(path, h)
    
    // MCP Server挂载
    mcpServer := orchestrator.NewMCPServer(graph, deps)
    app.Mount("/mcp", mcpServer)
    
    app.Run()
}
```

### 13.2 TaskServiceHandler实现

```go
package handler

type TaskServiceHandler struct {
    Graph *compose.Graph
    Deps  *orchestrator.Dependencies
}

func (h *TaskServiceHandler) SubmitTask(ctx context.Context, req *connect.Request[capv1.SubmitTaskRequest]) (*connect.Response[capv1.SubmitTaskResponse], error) {
    // 1. 创建任务记录（ent）
    task, err := h.Deps.TaskRepo.Create().
        SetID(ulid.Make().String()).
        SetGoal(req.Msg.Goal).
        SetRepositoryURL(req.Msg.RepositoryUrl).
        SetBaseBranch(req.Msg.BaseBranch).
        SetStatus("pending").
        SetPriority(int(req.Msg.Priority)).
        SetConstraints(req.Msg.Constraints).
        SetVerificationCriteria(req.Msg.VerificationCriteria).
        SetResultBranch(fmt.Sprintf("%s/agent/%s", req.Msg.BaseBranch, task.ID)).
        Save(ctx)
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, err)
    }
    
    // 2. 初始化上下文（Redis）
    h.Deps.ContextStore.Init(ctx, task.ID, req.Msg.Goal, req.Msg.Constraints)
    
    // 3. 触发编排（异步）
    go h.Graph.Invoke(ctx, orchestrator.TaskInput{
        TaskID: task.ID,
        Goal:   req.Msg.Goal,
    })
    
    return connect.NewResponse(&capv1.SubmitTaskResponse{
        TaskId:       task.ID,
        Status:       "pending",
        ResultBranch: task.ResultBranch,
        CreatedAt:    task.CreatedAt.Unix(),
    }), nil
}
```

### 13.3 前端API调用

```typescript
// 前端直接使用buf生成的TypeScript client
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { TaskService } from "./gen/cap/v1/task_connect";

const client = createPromiseClient(TaskService, 
    createConnectTransport({ baseUrl: "/api/v1" }));

// 提交任务
const { taskId } = await client.submitTask({
    goal: "实现用户登录功能",
    repositoryUrl: "https://github.com/acme/app",
    baseBranch: "main",
    constraints: ["使用JWT", "禁止明文存储密码"],
    verificationCriteria: ["所有测试通过", "覆盖率>80%"],
});

// WebSocket实时监听
const ws = new WebSocket("wss://api.host/api/v1/ws");
ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);
    if (msg.type === "task.status_changed") {
        updateProgress(msg.payload.progress);
    }
    if (msg.type === "agent.log") {
        appendLog(msg.payload.message);
    }
};
```

---

## 附录：术语表

| 术语 | 定义 |
|------|------|
| Task | 任务，用户提交的一个完整请求 |
| Subtask | 子任务，Task拆解后的执行单元 |
| Agent | 智能体，在沙箱内执行任务的程序实例 |
| Worker | 工作进程，承载Agent执行的运行时环境 |
| Sandbox | 沙箱，Agent执行的隔离环境（CubeSandbox/Docker） |
| Template | 模板，预定义的Agent角色配置（observer/executor等） |
| Orchestrator | 编排器，负责任务拆解和调度的核心组件 |
| Context | 上下文，Agent执行时可见的信息集合 |
| Artifact | 产出物，Agent执行产生的文件或报告 |
| Guardian | 审查员Agent，负责安全审查和高风险操作检测 |
| ReAct | 推理-行动循环，Agent的执行模式（Thought→Action→Observation） |
| MCP | Model Context Protocol，Agent与平台的通信协议 |
| Outbox | 发件箱模式，保证领域事件可靠投递的设计模式 |
| DAG | 有向无环图，子任务之间的依赖关系 |
| ULID | 排序友好的唯一标识符 |
| Eino | 字节跳动开源的Go Agent编排框架 |
| FlowGram | 字节开源的前端可视化编排引擎 |
| vibe-go | Go微服务框架（项目骨架） |
| connect-go | 支持HTTP/JSON+gRPC双协议的RPC框架 |
| ent | Go的Schema驱动ORM |
| buf | Protobuf管理工具和代码生成器 |
| CubeSandbox | 腾讯云开源的AI安全沙箱（MicroVM） |
