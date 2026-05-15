# task_051

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_051.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/orchestrator/templates.go`

产出类型：
- `AgentTemplate` struct — Agent 模板定义
- `ToolPermission` struct — 工具权限
- `GetToolsForTemplate()` — 根据模板获取工具集函数

### 2. 契约参考

```go
// Agent 角色定义（来自 Cloud-Agent-Platform.md 3.3）
type AgentTemplate struct {
    ID            string   // observer/strategist/executor/guardian/tester/researcher
    Name          string
    Description   string
    DefaultModel string   // claude-sonnet / glm-5.1
    Capabilities  Capabilities
    Tools        []string // 可用工具列表
}

type Capabilities struct {
    Analysis int  // 1-5
    Coding   int  // 1-5
    Review   int  // 1-5
    Testing  int  // 1-5
    Research int  // 1-5
}

// 工具集分配（来自 Cloud-Agent-Platform.md 3.6）
var templateTools = map[string][]string{
    "observer":   {"read_file", "list_files", "search_code", "git_status", "ask_llm"},
    "strategist": {"read_file", "list_files", "search_code", "ask_llm"},
    "executor":   {"read_file", "write_file", "edit_file", "list_files", "search_code", "git_status", "git_diff", "git_commit", "execute_command"},
    "guardian":   {"read_file", "git_diff", "search_code"},
    "tester":     {"read_file", "write_file", "edit_file", "execute_command", "git_status", "git_diff"},
    "researcher": {"read_file", "list_files", "search_code", "execute_command", "ask_llm"},
}

// Prompt 模板路径
const promptPath = "plugins/orchestrator/prompts/%s.tmpl"
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/tools.Tool` — T47 工具接口

**本层产出（其他 Task 会依赖的）**：
- `plugins/orchestrator.AgentTemplate` — Agent 模板定义
- `plugins/orchestrator.GetToolsForTemplate()` — 工具集获取函数

### 4. 约定

- 模板 ID 使用 camelCase（observer, strategist, executor...）
- Prompt 模板文件放在 `plugins/orchestrator/prompts/` 目录
- 模板名称使用 PascalCase（Observer, Strategist, Executor...）
- 所有模板必须定义 Capabilities

### 5. 验收标准

- 测试命令：`go test ./plugins/orchestrator/... -v -run TestTemplates`
- 必须覆盖的 case：
  1. 所有 6 个模板定义正确
  2. 工具集分配符合表格
  3. Prompt 模板文件存在
- Done 判定：测试全部通过 + `go build ./plugins/orchestrator/...`

## 描述

T42a: Agent角色框架 - 角色接口定义，prompt模板结构，工具集分配表

## 需求 (requirements)

T42a: Agent角色框架 - 角色接口定义，prompt模板结构，工具集分配表

## 验收标准 (acceptance)

- 6个模板定义正确
- 工具集分配正确

## 交付物 (deliverables)

- plugins/orchestrator/templates.go — 模板定义
- plugins/orchestrator/prompts/*.tmpl — Prompt 模板

## 设计方案 (design)

1. 创建 `plugins/orchestrator/templates.go`
2. 定义 AgentTemplate 结构体
3. 定义 templateTools 映射
4. 创建 prompt 模板文件

## 验证证据（完成前必填）

- [ ] **实现证明**: 模板定义完成
- [ ] **测试验证**: `go test ./plugins/orchestrator/...` 通过
- [ ] **影响范围**: 所有 Agent 实现依赖此框架

### 测试步骤
1. `go test ./plugins/orchestrator/... -v -run TestTemplates`
2. `go build ./plugins/orchestrator/...`

### 验证结果
