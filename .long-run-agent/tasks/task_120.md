# task_120

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_120.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: Orchestrator 注入到 TaskService — 连接事件处理和任务编排


## 需求 (requirements)

当前 Orchestrator 在 cmd/server/main.go 创建后被 _ = orch 忽略。TaskService 没有 orchestrator 依赖。需要:1)TaskService 接受 orchestrator 接口注入 2)TaskSubmitted 后通过 orchestrator 自动触发 Decompose 3)连接 outbox 事件到 orchestrator 的 dispatcher



## 验收标准 (acceptance)


- TaskService 构造函数接受 orchestrator 参数

- Orchestrator 在 main.go 被正确注入

- TaskSubmitted 后能触发 orchestrator.StartTask()

- go build 和 go test 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

1)定义 orchestrator 接口(在 service 的 interfaces.go) 2)TaskService 增加 orchestrator 字段 3)Submit 成功后异步调用 orchestrator.StartTask() 或通过事件驱动 4)main.go 注入实际 orchestrator


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