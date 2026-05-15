# task_083

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_083.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P2: Fault预案 — Redis故障/LLM限流/Worker泄漏 detection + graceful degradation


## 需求 (requirements)

Implement graceful degradation for infrastructure failures: (1) Redis故障: Cache fallback to in-memory (simple map with TTL); Context reads fail → read from PostgreSQL warm layer; Outbox forwarder fails → retry with exponential backoff (already in poller but needs circuit breaker); (2) LLM限流: sentinel-go adaptive降级 (Claude Sonnet→GLM-4 when 429); Circuit breaker per model; (3) Worker泄漏: WorkerManager health check — destroy workers idle > 30min; Max worker lifetime config; Memory usage monitoring and auto-destroy > 1.5GB; (4) Circuit breaker pattern: plugins/llmrouter/circuitbreaker.go already exists but not wired to LLM calls. Wire it. Add circuit breaker for Redis get/set



## 验收标准 (acceptance)


- Redis down: system still accepts tasks (from PG cache); LLM rate limited: automatic model downgrade; Worker leak: workers destroyed after 30min idle; Circuit breakers trip and recover




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