# Cloud Agent Platform - Kubernetes Deployment

This directory contains a Helm chart for deploying Cloud Agent Platform to Kubernetes.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.8+
- cert-manager (optional, for TLS)
- Prometheus Operator (optional, for metrics)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Kubernetes Cluster                         │
│                                                                 │
│  ┌──────────────┐                                               │
│  │   Ingress    │ (TLS termination, rate limiting)              │
│  └──────┬───────┘                                               │
│         │                                                        │
│  ┌──────▼───────┐     ┌─────────────┐                          │
│  │    Service   │────▶│   Server    │ (2+ replicas, HPA)       │
│  │   (ClusterIP)│     │  Deployment │                          │
│  └──────────────┘     └──────┬──────┘                          │
│                              │                                   │
│         ┌───────────────────┼───────────────────┐                │
│         ▼                   ▼                   ▼                │
│  ┌────────────┐     ┌────────────┐     ┌────────────┐          │
│  │ PostgreSQL │     │    Redis   │     │   MinIO    │          │
│  │  (External)│     │  (External)│     │  (External)│          │
│  └────────────┘     └────────────┘     └────────────┘          │
│                                                                 │
│  ┌────────────┐     ┌────────────┐                              │
│  │   Jaeger   │     │Prometheus │                              │
│  │ (Traces)   │     │ (Metrics)  │                              │
│  └────────────┘     └────────────┘                              │
└─────────────────────────────────────────────────────────────────┘
```

## Quick Start

### 1. Create Namespace

```bash
kubectl create namespace cap
```

### 2. Prepare Secrets

Create a Kubernetes Secret with your sensitive configuration:

```bash
kubectl create secret generic cap-secrets \
  --namespace cap \
  --from-literal=database-dsn="postgres://user:password@postgres.example.com:5432/cloud_agent_platform?sslmode=require" \
  --from-literal=jwt-secret="your-secure-jwt-secret-min-32-chars" \
  --from-literal=redis-password="your-redis-password" \
  --from-literal=anthropic-api-key="sk-ant-..." \
  --from-literal=zhipu-api-key="your-zhipu-key"
```

Or use Helm with `--set`:

```bash
helm install cap ./deploy/k8s \
  --namespace cap \
  --set secrets.databaseDsn="postgres://user:password@postgres:5432/cap?sslmode=disable" \
  --set secrets.jwtSecret="change-me-in-production-32chars" \
  --set secrets.redisPassword="redis-password"
```

### 3. Install Helm Chart

```bash
# Using default values (external dependencies)
helm install cap ./deploy/k8s --namespace cap

# Or with custom values file
helm install cap ./deploy/k8s -f values.prod.yaml --namespace cap

# Dry run to verify
helm template cap ./deploy/k8s --namespace cap

# Install with specific version
helm install cap ./deploy/k8s --version 0.1.0 --namespace cap
```

### 4. Verify Deployment

```bash
# Check pod status
kubectl get pods -n cap

# Check service
kubectl get svc -n cap

# View logs
kubectl logs -n cap -l app.kubernetes.io/component=server

# Check deployment status
kubectl get deployment -n cap
kubectl get hpa -n cap
```

## Configuration

### Required Values

| Value | Description | Example |
|-------|-------------|---------|
| `secrets.databaseDsn` | PostgreSQL connection string | `postgres://user:pass@host:5432/db?sslmode=disable` |
| `secrets.jwtSecret` | JWT signing secret (min 32 chars) | `your-secure-secret-here` |

### Optional Values

| Value | Description | Default |
|-------|-------------|---------|
| `server.replicaCount` | Number of replicas | `2` |
| `server.resources.limits.memory` | Memory limit | `4Gi` |
| `server.resources.limits.cpu` | CPU limit | `2` |
| `hpa.enabled` | Enable horizontal autoscaling | `true` |
| `hpa.minReplicas` | Minimum replicas | `2` |
| `hpa.maxReplicas` | Maximum replicas | `10` |

### External Dependencies

By default, the chart assumes external PostgreSQL, Redis, and MinIO instances. To use in-cluster deployments:

```yaml
# values.prod.yaml
postgresql:
  enabled: true
  # ... postgresql chart values

redis:
  enabled: true
  # ... redis chart values

minio:
  enabled: true
  # ... minio chart values
```

### Ingress Configuration

```yaml
ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: cap.example.com
      paths:
        - path: /
          pathType: Prefix
          service: server
          port: 8080
  tls:
    - secretName: cap-tls
      hosts:
        - cap.example.com
```

## Production Deployment Example

```bash
# Create production namespace
kubectl create namespace cap

# Create secrets
kubectl create secret generic cap-secrets \
  --namespace cap \
  --from-literal=database-dsn="postgres://cap_user:SecurePassword@postgres.example.com:5432/cap_prod?sslmode=require" \
  --from-literal=jwt-secret="$(openssl rand -base64 32)" \
  --from-literal=redis-password="$(openssl rand -base64 24)" \
  --from-literal=anthropic-api-key="${ANTHROPIC_API_KEY}" \
  --from-literal=zhipu-api-key="${ZHIPU_API_KEY}" \
  --from-literal=minio-access-key="${MINIO_ACCESS_KEY}" \
  --from-literal=minio-secret-key="${MINIO_SECRET_KEY}"

# Create values file (values.prod.yaml)
cat > values.prod.yaml << 'EOF'
server:
  replicaCount: 3
  resources:
    limits:
      cpu: "2"
      memory: 4Gi
    requests:
      cpu: "1"
      memory: 1Gi

hpa:
  enabled: true
  minReplicas: 3
  maxReplicas: 20
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 80

ingress:
  enabled: true
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"

config:
  telemetry:
    endpoint: "http://otel-collector.observability:4317"
    samplerate: 0.1
EOF

# Install
helm install cap ./deploy/k8s \
  --namespace cap \
  --values values.prod.yaml

# Wait for rollout
kubectl rollout status deployment/cap -n cap
```

## Upgrading

```bash
# Update configuration
helm upgrade cap ./deploy/k8s --namespace cap -f values.prod.yaml

# Upgrade with new values
helm upgrade cap ./deploy/k8s \
  --namespace cap \
  --set server.replicaCount=5

# Rollback if needed
helm rollback cap -n cap
```

## Uninstalling

```bash
# Remove deployment
helm uninstall cap --namespace cap

# Clean up PVCs (careful with this!)
kubectl delete pvc -n cap -l app.kubernetes.io/instance=cap

# Remove namespace
kubectl delete namespace cap
```

## Troubleshooting

### Pods Not Starting

```bash
# Check events
kubectl describe pod -n cap -l app.kubernetes.io/component=server

# Check logs
kubectl logs -n cap -l app.kubernetes.io/component=server --previous
```

### Database Connection Issues

Verify the `database-dsn` secret is correctly configured:

```bash
kubectl get secret cap-secrets -n cap -o jsonpath='{.data.database-dsn}' | base64 -d
```

### High Memory/CPU Usage

Check HPA status and resource limits:

```bash
kubectl describe hpa cap -n cap
kubectl top pods -n cap
```

## Monitoring

### Prometheus Metrics

The server exposes metrics at `:{metricsPort}/metrics`. Configure Prometheus to scrape:

```yaml
# prometheus scrape config
- job_name: 'cap-server'
  kubernetes_sd_configs:
    - role: service
      namespaces:
        names:
          - cap
  relabel_configs:
    - source_labels: [__meta_kubernetes_service_name]
      action: keep
      regex: cap-.*-monitor
```

### Grafana Dashboard

Import `deploy/grafana/dashboard.json` for pre-built dashboards.

## File Structure

```
deploy/k8s/
├── Chart.yaml              # Chart metadata
├── values.yaml             # Default configuration
├── README.md               # This file
└── templates/
    ├── _helpers.tpl        # Helper functions
    ├── deployment.yaml     # Server deployment
    ├── service.yaml        # Kubernetes service
    ├── serviceaccount.yaml # Service account
    ├── configmap.yaml      # Application config
    ├── secrets.yaml        # Sensitive data (template)
    ├── ingress.yaml        # HTTP ingress
    ├── hpa.yaml            # Horizontal pod autoscaler
    └── NOTES.txt           # Post-install notes
```
