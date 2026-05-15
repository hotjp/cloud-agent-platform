# task_007

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_007.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`internal/storage/postgres.go`、`ent/ent.go`

产出类型：
- `NewPostgresClient()` — PostgreSQL 客户端工厂函数
- `ent.Client` — ent ORM 客户端

### 2. 契约参考

```go
// PostgreSQL 配置
type PostgresConfig struct {
    DSN          string        `koanf:"dsn"`
    MaxOpen      int           `koanf:"max_open"`
    MaxIdle      int           `koanf:"max_idle"`
    MaxLifetime  time.Duration `koanf:"max_lifetime"`
}

// 连接池配置
MaxOpen: 25
MaxIdle: 10
MaxLifetime: 5m
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `github.com/jackc/pgx/v5` — pgx 驱动
- `entgo.io/ent` — ent ORM

**本层产出（其他 Task 会依赖的）**：
- `internal/storage.PostgresClient` — PostgreSQL 客户端

### 4. 约定

- L1-Storage 层使用 pgx/v5 和 ent
- 连接池参数可配置
- 必须支持事务管理（`BeginTx`/`Commit`/`Rollback`）
- 错误码：L1_001 ~ L1_199

### 5. 验收标准

- 测试命令：`go test ./internal/storage/... -v -run TestPostgres`
- 必须覆盖的 case：
  1. 连接成功
  2. 连接池参数生效
  3. 事务提交/回滚正常
- Done 判定：`go build ./...` 无错误

## 描述

T10: PostgreSQL连接 - pgx/v5连接池，ent生成器配置，事务管理封装



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T10:_PostgreSQL连接_-_.py




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