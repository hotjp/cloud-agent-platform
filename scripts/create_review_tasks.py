import json, os, datetime

task_list_path = ".long-run-agent/task_list.json"
tasks_dir = ".long-run-agent/tasks"

with open(task_list_path) as f:
    data = json.load(f)

next_num = len(data["tasks"]) + 1

def tid(n):
    return f"task_{n:03d}"

review_tasks = [
    {
        "desc": "Review R01: internal/config — 配置系统审查",
        "detail": "审查 internal/config/ 下所有文件。重点: koanf多源加载、APP_前缀环境变量、无全局单例、必填校验。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R02: internal/domain 核心层 — 领域实体+错误码+状态机+事件",
        "detail": "审查 internal/domain/ 下 entity/errors/events/statemachine/task-types/repositories.go。重点: 零外部依赖、错误码L2=[200,399]、领域事件格式、状态机声明式定义。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R03: internal/domain/task — Task领域模型",
        "detail": "审查 internal/domain/task/。重点: 9态状态机、状态转换守卫、领域不变量、Outbox事件收集。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R04: internal/domain/context — 上下文领域模型",
        "detail": "审查 internal/domain/context/。重点: TaskContext/FileState/ConversationTurn实体、压缩保留goal/constraints/user_decision。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R05: internal/domain/repository+worker — 仓库与Worker接口",
        "detail": "审查 internal/domain/repositories.go 和 internal/domain/worker/backend.go。重点: 接口方法签名与ent实现对齐、WorkerBackend双实现支持。修复发现的问题。",
        "priority": "P2",
    },
    {
        "desc": "Review R06: internal/infra/persistence — 存储层",
        "detail": "审查 internal/infra/persistence/ 所有文件。重点: ent Schema与设计对齐、事务管理（同库同事务）、SELECT FOR UPDATE SKIP LOCKED、错误码使用。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R07: internal/infra/cache — Redis缓存层",
        "detail": "审查 internal/infra/cache/ 所有文件。重点: 分布式锁Redlock、看门狗续期、TaskContext热层、缓存穿透防护。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R08: internal/infra/outbox — Outbox系统",
        "detail": "审查 internal/infra/outbox/ 所有文件。重点: 事务内写入、轮询器生命周期、SKIP LOCKED防重复、Redis Stream发布、ACK+指数退避重试。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R09: internal/service — 业务服务层",
        "detail": "审查 internal/service/（不含context子目录）。重点: TaskService 5方法、事务边界、接口注入（禁止import plugins）、错误码L4=[600,799]。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R10: internal/service/context — 上下文服务",
        "detail": "审查 internal/service/context/。重点: full/summary/delta三种模式、压缩引擎L1+L3、保留关键字段。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R11: internal/gateway+ws — 网关层+WebSocket",
        "detail": "审查 internal/gateway/ 含ws子目录。重点: connect-go handler、JWT仅解密不验证、中间件顺序(Recover→RequestID→Metrics→Logging→CORS→Auth→Routing)、sentinel-go限流、WebSocket Hub。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R12: internal/orchestrator — 编排层",
        "detail": "审查 internal/orchestrator/ 含matcher子目录。重点: 编排接口、Agent匹配算法(能力×0.5+成功率×0.3+成本×0.2)、事件定义。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R13: plugins/orchestrator — Eino编排图",
        "detail": "审查 plugins/orchestrator/ 所有文件。重点: 3路路由(简单/中等/复杂)、Eino Graph定义(节点/边/条件分支)、节点动态注册、ReAct循环。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R14: plugins/llmrouter — LLM路由",
        "detail": "审查 plugins/llmrouter/ 所有文件。重点: Claude/GLM多模型路由、自适应升降级(3次<80%降级/5次>95%升级)、熔断器、重试策略。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R15: plugins/workermanager — Worker管理",
        "detail": "审查 plugins/workermanager/ 所有文件。重点: SandboxBackend双实现、Docker安全加固(seccomp/AppArmor/2GB)、CubeSandbox(<60ms)、降级策略。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R16: internal/agent/react+tools — Agent核心",
        "detail": "审查 internal/agent/react/ 和 internal/agent/tools/。重点: ReAct循环(Thought→Action→Observation,最多15步)、Tool接口统一、文件5工具+Git4工具+命令+LLM工具。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R17: internal/agent/roles — Agent角色系统",
        "detail": "审查 internal/agent/roles/。重点: 6核心角色(observer/strategist/executor/guardian/tester/researcher)、prompt模板、工具集分配、角色权限表。修复发现的问题。",
        "priority": "P1",
    },
    {
        "desc": "Review R18: internal/worker — Worker池与沙箱",
        "detail": "审查 internal/worker/ 含pool和sandbox子目录。重点: Worker池预热/扩缩容、健康检查、Sandbox抽象与plugins/workermanager对齐。修复发现的问题。",
        "priority": "P2",
    },
    {
        "desc": "Review R19: 横切模块 — guardian/mcp/authz/storage/observability",
        "detail": "审查 internal/guardian/、internal/mcp/、internal/authz/、internal/storage/、internal/observability/。重点: Guardian超时5min+WS推送、MCP 9Tools+4Resources、Authz限流、MinIO签名URL(1h)+90天TTL、Metrics(cap_前缀)、Tracing(OTel Spans)。修复发现的问题。",
        "priority": "P2",
    },
    {
        "desc": "Review R20: plugins/gitclient+mcpserver+tools — 辅助插件",
        "detail": "审查 plugins/gitclient/、plugins/mcpserver/、plugins/tools/、plugins/plugins.go。重点: go-git安全(禁止push main)、MCP SSE传输、插件统一注册。修复发现的问题。",
        "priority": "P2",
    },
    {
        "desc": "Review R21: cmd/ — 入口与组装",
        "detail": "审查 cmd/server/main.go 和 cmd/mcp/main.go。重点: 依赖注入组装顺序、优雅关闭、信号处理。修复发现的问题。",
        "priority": "P2",
    },
    {
        "desc": "Review R22: 跨层集成验证 — 依赖方向+接口对齐+编译",
        "detail": "全项目审查。重点: 依赖方向L5→L3→L4→L2→L1无违反、核心层无插件import、interfaces.go与插件实现对齐、go build ./...通过、go test ./...通过。修复所有问题。",
        "priority": "P0",
    },
]

now = datetime.datetime.now().isoformat()

for i, rt in enumerate(review_tasks):
    n = next_num + i
    t_id = tid(n)

    task_entry = {
        "id": t_id,
        "description": rt["desc"],
        "template": "task",
        "priority": rt["priority"],
        "status": "pending",
        "parent_id": None,
        "output_req": "4k",
        "context_hint": "4k",
        "task_file": f"tasks/{t_id}.md",
        "created_at": now,
        "updated_at": now,
        "dependencies": [],
        "dependency_type": "all",
        "ralph": {
            "iteration": 0,
            "max_iterations": 3,
            "quality_checks": {
                "tests_passed": False,
                "lint_passed": False,
                "acceptance_met": False
            },
            "issues": [],
            "optimization_history": []
        }
    }

    data["tasks"].append(task_entry)

    task_md = f"""# {t_id}

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/{t_id}.md`（任务描述文件）
**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）
**产出物**: 修复后的代码文件
**这是配置文件**，不是最终产出！

## 描述

{rt["desc"]}

## 审查要求

{rt["detail"]}

## 架构参考文档
- agent.md — 架构约束+编码规则
- docs/architecture.md — 完整技术规范
- docs/Cloud-Agent-Platform.md — 业务设计

## 验证证据（完成前必填）

- [ ] **实现证明**: 列出发现的问题和修复内容
- [ ] **测试验证**: 运行相关测试，结果如何
- [ ] **影响范围**: 修复是否影响其他模块
"""
    with open(os.path.join(tasks_dir, f"{t_id}.md"), "w") as f:
        f.write(task_md)

    print(f"Created {t_id}: {rt['desc']}")

# R22 depends on all previous review tasks
r22_idx = next_num + len(review_tasks) - 1
for i in range(len(review_tasks) - 1):
    dep_id = tid(next_num + i)
    for t in data["tasks"]:
        if t["id"] == tid(r22_idx):
            t["dependencies"].append(dep_id)
            break

with open(task_list_path, "w") as f:
    json.dump(data, f, indent=2, ensure_ascii=False)

print(f"\nDone: {len(review_tasks)} review tasks created ({tid(next_num)} ~ {tid(r22_idx)})")
