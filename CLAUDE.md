# CLAUDE.md — Cloud Agent Platform 开发约束

> 这是 Claude Code 的项目指令文件。每次 Claude Code 在此项目中工作时必须遵守以下所有规则。

## 项目信息

- **项目名**: Cloud Agent Platform
- **路径**: `/Users/kingj/code/cloud-agent-platform`
- **技术栈**: TypeScript (Node.js 22) + Fastify + Redis 7 + PostgreSQL 15 + Docker
- **包管理**: pnpm workspace (monorepo)
- **定位**: 云端多 Agent 编排平台

## 技术方案文件（必读）

开始任何开发前，先阅读以下文件理解架构：

- `~/Desktop/Cloud-Agent-Platform-独立项目技术方案.md` — 完整架构设计
- `~/Desktop/Cloud-Agent-Platform-接口契约.md` — MCP + REST + WS + Git 协议

## 代码规范

### TypeScript

- 严格模式：`strict: true`
- 禁止 `as any`，除非有注释说明原因
- 所有公共函数必须有 JSDoc 注释
- 导入顺序：node 内置 → 第三方 → 项目内部，每组空行分隔
- 错误处理：禁止空 catch，至少 `console.error` 或 `logger.error`
- 异步操作：必须处理错误，不允许未捕获的 Promise rejection

### 文件组织

- 一个文件一个主要导出（class / function / type）
- 文件名 kebab-case：`task-decomposer.ts`
- 目录名 kebab-case：`session-store/`
- 类型定义放在 `types.ts` 或 `*.types.ts`
- 测试文件跟源文件同名加 `.test.ts`，放在同目录或 `__tests__/`

### 命名

- 变量/函数：camelCase
- 类/接口/类型：PascalCase
- 常量：UPPER_SNAKE_CASE
- 枚举值：UPPER_SNAKE_CASE
- 数据库列：snake_case
- Redis key：`category:entity:qualifier` 格式
- API 路径：kebab-case (`/agent-templates`)
- Git 分支：`category/description`

## 硬约束（违反即 bug）

### 1. 禁止 mock 数据绕过功能

所有 API 端点必须有真实的业务逻辑实现。不允许返回硬编码的假数据来绕过功能。

### 2. 类型定义即契约

接口类型必须与实际数据源完全一致。类型跟数据对不上 = bug，不是"以后再对齐"。

### 3. 前后端/模块间对接必须先列对照表

任何跨模块对接，必须先确认双方的字段名、类型、结构逐个对齐，不能靠猜。

### 4. 最小改动原则

改最小的范围解决问题。不顺便重构、不顺便加功能、不改无关代码。

### 5. 性能是硬约束

- 不确定资源影响时，宁可保守
- 循环内禁止同步 I/O
- 大数据处理必须用流式或分页
- 数据库查询必须有合理索引
- Redis 操作禁止 `KEYS *`，用 `SCAN`

### 6. 禁止假设返回结构

`result.data.xxx` 这种"假设返回结构"的写法，必须先确认 API 实际返回结构再使用。

### 7. 错误必须可追溯

所有错误必须包含上下文信息（什么操作失败、输入是什么、期望是什么）。不允许只抛一个空 Error。

## 架构规则

### 依赖方向

```
API层 → Service层 → Store层 → 基础设施层
```

- 上层可以依赖下层
- 下层禁止依赖上层
- 同层之间通过接口通信，不直接引用实现

### Session Store 使用

- 读写 Redis 必须通过 `packages/session-store/` 封装的客户端
- 禁止直接使用 `ioredis`，除非在 session-store 包内部
- Key 必须遵循技术方案中的 key 设计规范
- 所有写操作必须设置 TTL（防止内存泄漏）

### Worker 沙盒

- Worker 代码只能在 `worker/` 目录下
- Worker 内部禁止直接访问 Redis，通过 HTTP API 与平台通信
- Worker 容器必须是无状态的（挂了可以随时重建）

### MCP Server

- MCP Tool 定义在 `packages/mcp-server/src/tools.ts`
- 每个 Tool 对应接口契约文档中的一个接口
- Tool 的 `inputSchema` 必须与接口契约完全一致
- MCP Server 通过 REST API 与平台通信，不直接访问数据库或 Redis

## Git 规范

### Commit 格式

```
type(scope): description

[optional body]
```

type:
- `feat`: 新功能
- `fix`: 修复
- `refactor`: 重构（不改变行为）
- `test`: 测试
- `docs`: 文档
- `chore`: 构建/配置

scope: 模块名（coordinator / session-store / worker / api / mcp-server / core）

示例：
```
feat(coordinator): 添加任务拆解引擎
fix(session-store): 修复上下文压缩时丢失约束信息
test(api): 添加任务提交 API 的集成测试
```

### 分支

- `main` — 稳定分支，只接受 PR
- `dev` — 开发分支
- `feat/xxx` — 功能分支
- `fix/xxx` — 修复分支

## 测试规范

- 每个模块必须有单元测试
- API 端点必须有集成测试
- 测试文件路径与源文件一致
- 测试覆盖率目标：核心逻辑 > 80%
- 测试命名：`describe('模块名') → it('should 做什么')`

## 安全

- 不在代码中硬编码密钥、token、密码
- 敏感配置通过环境变量读取
- `.env.example` 提供模板，`.env` 在 `.gitignore` 中
- 用户输入必须校验和消毒
- SQL 查询必须用参数化，禁止字符串拼接

## 日志

- 使用结构化日志（JSON 格式）
- 级别：`error` > `warn` > `info` > `debug`
- 生产环境默认 `info`
- 禁止在日志中输出敏感信息（token、密码、密钥）
- Agent 执行日志保留到审计表

## 禁止事项清单

- ❌ mock 数据绕过功能
- ❌ `as any` 绕过类型检查（除非注释说明原因）
- ❌ 假设 API 返回结构
- ❌ 顺便重构/加功能
- ❌ 循环内同步 I/O
- ❌ `KEYS *` 扫描 Redis
- ❌ 硬编码密钥/Token
- ❌ 字符串拼接 SQL
- ❌ 空 catch 块
- ❌ 未处理的 Promise rejection
- ❌ 跳过测试说"以后再补"
