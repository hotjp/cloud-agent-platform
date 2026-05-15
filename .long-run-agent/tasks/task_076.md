# task_076

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_076.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P1: golangci-lint configuration — .golangci.yml + CI gate


## 需求 (requirements)

Create .golangci.yml with: (1) Enable linters: gofmt, govet, errcheck, staticcheck, gosimple, ineffassign, typecheck, unused, mirrotevasgn; (2) Configure go version 1.25; (3) Excludevendor, exclude generated files; (4) Enable revive for Go style rules; (5) Configure issues exclude rules for generic Go patterns; (6) Create .github/workflows/lint.yml GitHub Actions workflow: runs golangci-lint on all modules, fails on warnings; (7) Add golangci-lint installation to Taskfile (if needed)



## 验收标准 (acceptance)


- golangci-lint run passes on codebase (after fixing any lint errors it finds); CI workflow exists and runs on push/PR; go build passes




## 交付物 (deliverables)

- `.golangci.yml` — golangci-lint configuration with enabled linters (errcheck, govet, staticcheck, unused, gosimple, ineffassign, typecheck, gofmt, goimports, revive, etc.)
- `.github/workflows/lint.yml` — GitHub Actions CI workflow for linting on push/PR



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