# task_047

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_047.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/workermanager/docker_backend.go`

产出类型：
- `DockerBackend` struct — Docker 沙箱后端实现
- 实现 `SandboxBackend` 接口

### 2. 契约参考

```go
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
        Resources:      container.Resources{
            Memory: 2 << 30,  // 2GB
            CPUQuota: 100000, // 1 CPU
        },
    }, nil, nil, "")
    if err != nil {
        return nil, err
    }
    return &Worker{ID: container.ID, Type: "docker", Workspace: "/workspace"}, nil
}
```

安全加固（来自 Cloud-Agent-Platform.md 11.3）：
- seccomp-worker.json：禁止 mount/umount2/pivot_root/open_by_handle_at/ptrace 等系统调用
- AppArmor：限制文件系统访问
- CapDrop: ALL：移除所有特权
- ReadonlyRootfs: true：根文件系统只读
- Tmpfs: /tmp：可写临时目录

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/workermanager.SandboxBackend` — T32a 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/workermanager.DockerBackend` — Docker 实现

### 4. 约定

- 必须实现 `SandboxBackend` 接口
- Docker 镜像：`cap-worker:latest`（需提前构建）
- seccomp 配置必须在容器启动时挂载
- 网络隔离：禁止访问内网，仅允许 LLM API 和 Git

### 5. 验收标准

- 测试命令：`go test ./plugins/workermanager/... -v -run TestDockerBackend`
- 必须覆盖的 case：
  1. Create 成功创建容器
  2. Destroy 成功停止并删除容器
  3. IsAvailable 检查 Docker Daemon 连接
- Done 判定：测试全部通过 + `go build ./plugins/workermanager/...`

## 描述

T32b: Docker沙箱实现 - DockerBackend，seccomp/AppArmor安全加固，2GB内存限制

## 需求 (requirements)

T32b: Docker沙箱实现 - DockerBackend，seccomp/AppArmor安全加固，2GB内存限制

## 验收标准 (acceptance)

- Docker 容器创建成功
- 安全加固配置正确

## 交付物 (deliverables)

- plugins/workermanager/docker_backend.go — DockerBackend 实现
- seccomp-worker.json — seccomp 配置

## 设计方案 (design)

1. 创建 `plugins/workermanager/docker_backend.go`
2. 实现 DockerBackend 结构体
3. 实现 Create/Destroy/IsAvailable
4. 创建 seccomp-worker.json

## 验证证据（完成前必填）

- [ ] **实现证明**: DockerBackend 实现完整
- [ ] **测试验证**: `go test ./plugins/workermanager/...` 通过
- [ ] **影响范围**: WorkerManager 降级依赖

### 测试步骤
1. `go test ./plugins/workermanager/... -v -run TestDockerBackend`
2. `go build ./plugins/workermanager/...`

### 验证结果
