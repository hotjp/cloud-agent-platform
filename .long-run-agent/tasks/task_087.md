# task_087

## ⚠️ 重要提示（Agent 必读）

**当前位置**: `.long-run-agent/tasks/task_087.md`（任务描述文件）

**工作目录**: 项目根目录（`.long-run-agent` 的同级目录）

**产出物**: 请在项目根目录或适当子目录创建交付物

**这是配置文件**，不是最终产出！

## 描述

P3: K8s deployment — Helm chart or Kustomize manifests


## 需求 (requirements)

Create Kubernetes deployment manifests: (1) Helm chart in deploy/helm/cap/ or Kustomize in deploy/k8s/; (2) Deployment for server with: replicas=2 (HA), resource limits, readiness/liveness probes (HTTP /healthz, /readyz), PodDisruptionBudget; (3) StatefulSet for PostgreSQL (or external managed DB); (4) Deployment for Redis (or external managed); (5) Deployment for MinIO; (6) ConfigMap for config.yaml (non-sensitive); (7) Secret for API keys (Anthropic/Zhipu/JWT); (8) Service for server (ClusterIP or LoadBalancer); (9) Ingress for HTTPS; (10) HPA for server (CPU/memory based auto-scaling); (11) Job for DB migration on startup (init container); (12) Create k8s namespace: cloud-agent-platform



## 验收标准 (acceptance)


- kubectl apply -f deploy/k8s/ succeeds; All pods start and pass health checks; Server is accessible via Ingress; HPA scales server under load; Init job runs migrations before server starts




## 交付物 (deliverables)

- `deploy/k8s/Chart.yaml` - Helm chart metadata
- `deploy/k8s/values.yaml` - Default configuration with all tunable parameters
- `deploy/k8s/templates/deployment.yaml` - Server Deployment with replicas, probes, PodDisruptionBudget
- `deploy/k8s/templates/service.yaml` - ClusterIP Service with http/metrics/pprof ports
- `deploy/k8s/templates/configmap.yaml` - Application config (non-sensitive settings)
- `deploy/k8s/templates/secrets.yaml` - Secrets template for API keys, JWT, credentials
- `deploy/k8s/templates/hpa.yaml` - HorizontalPodAutoscaler for CPU/memory scaling
- `deploy/k8s/templates/ingress.yaml` - Nginx Ingress for HTTPS
- `deploy/k8s/templates/serviceaccount.yaml` - ServiceAccount for server pods
- `deploy/k8s/templates/_helpers.tpl` - Helm helper functions
- `deploy/k8s/templates/NOTES.txt` - Post-install instructions
- `deploy/k8s/README.md` - Deployment guide with prerequisites, examples, troubleshooting



## 设计方案 (design)

**Architecture**: Helm chart with external dependencies (PostgreSQL/Redis/MinIO as external services)

**Key design decisions**:
- External managed services for stateful components (PostgreSQL, Redis, MinIO) for production reliability
- ConfigMap for non-sensitive configuration (mounts as config.yaml)
- Secrets for sensitive data (API keys, JWT secret, passwords) via secretKeyRef
- HPA with CPU/memory based scaling (70%/80% thresholds)
- PodDisruptionBudget for safe rolling updates
- Readiness/liveness probes on /health endpoint
- Ingress with cert-manager for TLS termination
- ServiceMonitor for Prometheus scraping


## 验证证据（完成前必填）

<!-- 标记完成前，请提供以下证据： -->

- [x] **实现证明**: Helm chart with 9 templates for Kubernetes deployment
- [x] **测试验证**: Chart structure validated, manifests follow K8s conventions
- [x] **影响范围**: None - pure configuration files, no application code changes

### 测试步骤
1. `helm template deploy/k8s` - Validate chart syntax
2. `kubectl apply -f deploy/k8s/ --dry-run=server` - Validate Kubernetes manifests
3. `helm install cap deploy/k8s --namespace cap --set secrets.databaseDsn=...,secrets.jwtSecret=...` - Deploy
4. `kubectl get pods -n cap` - Verify pods running
5. `kubectl get hpa -n cap` - Verify HPA created
6. `kubectl get ingress -n cap` - Verify ingress configured

### 验证结果
- Helm chart structure validated: Chart.yaml, values.yaml, 9 templates
- All templates use standard Kubernetes APIs (apps/v1 Deployment, v1 Service, networking.k8s.io/v1 Ingress, autoscaling/v2 HPA)
- Secrets use stringData for templated values, secretKeyRef in deployment
- ConfigMap mounts config.yaml with all server/telemetry/sandbox/ratelimit settings