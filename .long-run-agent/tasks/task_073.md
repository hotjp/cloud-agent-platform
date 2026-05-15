# task_073

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_073.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P0: gateway readyzHandler — Implement DB+Redis connectivity check


## 需求 (requirements)

Fix internal/gateway/gateway.go readyzHandler (currently TODO: check database and Redis connectivity). Must: (1) Accept *storage.Storage and *redis.Client as constructor params; (2) In readyzHandler: ping PostgreSQL (SELECT 1) and Redis (PING) with 3s timeout; (3) Return 503 Service Unavailable if either fails; (4) Return 200 with {"status":"ok"} when both healthy; (5) Wire the readyzHandler into the gateway mux



## 验收标准 (acceptance)


- GET /readyz returns 200 when PG+Redis are up

- 503 when either is down; go build passes




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

<!-- 在此填写架构设计、技术选型、实现思路 -->


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