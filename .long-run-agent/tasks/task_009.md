# task_009

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_009.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/storage/redis.go`

产出类型：
- `NewRedisClient()` — Redis 客户端工厂函数
- `RedisCache` — 缓存层封装
- `DistributedLock` — 分布式锁接口

### 2. 契约参考

```go
// Redis 配置
type RedisConfig struct {
    Addr     string `koanf:"addr"`
    Password string `koanf:"password"`
    DB       int    `koanf:"db"`
}

// 缓存接口
type Cache interface {
    Get(ctx context.Context, key string) (string, error)
    Set(ctx context.Context, key string, value string, ttl time.Duration) error
    Del(ctx context.Context, key string) error
}

// 分布式锁接口
type DistributedLock interface {
    Obtain(ctx context.Context, key string, ttl time.Duration) (*Lock, error)
}
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `github.com/redis/go-redis/v9` — Redis 客户端

**本层产出（其他 Task 会依赖的）**：
- `internal/storage.RedisClient` — Redis 客户端
- `internal/storage.Cache` — 缓存接口

### 4. 约定

- 使用 go-redis/v9
- Key 格式：`category:entity:qualifier`
- 禁止使用 `KEYS *`，必须用 `SCAN`
- 所有写操作必须设置 TTL
- 错误码：L1_001 ~ L1_199

### 5. 验收标准

- 测试命令：`go test ./internal/storage/... -v -run TestRedis`
- 必须覆盖的 case：
  1. 连接成功
  2. Get/Set/Del 正常
  3. TTL 生效
- Done 判定：`go build ./...` 无错误

## 描述

T12: Redis客户端 - go-redis/v9连接，缓存层封装，分布式锁基础接口



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T12:_Redis客户端_-_go-r.py




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