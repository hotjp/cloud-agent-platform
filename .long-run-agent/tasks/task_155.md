# task_155

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_155.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

POC-9: Agent Worker 真实容器 — 制作 cap-openclaw-worker Docker 镜像：1) 基于 OpenClaw 子 Agent，2) 挂载宿主机 Volume 到 /workspace，3) 通过环境变量接收 TASK_ID/LLM_GATEWAY_URL 等，4) 调用 LLM 网关（非直连 Provider），5) 调用平台 MCP 工具（read_file/write_file/exec_command），6) 完成后通过 REST API 上报结果。验收：docker build 成功 + 手动跑一个任务能看到真实 LLM 交互。


## 需求 (requirements)

POC-9: Agent Worker 真实容器 — 制作 cap-openclaw-worker Docker 镜像：1) 基于 OpenClaw 子 Agent，2) 挂载宿主机 Volume 到 /workspace，3) 通过环境变量接收 TASK_ID/LLM_GATEWAY_URL 等，4) 调用 LLM 网关（非直连 Provider），5) 调用平台 MCP 工具（read_file/write_file/exec_command），6) 完成后通过 REST API 上报结果。验收：docker build 成功 + 手动跑一个任务能看到真实 LLM 交互。



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/POC-9:_Agent_Worker_.py




## 设计方案 (design)

快速任务


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