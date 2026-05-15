# task_008

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_008.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 五段式上下文

### 1. 目标

实现文件：`ent/schema/*.go`、`ent/migrations/`

产出类型：
- `ent/schema/task.go` — Task 表 Schema
- `ent/schema/subtask.go` — Subtask 表 Schema
- `ent/schema/audit_log.go` — AuditLog 表 Schema
- `ent/schema/outbox_event.go` — OutboxEvent 表 Schema
- 迁移 SQL 文件

### 2. 契约参考

```sql
-- outbox_events 表结构
CREATE TABLE outbox_events (
    id VARCHAR(64) PRIMARY KEY,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id VARCHAR(64) NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMP NOT NULL,
    idempotency_key VARCHAR(256) NOT NULL UNIQUE,
    version INT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### 3. 依赖接口

**上游提供（本 Task import 的）**：
- `entgo.io/ent` — ent ORM

**本层产出（其他 Task 会依赖的）**：
- `ent/schema/*` — 所有表 Schema

### 4. 约定

- 使用 `ent migrate` 进行迁移
- 迁移文件放在 `ent/migrations/`
- 必须创建索引：status, created_at, aggregate_id
- 迁移前检查当前版本

### 5. 验收标准

- 测试命令：`ent generate ./ent`
- 必须覆盖的 case：
  1. 所有 Schema 生成成功
  2. 迁移文件生成
  3. 索引正确
- Done 判定：`ent generate` 成功

## 描述

T11: 数据库迁移脚本 - tasks/subtasks/audit_logs/outbox_events四张表的迁移文件



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/T11:_数据库迁移脚本_-_tasks.py




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