# task_046

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_046.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/workermanager/manager.go`、`plugins/workermanager/sandbox.go`

产出类型：
- `SandboxBackend` interface — 沙箱后端接口
- `WorkerManager` struct — Worker 管理者
- `WorkerSpec` struct — Worker 规格

### 2. 契约参考

```go
// SandboxBackend — 沙箱后端接口（两种实现共用）
type SandboxBackend interface {
    Create(ctx context.Context, spec WorkerSpec) (*Worker, error)
    Destroy(ctx context.Context, worker *Worker) error
    IsAvailable(ctx context.Context) bool
}

// WorkerManager — 按配置选择后端，支持自动降级
type WorkerManager struct {
    primary    SandboxBackend  // 配置指定的主后端
    fallback   SandboxBackend  // Docker 降级后端（永远可用）
    pool       *WorkerPool
}

type Worker struct {
    ID        string
    Type      string  // "docker" 或 "cubesandbox"
    Workspace string  // 工作目录
    Spec      WorkerSpec
}

type WorkerSpec struct {
    TaskID      string
    MemoryLimit int64  // 字节，默认 2GB
    Timeout     time.Duration
    EnvVars    map[string]string
}

// 配置切换
sandbox:
  backend: "docker"       // "docker" | "cubesandbox"
  fallback_to_docker: true  // cubesandbox 失败时自动降级
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- 无上游依赖

**本层产出（其他 Task 会依赖的）**：
- `plugins/workermanager.SandboxBackend` — 沙箱后端接口
- `plugins/workermanager.WorkerManager` — Worker 管理器

### 4. 约定

- 插件层实现，接口由 T32b 和 T32c 实现
- `IsAvailable()` 返回 false 时自动降级
- Worker 必须设置超时，超时后强制 Destroy
- `Create` 失败时记录日志并尝试 fallback

### 5. 验收标准

- 测试命令：`go test ./plugins/workermanager/... -v -run TestWorkerManager`
- 必须覆盖的 case：
  1. 配置 backend=docker 时使用 DockerBackend
  2. 配置 backend=cubesandbox 且不可用时自动降级到 Docker
  3. Worker 正确获取和释放
- Done 判定：测试全部通过 + `go build ./plugins/workermanager/...`

## 描述

T32a: SandboxBackend接口 - WorkerManager，按配置选择后端，支持自动降级

## 需求 (requirements)

T32a: SandboxBackend接口 - WorkerManager，按配置选择后端，支持自动降级

## 验收标准 (acceptance)

- 按配置选择后端
- 自动降级测试通过

## 交付物 (deliverables)

- plugins/workermanager/sandbox.go — SandboxBackend 接口
- plugins/workermanager/manager.go — WorkerManager 实现

## 设计方案 (design)

1. 创建 `plugins/workermanager/` 目录
2. 定义 SandboxBackend 接口
3. 实现 WorkerManager
4. 实现配置加载和后端选择

## 验证证据（完成前必填）

- [ ] **实现证明**: WorkerManager 实现完整
- [ ] **测试验证**: `go test ./plugins/workermanager/...` 通过
- [ ] **影响范围**: T30b 和 T31a 依赖此接口

### 测试步骤
1. `go test ./plugins/workermanager/... -v`
2. `go build ./plugins/workermanager/...`

### 验证结果
