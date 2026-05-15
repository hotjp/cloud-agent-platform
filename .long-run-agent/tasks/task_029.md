# task_029

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_029.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`plugins/workermanager/pool.go`

产出类型：
- `WorkerPool` struct — Worker 池

### 2. 契约参考

```go
type WorkerPool struct {
    backend SandboxBackend // T32a
    warmup   int         // 预热数量
    maxSize  int         // 最大容量
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `plugins/workermanager.SandboxBackend` — T32a 定义

**本层产出（其他 Task 会依赖的）**：
- `plugins/workermanager.WorkerPool` — Worker 池

### 4. 约定

- Plugin 层实现
- 预热/扩缩容/健康检查
- 复用 T32 SandboxBackend 接口

### 5. 验收标准

- 测试命令：`go test ./plugins/workermanager/... -v -run TestPool`
- 必须覆盖的 case：
  1. 预热正常
  2. 扩缩容正常
- Done 判定：测试通过 + `go build ./...`

## 描述

T60: Worker池生产化 - 预热/扩缩容/健康检查，复用T32 SandboxBackend接口



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T60:_Worker池生产化_-_预热.py




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