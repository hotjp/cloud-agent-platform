# task_056

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_056.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/tools/command_tool.go`、`plugins/tools/llm_tool.go`

产出类型：
- `ExecuteCommandTool` struct — 命令执行
- `AskLLMTool` struct — LLM 提问

### 2. 契约参考

```go
// ExecuteCommandTool（来自 Cloud-Agent-Platform.md 7.2）
type ExecuteCommandTool struct {
    Workspace    string
    AllowedHosts []string  // 网络白名单
    MaxTimeout   time.Duration
    Forbidden    []string   // 禁止的命令模式
}

// 禁止模式
var forbidden = []string{
    "sudo", "su ", "rm -rf /", "chmod +s",
    ">/dev/sda", ":(){:|:&};:",
}

// ask_llm（来自 Cloud-Agent-Platform.md 7.2）
// 功能：用于技术研究，不计入任务 token 预算
type AskLLMTool struct {
    llm LLMCaller
}
```

安全约束：
- execute_command: 禁止 sudo/su/rm -rf 等危险命令
- execute_command: curl 只能访问白名单域名
- execute_command: 最长 60 秒超时
- ask_llm: 不计入任务 token 预算

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `github.com/redis/go-redis/v9` — Redis（用于 token 计数）
- `plugins/llmrouter.LLMCaller` — LLM 调用接口

**本层产出（其他 Task 会依赖的）**：
- `plugins/tools.ExecuteCommandTool`
- `plugins/tools.AskLLMTool`

### 4. 约定

- 命令执行必须验证所有禁止模式
- 超时后强制杀死进程
- ask_llm 调用单独统计 token
- 错误码：L4_602

### 5. 验收标准

- 测试命令：`go test ./plugins/tools/... -v -run TestCommandTool`
- 必须覆盖的 case：
  1. 正常执行允许的命令
  2. 禁止命令被拦截（sudo, rm -rf /）
  3. 超时命令被杀死
  4. ask_llm 调用成功
- Done 判定：测试全部通过 + `go build ./plugins/tools/...`

## 描述

T47c: 命令和LLM工具 - execute_command(白名单+超时)和ask_llm工具实现

## 需求 (requirements)

T47c: 命令和LLM工具 - execute_command(白名单+超时)和ask_llm工具实现

## 验收标准 (acceptance)

- 命令执行安全约束生效
- ask_llm 实现正确

## 交付物 (deliverables)

- plugins/tools/command_tool.go — ExecuteCommandTool
- plugins/tools/llm_tool.go — AskLLMTool
- plugins/tools/command_tool_test.go — 测试

## 设计方案 (design)

1. 实现 ExecuteCommandTool（含禁止模式检测）
2. 实现 AskLLMTool
3. 编写安全测试

## 验证证据（完成前必填）

- [ ] **实现证明**: 命令和LLM工具实现完成
- [ ] **测试验证**: `go test ./plugins/tools/...` 通过
- [ ] **影响范围**: Researcher Agent 调用

### 测试步骤
1. `go test ./plugins/tools/... -v -run TestCommandTool`
2. `go build ./plugins/tools/...`

### 验证结果
