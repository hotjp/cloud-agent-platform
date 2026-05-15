# task_078

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_078.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P1: Test Coverage — Bring unit test coverage to 60%


## 需求 (requirements)

Current test coverage is estimated very low. Identify gaps with: go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out. Priority test files: (1) internal/service/task_service_test.go — test Submit/Get/List/Cancel/Decompose/Retry; (2) internal/domain/statemachine_test.go — state transitions; (3) internal/domain/errors_test.go — error codes; (4) internal/gateway/gateway_test.go — handler mapping; (5) internal/authz/authz_test.go; (6) internal/infra/persistence/task_repository_test.go; (7) internal/infra/cache/*; Use testify suite + gomock. Integration tests (require dockertest tag) for storage and outbox. Target: >60% line coverage across all packages



## 验收标准 (acceptance)


- go test ./... -coverprofile=coverage.out; overall coverage >60%; all tests pass; no race conditions




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