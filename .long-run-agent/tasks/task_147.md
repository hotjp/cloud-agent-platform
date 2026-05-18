# task_147

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_147.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

POC-1: OpenClaw Agent Worker 镜像 — 制作 OpenClaw 子 Agent 的 Docker 镜像（cap-openclaw-worker），能挂载共享 Volume、通过环境变量接收任务信息、调用 MCP 工具（read_file/write_file/exec_command）、走 LLM 网关调 ask_llm。替代当前 cap-worker 的 entrypoint.sh 自建循环。详见 docs/EXTERNAL-AGENT-ARCHITECTURE.md 第三章。


## 需求 (requirements)

POC-1: OpenClaw Agent Worker 镜像 — 制作 OpenClaw 子 Agent 的 Docker 镜像（cap-openclaw-worker），能挂载共享 Volume、通过环境变量接收任务信息、调用 MCP 工具（read_file/write_file/exec_command）、走 LLM 网关调 ask_llm。替代当前 cap-worker 的 entrypoint.sh 自建循环。详见 docs/EXTERNAL-AGENT-ARCHITECTURE.md 第三章。



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/POC-1:_OpenClaw_Agen.py




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