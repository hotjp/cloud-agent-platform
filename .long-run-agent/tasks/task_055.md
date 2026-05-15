# task_055

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_055.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/tools/git_tool.go`

产出类型：
- `GitStatusTool` struct — Git 状态
- `GitDiffTool` struct — Git 差异
- `GitCommitTool` struct — Git 提交
- `GitPushTool` struct — Git 推送

### 2. 契约参考

```go
// Git 工具（来自 Cloud-Agent-Platform.md 7.2）
type GitStatusTool struct {
    repoPath string
}

type GitDiffTool struct {
    repoPath string
}

type GitCommitTool struct {
    repoPath string
    // 安全约束：禁止 push 到 main/master
}

type GitPushTool struct {
    repoPath string
    // 安全约束：禁止 force push（除非显式允许）
}
```

安全约束：
- git_commit: 禁止 push 到 main/master 分支
- git_push: 禁止 force push（除非显式 allow）
- 禁止操作 /workspace 外的仓库

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `github.com/go-git/go-git/v5` — go-git 库

**本层产出（其他 Task 会依赖的）**：
- `plugins/tools.GitStatusTool`
- `plugins/tools.GitDiffTool`
- `plugins/tools.GitCommitTool`
- `plugins/tools.GitPushTool`

### 4. 约定

- 使用 go-git 纯 Go 实现（Worker 不需要 git 二进制）
- 仓库路径必须在 /workspace 内
- 错误日志包含操作的分支名
- 错误码：L4_602

### 5. 验收标准

- 测试命令：`go test ./plugins/tools/... -v -run TestGitTool`
- 必须覆盖的 case：
  1. git_status 返回当前状态
  2. git_commit 成功提交
  3. git_commit 禁止推送到 main（被拦截）
  4. git_push 禁止 force push（被拦截）
- Done 判定：测试全部通过 + `go build ./plugins/tools/...`

## 描述

T47b: Git命令工具 - git_status/git_diff/git_commit/git_push实现，安全约束

## 需求 (requirements)

T47b: Git命令工具 - git_status/git_diff/git_commit/git_push实现，安全约束

## 验收标准 (acceptance)

- 4个Git工具实现正确
- 安全约束测试通过

## 交付物 (deliverables)

- plugins/tools/git_tool.go — 4个Git工具
- plugins/tools/git_tool_test.go — 测试

## 设计方案 (design)

1. 实现 GitStatusTool
2. 实现 GitDiffTool
3. 实现 GitCommitTool（含 main/master 拦截）
4. 实现 GitPushTool（含 force push 拦截）

## 验证证据（完成前必填）

- [ ] **实现证明**: 4个Git工具实现完成
- [ ] **测试验证**: `go test ./plugins/tools/...` 通过
- [ ] **影响范围**: Executor Agent 调用

### 测试步骤
1. `go test ./plugins/tools/... -v -run TestGitTool`
2. `go build ./plugins/tools/...`

### 验证结果
