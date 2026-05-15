# task_128

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_128.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

T-E2E-01: 两个Agent并行改同仓库不同文件


## 需求 (requirements)

同时提交两个task改同一仓库(octocat/Hello-World)的不同文件(hello-a.txt和hello-b.txt)



## 验收标准 (acceptance)


- 共享同一Git容器

- 各自独立branch

- 两个task都COMPLETED




## 交付物 (deliverables)

- `test/e2e/parallel_agents_test.go` - E2E测试：两个独立子任务并行执行无冲突
- `internal/orchestrator/orchestrator.go` - 修改StartTask支持所有就绪子任务并行启动
- `internal/orchestrator/orchestrator.go` - 修复handleAgentSuccess并发完成处理



## 设计方案 (design)

<!-- 在此填写架构设计、技术选型、实现思路 -->


## 验证证据（完成前必填）

- [x] **实现证明**: 修改orchestrator.go的StartTask方法，将原来只启动第一个就绪子任务改为启动所有就绪子任务（无依赖关系的子任务并行执行）。修改handleAgentSuccess处理并发完成场景（任务已由其他session完成时优雅退出）。
- [x] **测试验证**: `go test -p 2 -count=1 -timeout 60s -run 'TestParallelAgents' ./test/e2e/` 通过，所有E2E测试通过
- [x] **影响范围**: 修改了`TestFullPipeline_SubmitToComplete`的预期行为（现在并行执行所有独立子任务而非只执行一个），已同步更新测试断言

### 测试步骤
1. `go test -p 2 -count=1 -timeout 60s -run 'TestParallelAgents' ./test/e2e/` - 运行并行Agent测试
2. `go test -p 2 -count=1 -timeout 120s ./test/e2e/...` - 运行所有E2E测试验证无回归

### 验证结果
```
ok  	github.com/cloud-agent-platform/cap/test/e2e	6.483s
```