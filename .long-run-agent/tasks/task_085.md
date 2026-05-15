# task_085

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_085.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P2: docker-compose.prod.yml — Production-grade deployment configuration


## 需求 (requirements)

Review and finalize docker-compose.prod.yml: (1) Add health checks for all services with proper intervals/timeouts; (2) Configure resource limits (memory/CPU) for server container; (3) Add restart policies: always for all services; (4) Configure log rotation (max-size 100m, max-files 3); (5) Add nginx sidecar for TLS termination + rate limiting; (6) Environment variable validation at startup (APP_SERVER_PORT, APP_DATABASE_DSN etc.); (7) Volume mounts for persistent data (postgres, redis, minio); (8) Add deploy/healthcheck for each service; (9) Create .env.example with all required env vars; (10) Add init container for DB migration (ent migrate); (11) Production image should use non-root user



## 验收标准 (acceptance)


- docker-compose -f docker-compose.prod.yml config passes; All services have health checks; Server container has resource limits; .env.example covers all required vars; Image runs as non-root




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