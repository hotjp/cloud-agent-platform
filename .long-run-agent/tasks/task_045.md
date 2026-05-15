# task_045

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_045.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/tools/file_tool.go`

产出类型：
- `ReadFileTool` struct — 文件读取工具
- `WriteFileTool` struct — 文件写入工具
- 实现 `Tool` 接口

### 2. 契约参考

```go
// Tool 接口（来自 Cloud-Agent-Platform.md 7.1）
type Tool interface {
    Info() ToolInfo
    Run(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
}

type ToolInfo struct {
    Name        string
    Description string
    InputSchema map[string]interface{} // JSON Schema
}

// ReadFileTool InputSchema
{
    "type": "object",
    "properties": {
        "path": {"type": "string", "description": "文件路径（相对workspace）"},
        "offset": {"type": "integer", "description": "起始行号（可选）"},
        "limit": {"type": "integer", "description": "读取行数（默认100）"}
    },
    "required": ["path"]
}

// WriteFileTool InputSchema
{
    "type": "object",
    "properties": {
        "path": {"type": "string", "description": "文件路径（相对workspace）"},
        "content": {"type": "string", "description": "文件内容"}
    },
    "required": ["path", "content"]
}
```

安全约束：
- 文件路径必须在 `/workspace` 内（防止目录遍历）
- 单文件最大 10MB
- 禁止访问 `/workspace` 外的路径

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- 无上游依赖

**本层产出（其他 Task 会依赖的）**：
- `plugins/tools.ReadFileTool` — 读取工具
- `plugins/tools.WriteFileTool` — 写入工具

### 4. 约定

- Workspace 根目录：`/workspace`
- 文件操作必须验证路径前缀
- 禁止 `..` 路径遍历（`filepath.Join` 后检查）
- 日志记录文件操作（path, bytes read/written）
- 错误码：L4_602（工具调用失败）

### 5. 验收标准

- 测试命令：`go test ./plugins/tools/... -v -run TestFileTool`
- 必须覆盖的 case：
  1. 正常读取文件
  2. 路径遍历攻击（`../../etc/passwd`）被拦截
  3. 读取超过 10MB 的文件返回错误
  4. 正常写入文件
  5. 写入到 `/workspace` 外被拦截
- Done 判定：测试全部通过 + `go build ./plugins/tools/...`

## 描述

T31b: 基础工具集成 - read_file/write_file工具实现，Tool接口适配

## 需求 (requirements)

T31b: 基础工具集成 - read_file/write_file工具实现，Tool接口适配

## 验收标准 (acceptance)

- 读写文件测试通过
- 安全约束测试通过

## 交付物 (deliverables)

- plugins/tools/file_tool.go — ReadFileTool + WriteFileTool
- plugins/tools/file_tool_test.go — 测试

## 设计方案 (design)

1. 创建 `plugins/tools/file_tool.go`
2. 实现 ReadFileTool（Tool 接口）
3. 实现路径安全检查
4. 实现 WriteFileTool
5. 编写安全测试

## 验证证据（完成前必填）

- [ ] **实现证明**: 文件工具实现完成
- [ ] **测试验证**: `go test ./plugins/tools/...` 通过
- [ ] **影响范围**: ReAct Agent 调用

### 测试步骤
1. `go test ./plugins/tools/... -v -run TestFileTool`
2. `go build ./plugins/tools/...`

### 验证结果
