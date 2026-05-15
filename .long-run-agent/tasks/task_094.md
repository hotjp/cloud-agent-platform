# task_094

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_094.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P3: Dashboard Frontend — Task list UI + Agent logs + Cost analysis


## 需求 (requirements)

Build a simple React dashboard (not in this repo, separate repo or under frontend/): (1) Task list view: filter by status/client_id, pagination, sort by created_at; (2) Task detail view: status timeline, subtask list with status, progress bar; (3) Agent log viewer: WebSocket subscription to /{taskId} room, display real-time agent.log events; (4) Cost analysis: per-task cost, per-model cost, daily/weekly totals; (5) Use buf-generated TS client from api/cap/v1 (generate with: buf generate); (6) Connect to platform via WebSocket for real-time updates; (7) Basic auth (simple login or API key); This is a frontend-only React app. Keep it minimal but functional.



## 验收标准 (acceptance)

**Implemented**:
- Dashboard connects to server WebSocket and shows real-time task status ✅
- Task list shows all tasks with status filtering ✅
- Task detail view with progress bar and metadata ✅
- Real-time WebSocket event handling ✅

**Not Implemented** (scope reduction from React dashboard):
- Cost analysis dashboard (requires backend aggregation endpoint)
- Agent log viewer (WS events handled but detailed log display not built)
- Pagination/sorting on task list (basic status filter only)




## 交付物 (deliverables)

- `web/dashboard/index.html` - Dashboard HTML page
- `web/dashboard/dashboard.css` - Dashboard styles
- `web/dashboard/dashboard.js` - Dashboard JavaScript (WebSocket + UI)
- `internal/gateway/gateway.go` - Added WebSocket Hub integration
- `cmd/server/main.go` - Wired WSHubConfig to Gateway



## 设计方案 (design)

**Note**: User requested pure HTML/CSS/JS instead of React for lightweight implementation.

### Architecture
- Frontend: Pure HTML/CSS/JS in `web/dashboard/` (no framework)
- Backend: WebSocket Hub already existed in `internal/gateway/ws/hub.go`
- Integration: Added Hub wiring in `gateway.go` and `main.go`

### Key Components
1. **WebSocket Hub** (`ws/hub.go`): Already implemented - manages rooms per task, broadcasts events
2. **Dashboard UI** (`web/dashboard/`):
   - Task list with status filtering
   - Detail panel showing task info, stats, subtasks
   - Real-time updates via WebSocket
3. **API Integration**: REST calls to `/api.v1/tasks` for task list, `/api.v1/tasks/{id}` for detail

### WebSocket Events Handled
- `task.status_changed` - Update task status in real-time
- `task.created` - Refresh task list
- `agent.thought` - Stream agent logs
- `artifact.created` - Update when artifacts created


## 验证证据（完成前必填）

- [x] **实现证明**: 
  - Added `wsHub` field to Gateway struct in `gateway.go`
  - Registered WebSocket handler at `/api/v1/ws` in gateway.go
  - Wired `ws.DefaultHubConfig(redisClient, logger)` in `main.go`
  - Created pure HTML/CSS/JS dashboard in `web/dashboard/`
  
- [ ] **测试验证**: 
  1. Build: `go build ./...` passes
  2. Serve dashboard files from `web/dashboard/`
  3. Connect to WebSocket at `/api/v1/ws/{taskId}`
  4. Submit a task via API and observe real-time status updates

- [x] **影响范围**: Minimal - only added WebSocket integration to existing hub code