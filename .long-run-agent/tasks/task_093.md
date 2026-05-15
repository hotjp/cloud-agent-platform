# task_093

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_093.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P1: Worker Docker image — Dockerfile.worker + build in CI


## 需求 (requirements)

The project has Dockerfile for server but Worker runs in a separate container (or CubeSandbox). Create: (1) Dockerfile.worker or worker/Dockerfile with: Go build of worker binary (if worker is separate process), or just the agent runtime; (2) Worker needs: git, Go runtime, access to workspace directory; (3) Build args: BUILD_TAGS for build flags; (4) Non-root user (worker user, UID 1000); (5) seccomp profile (seccomp-worker.json); (6) Read-only rootfs except /workspace and /tmp; (7) Docker build with: docker build -t cap-worker -f Dockerfile.worker .; (8) GitHub Actions step to build worker image; (9) Consider multi-stage build: builder stage + runtime stage



## 验收标准 (acceptance)


- docker build -t cap-worker -f Dockerfile.worker . succeeds; Worker container starts and connects to platform server; Security: runs as non-root

- read-only rootfs

- seccomp enabled; CI builds worker image on every push




## 交付物 (deliverables)

- `Dockerfile.worker` — multi-stage build (golang:1.23-alpine → alpine:3.19), non-root worker user (UID 1000), BUILD_TAGS arg, HEALTHCHECK /health
- `seccomp-worker.json` — (already existed) security profile restricting dangerous syscalls



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