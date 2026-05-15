# task_054

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_054.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/tools/file_tool.go`

产出类型：
- `ReadFileTool` struct — 文件读取
- `WriteFileTool` struct — 文件写入
- `EditFileTool` struct — 文件编辑
- `ListFilesTool` struct — 目录列表
- `SearchCodeTool` struct — 代码搜索

### 2. 契约参考

```go
// 文件工具 InputSchema（来自 Cloud-Agent-Platform.md 7.2）
type ReadFileTool struct {
    Workspace string
}

type WriteFileTool struct {
    Workspace string
}

type EditFileTool struct {
    Workspace string
}

type ListFilesTool struct {
    Workspace string
}

type SearchCodeTool struct {
    Workspace string
}
```

安全约束：
- read_file: 禁止访问 /workspace 外
- write_file: 单次最多 10MB
- edit_file: 精确匹配 oldString 替换
- search_code: 支持正则，限制结果数量

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- 无上游依赖

**本层产出（其他 Task 会依赖的）**：
- `plugins/tools.ReadFileTool`
- `plugins/tools.WriteFileTool`
- `plugins/tools.EditFileTool`
- `plugins/tools.ListFilesTool`
- `plugins/tools.SearchCodeTool`

### 4. 约定

- Workspace 根目录：`/workspace`
- 路径遍历攻击防护：`filepath.Join` 后检查 `HasPrefix`
- 日志记录：path, bytes, duration
- 错误码：L4_602

### 5. 验收标准

- 测试命令：`go test ./plugins/tools/... -v -run TestFileTool`
- 必须覆盖的 case：
  1. 正常读取/写入/编辑文件
  2. 路径遍历被拦截
  3. 文件大小限制生效
- Done 判定：测试全部通过 + `go build ./plugins/tools/...`

## 描述

T47a: 文件工具集 - read_file/write_file/edit_file/list_files/search_code 5个工具实现

## 需求 (requirements)

T47a: 文件工具集 - read_file/write_file/edit_file/list_files/search_code 5个工具实现

## 验收标准 (acceptance)

- 5个文件工具实现正确
- 安全约束测试通过

## 交付物 (deliverables)

- plugins/tools/file_tool.go — 5个文件工具
- plugins/tools/file_tool_test.go — 测试

## 设计方案 (design)

1. 实现 ReadFileTool
2. 实现 WriteFileTool
3. 实现 EditFileTool
4. 实现 ListFilesTool
5. 实现 SearchCodeTool

## 验证证据（完成前必填）

- [ ] **实现证明**: 5个文件工具实现完成
- [ ] **测试验证**: `go test ./plugins/tools/...` 通过
- [ ] **影响范围**: ReAct Agent 调用

### 测试步骤
1. `go test ./plugins/tools/... -v -run TestFileTool`
2. `go build ./plugins/tools/...`

### 验证结果
