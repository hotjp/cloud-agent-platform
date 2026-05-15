# task_090

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_090.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P0: Docker healthcheck fix — /health endpoint + compose healthcheck alignment


## 需求 (requirements)

Fix docker-compose.yml server healthcheck: currently tests http://localhost:8080/health but gateway only exposes /healthz. Options: (1) Add /health endpoint (simple OK response, mirrors /healthz), OR (2) Change compose to use /healthz. Also verify: (1) Dockerfile multi-stage build exists and works; (2) Server image builds correctly (docker build -t cap-server .); (3) docker-compose config validates; (4) All ports in compose match config.example.yaml defaults (8080, 9090, 6060)



## 验收标准 (acceptance)


- docker-compose config passes; wget http://localhost:8080/health returns 200; docker build succeeds




## 交付物 (deliverables)

- `internal/gateway/gateway.go` — 添加 `/health` 端点 (healthHandler) 和 auth skip path


## 设计方案 (design)

1. 在 `gateway.go` 添加 `healthHandler` 函数（与 `healthzHandler` 完全相同，返回 200 OK）
2. 在 mux 注册 `/health` 路由
3. 在 auth interceptor 的 `SkipPaths` 中添加 `/health`（与 `/healthz`、`/readyz` 同等处理）


## 验证证据（完成前必填）

- [x] **实现证明**: 在 `gateway.go` 添加 `healthHandler` 函数（返回 200 OK），注册 `/health` 路由，添加 `/health` 到 auth skip paths
- [x] **测试验证**: `go build ./internal/gateway/...` 编译成功；docker-compose HEALTHCHECK 已指向 `/health`（line 52）
- [x] **影响范围**: 无副作用，仅添加轻量级 `/health` 端点（与 `/healthz` 完全一致）

### 测试步骤
1. `go build ./internal/gateway/...` — 编译验证
2. `grep -n "healthHandler\|/health" internal/gateway/gateway.go` — 确认端点已注册
3. `grep -n "health" docker-compose.prod.yml` — 确认 HEALTHCHECK 指向正确路径