# task_124

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_124.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

fix: Worker 执行后自动 git commit + push 到 result branch


## 需求 (requirements)

Worker 完成代码生成后需要:1)git add + commit 改动 2)push 到 result branch 3)将 result branch 信息回写到 task 状态 4)通过 GetDiff API 可以查看改动。当前 gitclient 插件已实现 clone/commit/push，需要在 Worker 执行流程中串联



## 验收标准 (acceptance)


- Worker 执行完后自动 commit+push

- task 状态包含 result branch 信息

- task_status 返回 git 分支和改动文件列表

- GetDiff 返回实际 diff

- go test 通过




## 交付物 (deliverables)

<!-- 在此填写交付物文件路径 -->



## 设计方案 (design)

Worker 执行流程末尾: 1)git add -A 2)git commit -m 'task: {goal}' 3)git push origin {resultBranch} 4)更新 task 状态为 completed 并写入 result_branch 字段


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