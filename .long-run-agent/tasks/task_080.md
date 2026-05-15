# task_080

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_080.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P1: L3-Authz wiring — Full JWT verification + RBAC + sentinel-go in request path


## 需求 (requirements)

The gateway Auth middleware currently only decrypts JWT (no verification). The L3-Authz layer (internal/authz/authz.go) is defined but not in the request path. Required: (1) Auth middleware should call L3-Authz service for full JWT verification before letting request through; (2) Wire sentinel-go rate limiter: global 100qps + per-client-id 10qps + per-task 5 concurrent; (3) Implement rate limit response (HTTP 429); (4) Wire authz cache (Redis TTL 5min) for auth decisions; (5) Add user context extraction from verified JWT; (6) Wire API Key auth alongside JWT



## 验收标准 (acceptance)


- Invalid JWT returns 401; valid JWT passes; rate limit exceeded returns 429; go build passes; authz tests pass




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