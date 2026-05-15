# task_081

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_081.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P2: End-to-end smoke test — Submit task → single Agent execute → return result + Git push


## 需求 (requirements)

Create an E2E smoke test that validates the full pipeline: (1) Start docker-compose (postgres/redis/minio/server); (2) Submit a simple task via curl/connect-go client (e.g., 'add README line'); (3) Verify task goes through pending→running→completed states; (4) Verify WebSocket events received (task.status_changed, agent.log); (5) Verify git push to result branch succeeds; (6) Verify GET /api/v1/tasks/:id returns correct state; Use a test GitHub repo. Put test in tests/e2e/smoke_test.go with //+build e2e tag



## 验收标准 (acceptance)


- smoke test passes in CI against docker-compose stack; all 5 phases of the pipeline verified; git result branch contains expected change




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