# task_018

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_018.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/gitclient/client.go`

产出类型：
- `GitClient` struct — Git 客户端

### 2. 契约参考

```go
type GitClient struct {
    repo *git.Repository
}

func (c *GitClient) Clone(ctx context.Context, url, dir string) error
func (c *GitClient) Commit(ctx context.Context, message string) (string, error)
func (c *GitClient) Push(ctx context.Context) error
```

安全约束：
- 禁止 push 到 main/master
- 禁止 force push

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `github.com/go-git/go-git/v5` — go-git

**本层产出（其他 Task 会依赖的）**：
- `plugins/gitclient.GitClient` — Git 客户端

### 4. 约定

- 使用 go-git 纯 Go 实现
- 禁止 push 到 main/master
- 错误码：L4_602

### 5. 验收标准

- 测试命令：`go test ./plugins/gitclient/... -v`
- 必须覆盖的 case：
  1. Clone 成功
  2. Commit 成功
  3. push 到 main 被拦截
- Done 判定：测试通过 + `go build ./...`

## 描述

T33: Git集成 - go-git实现clone/commit/push，禁止push到main/master约束



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T33:_Git集成_-_go-git实.py




## 设计方案 (design)

快速任务


## 验证证据（完成前必填）

<!-- 标记完成前，请提供以下证据： -->

- [ ] **实现证明**: 简要说明如何实现
- [ ] **测试验证**: 如何验证功能正常（测试步骤/截图/命令输出）
- [ ] **影响范围**: 是否影响其他功能

### 测试步骤
1. 
2. 
3. 

### 验证结果
<!-- 粘贴验证截图、命令输出或测试结果 -->