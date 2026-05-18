# task_133

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_133.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

T-E2E-05: 3个用户同时提交task


## 需求 (requirements)

模拟3个不同用户(不同JWT token)同时提交task



## 验收标准 (acceptance)


- 所有task正常执行

- 无资源冲突

- Git容器按项目复用




## 交付物 (deliverables)

- `test/e2e/concurrent_users_test.go` - 3个用户并发提交任务的E2E测试

## 设计方案 (design)

使用 httptest + mock handler 模拟平台API，3个不同用户使用不同JWT token并发提交任务：
- `concurrentUserTestHandler` 跟踪用户-任务关系，实现任务隔离
- 使用 sync.WaitGroup 实现真正的并发提交
- 验证用户只能看到自己的任务（通过 token 过滤）

## 验证证据（完成前必填）

- [x] **实现证明**: 创建了 `test/e2e/concurrent_users_test.go`，包含3个测试函数
- [x] **测试验证**: `go test -p 2 -count=1 -timeout 60s -run 'TestConcurrentUsers' ./test/e2e/` 通过
- [x] **影响范围**: 无影响，纯新增测试文件

### 测试步骤
1. `go test -p 2 -count=1 -timeout 60s -run 'TestConcurrentUsers' ./test/e2e/`
2. 验证3个用户都成功提交任务
3. 验证用户任务隔离（每个用户只能看到自己的任务）

### 验证结果
```
ok  	github.com/cloud-agent-platform/cap/test/e2e	0.513s
```