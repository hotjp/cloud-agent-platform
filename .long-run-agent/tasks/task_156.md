# task_156

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_156.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

POC-10: 真实文件访问延迟测试 — 重写 test/poc/file_access_test.go，接入真实组件：1) 启动 Git 容器（cap-git），2) 启动 Agent Worker 容器（cap-openclaw-worker），3) Worker 通过 MCP 调 read_file/write_file，4) 测 P50/P95/P99 延迟。极限场景：50 Agent 并发读同一项目、1GB 单文件、磁盘写满。输出延迟报告到 docs/POC-RESULTS.md。依赖: POC-8 + POC-9。


## 需求 (requirements)

POC-10: 真实文件访问延迟测试 — 重写 test/poc/file_access_test.go，接入真实组件：1) 启动 Git 容器（cap-git），2) 启动 Agent Worker 容器（cap-openclaw-worker），3) Worker 通过 MCP 调 read_file/write_file，4) 测 P50/P95/P99 延迟。极限场景：50 Agent 并发读同一项目、1GB 单文件、磁盘写满。输出延迟报告到 docs/POC-RESULTS.md。依赖: POC-8 + POC-9。



## 验收标准 (acceptance)


- 完成




## 交付物 (deliverables)


- src/POC-10:_真实文件访问延迟测试_—.py




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