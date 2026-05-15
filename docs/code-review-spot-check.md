# 代码抽检报告

## 抽检信息
- **日期**: 2026-05-15
- **抽检人**: Kimi CLI (自动化)
- **项目**: Cloud Agent Platform (Go)
- **抽检文件数**: 10
- **总行数**: ~6,500

## 发现汇总

| # | 严重程度 | 文件 | 行号 | 类别 | 描述 |
|---|---------|------|------|------|------|
| 1 | 🔴 高 | internal/service/task_service.go | 433, 672, 840, 1037 | Bug 风险 | 乐观锁冲突时访问可能为 nil 的 updatedTask，引发 panic |
| 2 | 🔴 高 | internal/orchestrator/orchestrator.go | 344, 714-721 | 并发安全 | executeAgentSession 与 CancelTask 并发读写 session.Status，无同步机制 |
| 3 | 🔴 高 | internal/orchestrator/orchestrator.go | 805-809, 276, 337 | 资源泄漏 | removeSession 从未被调用，sessions map 持续增长 |
| 4 | 🔴 高 | internal/orchestrator/orchestrator.go | 470 | Bug 风险 | agentRunner.Run 失败时 result 可能为 nil，访问 result.ExecutionDuration 引发 panic |
| 5 | 🔴 高 | plugins/llmrouter/router.go | 285, 300, 363, 375, 478 | 并发安全 | Complete/Stream/Embed 中无锁读取 providers map，与 RegisterProvider 写操作竞态 |
| 6 | 🔴 高 | internal/worker/pool/pool.go | 378-398 | 资源泄漏 | Stop 方法创建局部 wg 但未调用 Wait，worker 销毁 goroutine 泄漏 |
| 7 | 🟡 中 | internal/domain/task/task.go | 261-266 | Bug 风险 | findLastKey 依赖随机 map 遍历，WithGuard/WithAction 会附加到错误 transition |
| 8 | 🟡 中 | internal/orchestrator/orchestrator.go | 226-233, 252-258 | 架构一致性 | 内存状态 task.TransitionTo 先于 DB 事务提交，回滚后状态不一致 |
| 9 | 🟡 中 | internal/gateway/rest_adapter.go | 838-861 | 代码质量 | mapDomainStatusToString 缺少 submitted/assigned 状态映射，返回 UNSPECIFIED |
| 10 | 🟡 中 | internal/guardian/guardian.go | 47, 115-117 | 资源泄漏 | pendingReq 无后台超时清理，未处理的审批请求永久驻留内存 |
| 11 | 🟡 中 | internal/gateway/ws/hub.go | 442-455 | 资源泄漏 | Room 创建后永不删除，空 room 持续累积 |
| 12 | 🟡 中 | cmd/server/main.go | 282, 323, 240-266, 287-324 | 架构一致性 | 依赖注入不完整：gitClient、orch、llmRouter、wm 等组件创建后未接入服务层 |
| 13 | 🟢 低 | internal/service/task_service.go | 175, 186, 435, 451 等 | 错误处理 | tx.Rollback(ctx) 返回值被静默忽略 |
| 14 | 🟢 低 | internal/gateway/rest_adapter.go | 57 | 错误处理 | writeError 中 json.Encode 错误未处理，可能写入截断响应 |
| 15 | 🟢 低 | internal/infra/outbox/poller.go | 40 | 错误处理 | NewRedisStreamForwarder 在 logger 为 nil 时 panic，应返回 error |
| 16 | 🟢 低 | cmd/server/main.go | 105 | 错误处理 | zap.NewProduction() 错误被忽略 |
| 17 | 🟢 低 | internal/gateway/ws/client.go | 216-218 | 安全 | handleAuth JWT 验证为 TODO，任意 token 均通过认证 |
| 18 | 🟢 低 | internal/domain/task/task.go | 430-432 | 代码质量 | IsTerminalState 函数与 TaskStatus.IsTerminal() 方法重复 |

## 详细发现

### 发现 #1: 乐观锁冲突时 nil pointer panic
- **文件**: internal/service/task_service.go
- **行号**: 433-438, 672-676, 840-844, 1037-1041
- **类别**: Bug 风险 / 空指针
- **严重程度**: 🔴 高
- **描述**: 在 `Cancel`、`Decompose`、`Retry`、`Decide` 四个方法中，调用 `UpdateStatus` 或 `Update` 后，当返回 `err != nil` 且错误码为 `CodeL2OptimisticLock` 时，代码尝试访问 `updatedTask.Version` 来构造乐观锁错误信息。然而，在出错场景下 `updatedTask` 可能为 `nil`，直接访问其字段将触发运行时 panic。
- **建议修复**: 在访问 `updatedTask.Version` 前增加 nil 检查；或要求 Repository 在乐观锁冲突时保证返回非 nil 的 aggregate（携带当前 DB version）。
- **代码片段**:
  ```go
  updatedTask, err := s.taskRepo.UpdateStatus(ctx, req.TaskID, domain.TaskStatusCancelled, task.Version)
  if err != nil {
      tx.Rollback(ctx)
      if domain.CodeIs(err, domain.CodeL2OptimisticLock) {
          // ⚠️ updatedTask 可能为 nil，导致 panic
          spanErr = domain.NewL2OptimisticLockError("Task", req.TaskID, task.Version, updatedTask.Version)
          return nil, spanErr
      }
      ...
  }
  ```

### 发现 #2: session 状态并发竞态
- **文件**: internal/orchestrator/orchestrator.go
- **行号**: 344, 714-721
- **类别**: 并发安全
- **严重程度**: 🔴 高
- **描述**: `executeAgentSession` 在独立的 goroutine 中直接赋值 `session.Status = "running"`（line 344）。与此同时，`CancelTask` 方法在 `sessionsMu.RLock()` 保护下遍历并修改同一 session 的 `Status`（line 714-721）。`executeAgentSession` 未获取任何锁，构成典型的 data race。
- **建议修复**: 对 session 的写操作统一通过 `sessionsMu.Lock()` 保护，或在 `agentSession` 结构体内部使用 `sync.RWMutex` 保护自身字段。
- **代码片段**:
  ```go
  // executeAgentSession (line 344) — 无锁写
  session.Status = "running"

  // CancelTask (line 714-721) — 读锁写
  o.sessionsMu.RLock()
  for _, session := range o.sessions {
      if session.TaskID == taskID && session.Status == "running" {
          session.Status = "cancelled"  // 与上行竞态
          ...
      }
  }
  o.sessionsMu.RUnlock()
  ```

### 发现 #3: session 内存泄漏（removeSession 死代码）
- **文件**: internal/orchestrator/orchestrator.go
- **行号**: 805-809, 276, 337
- **类别**: 资源泄漏 / goroutine 泄漏
- **严重程度**: 🔴 高
- **描述**: `removeSession` 方法已定义但**从未被调用**。`executeAgentSession` 在成功或失败后仅更新 session 状态，不会将其从 `sessions` map 中移除。随着任务量增长，`sessions` map 将无限膨胀，造成不可逆的内存泄漏。
- **建议修复**: 在 `handleAgentSuccess` 和 `handleAgentFailure` 的尾部调用 `o.removeSession(session.ID)`；或在 `executeAgentSession` 的 defer 中清理。
- **代码片段**:
  ```go
  // 已定义但从未调用
  func (o *OrchestratorImpl) removeSession(sessionID string) {
      o.sessionsMu.Lock()
      defer o.sessionsMu.Unlock()
      delete(o.sessions, sessionID)
  }
  ```

### 发现 #4: agentRunner.Run 失败时 result 可能为 nil 导致 panic
- **文件**: internal/orchestrator/orchestrator.go
- **行号**: 470
- **类别**: Bug 风险 / 空指针
- **严重程度**: 🔴 高
- **描述**: `agentRunner.Run` 返回 `(result *AgentResult, err error)`。当 `err != nil` 时，`result` 可能为 `nil`。但后续构造 `ExperienceRecordParams` 时直接访问 `result.ExecutionDuration.Milliseconds()`，将触发 nil pointer dereference。
- **建议修复**: 在 `err != nil` 分支中为 `DurationMs` 等字段赋予零值，或判断 `result != nil` 后再访问。
- **代码片段**:
  ```go
  result, err := o.agentRunner.Run(ctx, subtask, task)
  ...
  if o.expRecorder != nil {
      expParams := ExperienceRecordParams{
          ...
          DurationMs: int(result.ExecutionDuration.Milliseconds()), // ⚠️ err!=nil 时 result 可能 nil
          TokensUsed: result.TokensUsed,
          ...
      }
  }
  ```

### 发现 #5: providers map 并发读写竞态
- **文件**: plugins/llmrouter/router.go
- **行号**: 285, 300, 363, 375, 478
- **类别**: 并发安全
- **严重程度**: 🔴 高
- **描述**: `Router` 的 `Complete`、`Stream`、`Embed`、`findAlternativeModel` 等方法在读取 `r.providers` map 时未持有任何锁，而 `RegisterProvider` 方法在 `r.mu.Lock()` 下写入该 map。Go 中并发读写 map 会导致运行时 fatal error（"concurrent map read and map write"）。
- **建议修复**: 所有读取 `r.providers` 的代码块统一使用 `r.mu.RLock()` / `RUnlock()` 保护；或在初始化阶段完成所有 provider 注册，后续只读。
- **代码片段**:
  ```go
  func (r *Router) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
      ...
      provider, ok := r.providers[model]  // 无锁读
      ...
  }

  func (r *Router) RegisterProvider(provider LLMProvider) {
      r.mu.Lock()
      defer r.mu.Unlock()
      r.providers[provider.Name()] = provider  // 加锁写
  }
  ```

### 发现 #6: Pool.Stop 中局部 WaitGroup 未等待导致 goroutine 泄漏
- **文件**: internal/worker/pool/pool.go
- **行号**: 378-398
- **类别**: 资源泄漏 / goroutine 泄漏
- **严重程度**: 🔴 高
- **描述**: `Stop` 方法为每个 worker 创建了销毁 goroutine 并加入局部 `sync.WaitGroup`，但代码随后立即进入 `select` 等待 `done` channel（该 channel 与局部 wg 无关），**从未调用局部 `wg.Wait()`**。因此 `Stop` 返回时，worker sandbox 的销毁 goroutine 仍在后台运行，且可能持有对 pool 的引用。
- **建议修复**: 在 `select` 之前（或作为 `select` 的一个 case）等待局部 `wg`。
- **代码片段**:
  ```go
  p.mu.Lock()
  var wg sync.WaitGroup
  for id, w := range p.workers {
      wg.Add(1)
      go func(id string, w *Worker) {
          defer wg.Done()
          timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
          defer cancel()
          _ = p.sb.Destroy(timeout, id)
      }(id, w)
  }
  p.mu.Unlock()

  // ❌ 局部 wg 从未被 Wait
  select {
  case <-done:
  case <-time.After(p.cfg.ShutdownTimeout):
      p.logger.Warn("timeout during worker cleanup, forcing shutdown")
  }
  ```

### 发现 #7: findLastKey 依赖随机 map 遍历顺序
- **文件**: internal/domain/task/task.go
- **行号**: 261-266
- **类别**: Bug 风险
- **严重程度**: 🟡 中
- **描述**: `findLastKey` 通过 `for k := range sm.transitions { return &k }` 返回 "最后一个" transition key。但 Go map 的遍历顺序是随机的，因此该函数实际返回的是**任意一个** key。`WithGuard` 和 `WithAction` 依赖此方法将 guard/action 附加到"最近添加"的 transition 上，会导致行为不可预期。虽然目前代码中这两个方法未被调用，但属于重大设计缺陷。
- **建议修复**: 用 slice 按插入顺序维护 transitions，或让 `AddTransition` 返回 key 供后续 `WithGuard`/`WithAction` 显式引用。
- **代码片段**:
  ```go
  func (sm *StateMachine) findLastKey() *transitionKey {
      for k := range sm.transitions {
          return &k  // 随机返回
      }
      return nil
  }
  ```

### 发现 #8: 内存状态与 DB 事务不一致
- **文件**: internal/orchestrator/orchestrator.go
- **行号**: 226-233, 252-258, 及多处
- **类别**: 架构一致性
- **严重程度**: 🟡 中
- **描述**: `startSingleAgentExecution` 中，先在事务内调用 `task.TransitionTo("StartDecomposition")`（修改内存对象状态），再执行 DB `UpdateStatus`。如果后续 `tx.Commit` 失败并回滚，内存中的 `task.Status` 已经被改为 `decomposing`，但数据库仍是 `pending`。调用方若继续使用该 `task` 对象（如再次尝试 StartTask），将基于脏状态做决策。
- **建议修复**: 采用"先写 DB 成功，再更新内存对象"的顺序；或使用事务内的纯 DB 状态做校验，提交后再同步内存对象。
- **代码片段**:
  ```go
  if err := task.TransitionTo("StartDecomposition"); err != nil {
      tx.Rollback(ctx)
      ...
  }
  if _, err := o.taskRepo.UpdateStatus(ctx, task.ID, domain.TaskStatusDecomposing, task.Version); err != nil {
      tx.Rollback(ctx)
      return err
  }
  if err := tx.Commit(ctx); err != nil { ... }
  ```

### 发现 #9: REST 层缺少状态映射
- **文件**: internal/gateway/rest_adapter.go
- **行号**: 838-861
- **类别**: 代码质量
- **严重程度**: 🟡 中
- **描述**: `mapDomainStatusToString` 缺少对 `TaskStatusSubmitted` 和 `TaskStatusAssigned` 的 case 分支。当 task 处于这两个状态时，REST API 将返回 `"UNSPECIFIED"`，导致前端状态展示异常。
- **建议修复**: 补充 `case domain.TaskStatusSubmitted: return "SUBMITTED"` 和 `case domain.TaskStatusAssigned: return "ASSIGNED"`。

### 发现 #10: Guardian 审批请求无超时清理
- **文件**: internal/guardian/guardian.go
- **行号**: 47, 115-117
- **类别**: 资源泄漏
- **严重程度**: 🟡 中
- **描述**: `pendingReq` map 在 `RequestApproval` 中写入条目，仅在 `ProcessApproval` 中删除。系统**没有**后台 goroutine 检查 `ExpiresAt` 并自动清理超时请求。如果用户永远不操作，或外部系统未调用 `HandleTimeout`，该条目将永久驻留内存。
- **建议修复**: 启动一个后台 ticker，定期扫描 `pendingReq` 中 `time.Now().After(req.ExpiresAt)` 的条目，调用 `HandleTimeout` 清理。
- **代码片段**:
  ```go
  func (g *Guardian) RequestApproval(...) (*ApprovalRequest, error) {
      ...
      g.mu.Lock()
      g.pendingReq[task.ID] = req  // 只增
      g.mu.Unlock()
      ...
  }
  ```

### 发现 #11: WebSocket Room 永不删除
- **文件**: internal/gateway/ws/hub.go
- **行号**: 442-455
- **类别**: 资源泄漏 / 内存泄漏
- **严重程度**: 🟡 中
- **描述**: `getOrCreateRoom` 会在 `h.rooms` 中创建 room，但 `unregisterClient` 仅在 room 中移除 client，**从不删除 room 本身**。随着 task 数量增长，空 room 将持续累积，导致 `h.rooms` map 内存无限增长。
- **建议修复**: 在 `unregisterClient` 中检查 `room.ClientCount() == 0`，若为空则 `delete(h.rooms, key)`。
- **代码片段**:
  ```go
  func (h *Hub) unregisterClient(client *Client) {
      ...
      if room != nil {
          room.Remove(client)
          // ❌ 未检查 room 是否为空并删除
      }
      ...
  }
  ```

### 发现 #12: 依赖注入不完整
- **文件**: cmd/server/main.go
- **行号**: 282, 323, 240-266, 287-324
- **类别**: 架构一致性
- **严重程度**: 🟡 中
- **描述**: `main` 函数中创建了多个核心组件（`gitClient`、`llmRouter`、`orch`、`wm` 等），但大量组件仅以 `_ = component` 的方式存在，**未真正注入到 service 层或 gateway 层**。例如 `orchestrator.New()` 创建了一个无依赖的 orchestrator，但实际需要的是 `orchestrator.NewOrchestrator`（需要 taskRepo、subtaskRepo 等）。`TaskService` 也未接收 orchestrator 或 worker manager 的依赖。这导致系统虽然能编译启动，但核心编排和 worker 能力并未真正可用。
- **建议修复**: 梳理组件依赖图，将仓库、orchestrator、worker manager、llm router 等通过构造函数注入到 service 和 gateway 中。
- **代码片段**:
  ```go
  orch := orchestrator.New()  // 无参构造，与实际 NewOrchestrator 不符
  _ = orch

  gitClient := gitclient.New(logger)
  _ = gitClient  // 未注入到任何地方
  ```

### 发现 #13: tx.Rollback 返回值被忽略
- **文件**: internal/service/task_service.go
- **行号**: 175, 186, 435, 451 等（多处）
- **类别**: 错误处理
- **严重程度**: 🟢 低
- **描述**: 所有事务回滚点均使用 `tx.Rollback(ctx)` 而不检查返回值。虽然回滚失败的情况较少，但如果连接已断开，忽略错误会掩盖底层问题。
- **建议修复**: 统一使用 `_ = tx.Rollback(ctx)` 显式忽略（如果确实不需要处理），或记录日志。

### 发现 #14: writeError 中 JSON 编码错误未处理
- **文件**: internal/gateway/rest_adapter.go
- **行号**: 57
- **类别**: 错误处理
- **严重程度**: 🟢 低
- **描述**: `writeError` 在调用 `json.NewEncoder(w).Encode(resp)` 时未检查错误。如果响应已部分写入后发生编码错误，客户端可能收到截断的 JSON。
- **建议修复**: 记录日志或至少显式忽略 `_ = json.NewEncoder(w).Encode(resp)`。

### 发现 #15: NewRedisStreamForwarder 使用 panic
- **文件**: internal/infra/outbox/poller.go
- **行号**: 40
- **类别**: 错误处理
- **严重程度**: 🟢 低
- **描述**: 当 logger 为 nil 时直接 `panic("logger is required")`。在初始化路径中使用 panic 会直接导致进程崩溃，不如返回 error 让调用方决定如何处理。
- **建议修复**: 将函数签名改为返回 `(*RedisStreamForwarder, error)`，nil logger 时返回 error。

### 发现 #16: zap.NewProduction 错误被忽略
- **文件**: cmd/server/main.go
- **行号**: 105
- **类别**: 错误处理
- **严重程度**: 🟢 低
- **描述**: `logger, _ := zap.NewProduction()` 忽略了 error。虽然极少失败，但若日志初始化失败，后续所有 `logger.Info/Error` 均为 no-op，导致启动问题无法排查。
- **建议修复**: `if err != nil { fmt.Fprintf(os.Stderr, ...); os.Exit(1) }`

### 发现 #17: WebSocket JWT 认证未实现
- **文件**: internal/gateway/ws/client.go
- **行号**: 216-218
- **类别**: 安全
- **严重程度**: 🟢 低
- **描述**: `handleAuth` 方法仅检查 token 非空，未进行任何 JWT 签名验证或过期检查，直接标记 `authenticated = true`。攻击者可以发送任意非空字符串通过认证。
- **建议修复**: 接入 L3-Authz 服务的 JWT 验证逻辑，校验签名、过期时间和 claims。

### 发现 #18: IsTerminalState 重复定义
- **文件**: internal/domain/task/task.go
- **行号**: 430-432
- **类别**: 代码质量
- **严重程度**: 🟢 低
- **描述**: 包级函数 `IsTerminalState` 与 `TaskStatus` 的方法 `IsTerminal()` 功能完全重复，属于死代码/重复代码。
- **建议修复**: 删除 `IsTerminalState` 函数，统一使用 `TaskStatus.IsTerminal()`。

## 总体评估

- **代码健康度评分**: 6.5/10
- **关键风险**:
  1. **并发安全缺陷**: orchestrator 的 session 竞态、llmrouter 的 providers map 竞态，均可能在生产环境触发 panic 或数据损坏。
  2. **空指针 panic**: task_service 和 orchestrator 中存在多条在错误路径上访问可能为 nil 的指针的代码，高并发下触发概率上升。
  3. **资源泄漏**: session map、WebSocket room、guardian pendingReq 均缺乏清理机制，长期运行将导致 OOM。
  4. **架构断层**: main.go 中大量组件未真正注入，五层架构的 L4-L5 连接存在缺口，系统功能未完全贯通。

- **优化建议**:
  1. **立即修复** nil pointer 和 map 竞态问题（🔴 高），这些是生产事故的直接诱因。
  2. **补充生命周期管理**: 为 session、room、pendingReq 添加定时清理或事件驱动的删除逻辑。
  3. **完善依赖注入**: 梳理 `main.go` 的组件依赖图，将 orchestrator、worker manager、llm router 等真正接入 service 层。
  4. **统一错误处理规范**: 禁止在库代码中使用 panic（如 outbox forwarder），所有 `Rollback` 和 `Encode` 错误至少显式记录。
  5. **引入数据竞争检测**: 在 CI 中运行 `go test -race`，尤其覆盖 orchestrator、llmrouter、pool 三个包。
  6. **状态机完善**: 修复 `findLastKey` 的随机性问题，确保状态机扩展时行为可预期。
