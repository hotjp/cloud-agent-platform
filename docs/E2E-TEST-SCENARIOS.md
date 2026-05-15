# Test Scenarios for Cloud Agent Platform

## Phase 1: Single-Agent 测试（已通过 ✅）
- task_submit → Git Container → Worker → LLM → Commit → COMPLETED

## Phase 2: 多 Agent 并行测试

### T-E2E-01: 两个 Agent 并行改同一个仓库的不同文件
**场景**: 两个 task 同时提交，改同一个项目（octocat/Hello-World），但改不同文件
**预期**: 
- 共享同一个 Git 容器（projectID 相同）
- 两个 Worker 都能成功执行
- 各自创建独立 branch
- 文件都在 volume 里
**验证**: `lra list` 两个 task 都 completed

### T-E2E-02: 两个 Agent 并行改不同仓库
**场景**: task A 改 octocat/Hello-World，task B 改另一个仓库
**预期**:
- 创建两个不同的 Git 容器（不同 projectID）
- 独立执行，互不影响

### T-E2E-03: 非代码任务（空项目）
**场景**: 提交 task 不带 repo_url，Agent 生成一份文档
**预期**:
- Git 容器 git init 空仓库
- Worker 生成文件 → commit
- Git log 有 commit 记录

### T-E2E-04: MCP 工具链（Claude Code 调用 MCP）
**场景**: Claude Code 通过 MCP 提交 task，轮询状态，获取 diff
**流程**:
1. Claude Code 调 `task_submit`
2. 轮询 `task_status` 直到 COMPLETED
3. 调 `task_diff` 获取结果
**验证**: Claude Code 能拿到 diff 并展示给用户

### T-E2E-05: 多人并发提交
**场景**: 模拟 3 个不同用户（不同 JWT token）同时提交 task
**预期**:
- 所有 task 都能正常执行
- 不出现资源冲突
- Git 容器按项目复用

### T-E2E-06: 同项目串行任务
**场景**: task A 完成后，task B 在同一项目上继续修改
**预期**:
- task B 能看到 task A 的文件（共享 volume）
- Git log 有两次 commit
- 文件版本正确

### T-E2E-07: Agent 失败恢复
**场景**: 提交一个不可能完成的 task（如改不存在的文件、LLM 返回无效 JSON）
**预期**:
- task 进入 FAILED 状态
- Git 容器不受影响
- 可以重新提交

### T-E2E-08: Git Push（需要 writable repo）
**场景**: 配置 GIT_TOKEN，提交 task 到有写权限的仓库
**预期**:
- Worker commit 后自动 push
- 远程仓库有新 branch + commit

### T-E2E-09: 任务取消
**场景**: 提交 task 后立即调 task_cancel
**预期**:
- task 进入 CANCELLED 状态
- 容器被清理

### T-E2E-10: 压力测试 — 10 个 Agent 同时执行
**场景**: 快速连续提交 10 个 task
**预期**:
- 所有 task 完成
- 不超过 MaxContainers 限制
- Git 容器正确复用
