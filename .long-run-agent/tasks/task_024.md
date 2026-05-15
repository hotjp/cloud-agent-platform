# task_024

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_024.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/storage/context_store.go`

产出类型：
- `ContextStore` — 上下文存储接口
- `RedisContextStore` — Redis 实现

### 2. 契约参考

```go
type ContextStore interface {
    Get(ctx context.Context, taskID string) (*TaskContext, error)
    Update(ctx context.Context, taskID string, updater func(*TaskContext) error) error
    Archive(ctx context.Context, taskID string) error
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `internal/domain.TaskContext` — T44 定义
- `internal/storage.RedisClient` — T12 定义

**本层产出（其他 Task 会依赖的）**：
- `internal/storage.ContextStore` — 上下文存储

### 4. 约定

- L1-Storage 层实现
- TTL: 2-24 小时
- 看门狗自动续期

### 5. 验收标准

- 测试命令：`go test ./internal/storage/... -v -run TestContextStore`
- 必须覆盖的 case：
  1. Get/Update 正常
  2. 分布式锁正常
- Done 判定：测试通过 + `go build ./...`

## 描述

T45: Redis热层 - 分布式锁(Redlock)，看门狗自动续期，热上下文读写接口



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T45:_Redis热层_-_分布式锁(.py




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