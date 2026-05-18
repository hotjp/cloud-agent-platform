# task_134

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_134.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

T-E2E-07: Agent失败场景测试


## 需求 (requirements)

提交不可能完成的task,验证失败恢复



## 验收标准 (acceptance)


- task进入FAILED状态

- Git容器不受影响

- 可重新提交




## 交付物 (deliverables)

`test/e2e/agent_failure_test.go` - E2E测试文件，包含以下测试用例：
- TestAgentFailure_LLMError: LLM返回错误场景测试
- TestAgentFailure_ExecutionTimeout: Agent执行超时场景测试
- TestAgentFailure_CommandExecutionFailure: 命令执行失败场景测试
- TestAgentFailure_GitPermissionDenied: Git操作权限失败场景测试
- TestAgentFailure_MultipleErrorTypes: 多种错误类型可读性测试
- TestAgentFailure_ErrorTraceability: 错误可追溯性测试
- TestAgentFailure_CascadingFailure: 级联失败传播测试
- TestAgentFailure_ReadbleErrorMessages: 错误消息可读性测试
- TestAgentFailure_MCPProtocolErrors: MCP协议错误处理测试
- TestAgentFailure_ConcurrentFailures: 并发失败场景测试
- TestAgentFailure_ErrorsIs: 错误包装与unwrap测试



## 设计方案 (design)

<!-- 在此填写架构设计、技术选型、实现思路 -->


## 验证证据（完成前必填）

- [x] **实现证明**: 使用httptest创建mock HTTP handler模拟平台API，实现failureTestHandler支持多种失败模式（LLM错误、超时、命令失败、Git权限拒绝）。通过PlatformClient和MCP Server进行端到端测试。
- [x] **测试验证**: `go test -p 2 -count=1 -timeout 60s -run 'TestAgentFailure' ./test/e2e/` 通过所有11个测试用例
- [x] **影响范围**: 无影响，仅新增测试文件

### 测试步骤
1. `go test -p 2 -count=1 -timeout 60s -run 'TestAgentFailure' ./test/e2e/`
2. 验证所有测试通过

### 验证结果
```
ok  	github.com/cloud-agent-platform/cap/test/e2e	0.524s
```