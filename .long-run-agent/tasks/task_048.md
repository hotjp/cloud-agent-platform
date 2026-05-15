# task_048

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_048.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/workermanager/cubesandbox_backend.go`

产出类型：
- `CubeSandboxBackend` struct — CubeSandbox 沙箱后端实现
- 实现 `SandboxBackend` 接口

### 2. 契约参考

```go
type CubeSandboxBackend struct {
    client *cubesandbox.Client
}

func (b *CubeSandboxBackend) Create(ctx context.Context, spec WorkerSpec) (*Worker, error) {
    sandbox, err := b.client.CreateSandbox(ctx, &cubesandbox.SandboxConfig{
        CPU:    1,
        Memory: 2 * 1024 * 1024 * 1024, // 2GB
        Network: cubesandbox.NetworkConfig{
            AllowHosts: []string{
                "api.anthropic.com",  // Claude
                "open.bigmodel.cn",   // GLM
                "github.com",         // Git
            },
        },
        ReadOnlyRoot: true,
        Timeout:      spec.Timeout,
    })
    if err != nil {
        return nil, err
    }
    return &Worker{
        ID:        sandbox.ID,
        Type:      "cubesandbox",
        Workspace: "/workspace",
    }, nil
}

func (b *CubeSandboxBackend) IsAvailable(ctx context.Context) bool {
    return b.client != nil && b.client.Ping(ctx) == nil
}
```

CubeSandbox 规格（来自 Cloud-Agent-Platform.md 9.1）：
- 启动时间：<60ms
- 内存占用：<5MB（相比 Docker 2GB）
- 硬件级隔离：MicroVM
- 网络白名单：仅 LLM API + Git

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/workermanager.SandboxBackend` — T32a 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/workermanager.CubeSandboxBackend` — CubeSandbox 实现

### 4. 约定

- 必须实现 `SandboxBackend` 接口
- 网络白名单必须包含：api.anthropic.com, open.bigmodel.cn, github.com
- IsAvailable 检查客户端连接和 Ping
- CubeSandbox 不可用时 WorkerManager 自动降级到 Docker

### 5. 验收标准

- 测试命令：`go test ./plugins/workermanager/... -v -run TestCubeSandboxBackend`
- 必须覆盖的 case：
  1. Create 成功创建沙箱
  2. Destroy 成功销毁沙箱
  3. IsAvailable 检查连接状态
  4. 不可用时返回 false
- Done 判定：测试全部通过 + `go build ./plugins/workermanager/...`

## 描述

T32c: CubeSandbox实现 - CubeSandboxBackend，MicroVM硬件隔离，<60ms启动

## 需求 (requirements)

T32c: CubeSandbox实现 - CubeSandboxBackend，MicroVM硬件隔离，<60ms启动

## 验收标准 (acceptance)

- CubeSandbox 创建成功
- 网络白名单配置正确

## 交付物 (deliverables)

- plugins/workermanager/cubesandbox_backend.go — CubeSandboxBackend 实现

## 设计方案 (design)

1. 创建 `plugins/workermanager/cubesandbox_backend.go`
2. 实现 CubeSandboxBackend 结构体
3. 实现 Create/Destroy/IsAvailable
4. 配置网络白名单

## 验证证据（完成前必填）

- [ ] **实现证明**: CubeSandboxBackend 实现完整
- [ ] **测试验证**: `go test ./plugins/workermanager/...` 通过
- [ ] **影响范围**: WorkerManager 主后端选项

### 测试步骤
1. `go test ./plugins/workermanager/... -v -run TestCubeSandboxBackend`
2. `go build ./plugins/workermanager/...`

### 验证结果
