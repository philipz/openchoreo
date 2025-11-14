# ClickStack Observability Operations Runbook

**Version**: 1.0
**Last Updated**: 2025-11-14
**Maintainers**: OpenChoreo Platform Team

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture Quick Reference](#architecture-quick-reference)
3. [Deployment Procedures](#deployment-procedures)
4. [Health Checks](#health-checks)
5. [Common Operations](#common-operations)
6. [Incident Response](#incident-response)
7. [Performance Tuning](#performance-tuning)
8. [Backup and Recovery](#backup-and-recovery)
9. [Scaling Procedures](#scaling-procedures)
10. [Monitoring and Alerting](#monitoring-and-alerting)

---

## Overview

This runbook provides operational procedures for managing the ClickStack observability platform in OpenChoreo. ClickStack replaces OpenSearch with a high-performance, cost-effective stack built on ClickHouse, providing unified logs, traces, and metrics collection via OpenTelemetry.

**Key Components**:
- **ClickStack (ClickHouse)**: Columnar database for telemetry storage
- **OTLP Gateway**: OpenTelemetry Collector for data ingestion
- **HyperDX**: Web UI for querying and visualization
- **Observer API**: RESTful API for programmatic access

**Deployment Model**: Kubernetes Helm chart in `openchoreo-observability-plane` namespace

---

## Architecture Quick Reference

```
┌─────────────────────────────────────────────────────────┐
│                  Application Pods                        │
│           (instrumented with OTLP SDK)                   │
└──────────────────────┬──────────────────────────────────┘
                       │ OTLP (gRPC:4317, HTTP:4318)
                       ↓
┌─────────────────────────────────────────────────────────┐
│          OTLP Gateway (otel-collector)                   │
│  • Receivers: OTLP gRPC/HTTP, FluentForward              │
│  • Processors: Batch, MemoryLimiter, Resource           │
│  • Exporters: ClickHouse                                 │
└──────────────────────┬──────────────────────────────────┘
                       │ ClickHouse Native (9000)
                       ↓
┌─────────────────────────────────────────────────────────┐
│              ClickStack StatefulSet                      │
│  • 3 replicas (standard) or 1 replica (minimal)          │
│  • Database: telemetry                                   │
│  • Tables: logs_mv, traces, metrics                      │
└──────────────────────┬──────────────────────────────────┘
                       │
        ┌──────────────┴───────────────┐
        ↓                              ↓
┌──────────────────┐          ┌────────────────────┐
│   HyperDX UI     │          │   Observer API     │
│   (Port 3000)    │          │   (Port 8080)      │
└──────────────────┘          └────────────────────┘
```

**Namespaces**:
- `openchoreo-observability-plane`: All observability components
- `dp-*`: Data plane applications (log sources)

---

## Deployment Procedures

### Initial Deployment

**Prerequisites**:
- Kubernetes cluster (1.24+)
- kubectl configured
- Helm 3.8+
- Persistent volume provisioner
- 16 CPU, 32GB RAM minimum (standard mode)

**Standard Deployment** (Production):

```bash
# 1. Create namespace
kubectl create namespace openchoreo-observability-plane

# 2. Generate credentials secret
kubectl create secret generic clickstack-credentials \
  -n openchoreo-observability-plane \
  --from-literal=username=admin \
  --from-literal=password=$(openssl rand -base64 32)

# 3. Install ClickStack
helm upgrade --install openchoreo-observability-plane \
  ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set clickstack.enabled=true \
  --set gateway.enabled=true \
  --set hyperdx.enabled=true \
  --set observer.enabled=true \
  --set global.installationMode=standard \
  --set clickstack.replicas.standard=3 \
  --set clickstack.storage.size=200Gi \
  --timeout 15m \
  --wait

# 4. Verify deployment
kubectl get pods -n openchoreo-observability-plane
```

**Expected Output**:
```
NAME                              READY   STATUS    RESTARTS   AGE
clickstack-0                      1/1     Running   0          5m
clickstack-1                      1/1     Running   0          5m
clickstack-2                      1/1     Running   0          5m
gateway-xxx-yyy                   1/1     Running   0          5m
hyperdx-xxx-yyy                   1/1     Running   0          5m
observer-xxx-yyy                  1/1     Running   0          5m
```

**Minimal Deployment** (Development/Testing):

```bash
helm upgrade --install openchoreo-observability-plane \
  ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set clickstack.enabled=true \
  --set global.installationMode=minimal \
  --timeout 10m \
  --wait
```

**Post-Deployment Validation**:

```bash
# 1. Check all pods are running
kubectl get pods -n openchoreo-observability-plane -w

# 2. Verify ClickStack health
kubectl exec clickstack-0 -n openchoreo-observability-plane -- \
  clickhouse-client -q "SELECT 1"
# Expected: 1

# 3. Check schema initialization
kubectl exec clickstack-0 -n openchoreo-observability-plane -- \
  clickhouse-client -q "SHOW TABLES FROM telemetry"
# Expected: logs_mv (and other tables)

# 4. Test Observer API
kubectl port-forward svc/observer 8080:8080 -n openchoreo-observability-plane &
curl http://localhost:8080/health
# Expected: {"status":"ok"}
```

---

## Health Checks

### Component Health Status

**ClickStack (ClickHouse)**:

```bash
# Check pod status
kubectl get pods -l app.kubernetes.io/component=clickstack \
  -n openchoreo-observability-plane

# Connect to ClickHouse
kubectl exec -it clickstack-0 -n openchoreo-observability-plane -- clickhouse-client

# Inside ClickHouse client:
SELECT 1;  -- Should return 1
SHOW DATABASES;
SELECT count() FROM telemetry.logs_mv;
SELECT uptime() AS uptime_seconds;
```

**OTLP Gateway**:

```bash
# Check gateway health
kubectl exec deployment/gateway -n openchoreo-observability-plane -- \
  curl -s localhost:13133/ | jq '.status'
# Expected: "Server available"

# View metrics endpoint
kubectl port-forward deployment/gateway 8888:8888 -n openchoreo-observability-plane &
curl localhost:8888/metrics | grep otelcol_receiver_accepted
```

**HyperDX**:

```bash
# Port forward to HyperDX
kubectl port-forward svc/hyperdx 3000:3000 -n openchoreo-observability-plane &

# Test health (HTTP 200)
curl -I http://localhost:3000/health
```

**Observer API**:

```bash
# Test health endpoint
kubectl port-forward svc/observer 8080:8080 -n openchoreo-observability-plane &
curl http://localhost:8080/health

# Test sample query
curl -X POST http://localhost:8080/api/logs/component/test \
  -H "Content-Type: application/json" \
  -d '{
    "startTime": "2025-01-01T00:00:00Z",
    "endTime": "2025-12-31T23:59:59Z",
    "limit": 1
  }'
```

### Automated Health Check Script

Save as `scripts/check-observability-health.sh`:

```bash
#!/bin/bash
set -e

NS="openchoreo-observability-plane"

echo "=== ClickStack Observability Health Check ==="
echo ""

# Check namespace exists
if ! kubectl get namespace "$NS" &>/dev/null; then
  echo "❌ Namespace $NS does not exist"
  exit 1
fi
echo "✓ Namespace exists"

# Check pods
PODS=$(kubectl get pods -n "$NS" --no-headers 2>/dev/null | wc -l)
RUNNING=$(kubectl get pods -n "$NS" --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)
echo "✓ Pods: $RUNNING/$PODS running"

if [ "$RUNNING" -lt 4 ]; then
  echo "⚠️  Warning: Expected at least 4 running pods"
fi

# Check ClickStack
if kubectl exec clickstack-0 -n "$NS" -- clickhouse-client -q "SELECT 1" &>/dev/null; then
  echo "✓ ClickStack responding"
else
  echo "❌ ClickStack not responding"
  exit 1
fi

# Check row count
ROWS=$(kubectl exec clickstack-0 -n "$NS" -- clickhouse-client -q "SELECT count() FROM telemetry.logs_mv" 2>/dev/null || echo "0")
echo "✓ Logs stored: $ROWS rows"

# Check gateway
if kubectl exec deployment/gateway -n "$NS" -- curl -sf localhost:13133/ &>/dev/null; then
  echo "✓ OTLP Gateway healthy"
else
  echo "⚠️  OTLP Gateway unreachable"
fi

echo ""
echo "=== Health Check Complete ==="
```

---

## Common Operations

### Querying Logs

**Via ClickHouse CLI**:

```bash
kubectl exec -it clickstack-0 -n openchoreo-observability-plane -- clickhouse-client

# Recent logs
SELECT timestamp, log_level, component_id, log
FROM telemetry.logs_mv
ORDER BY timestamp DESC
LIMIT 10;

# Error logs from last hour
SELECT timestamp, component_id, log
FROM telemetry.logs_mv
WHERE log_level = 'ERROR'
  AND timestamp >= NOW() - INTERVAL 1 HOUR
ORDER BY timestamp DESC;

# Log count by component
SELECT component_id, count() AS log_count
FROM telemetry.logs_mv
WHERE timestamp >= NOW() - INTERVAL 24 HOUR
GROUP BY component_id
ORDER BY log_count DESC;
```

**Via Observer API**:

```bash
# Port forward first
kubectl port-forward svc/observer 8080:8080 -n openchoreo-observability-plane &

# Get component logs
curl -X POST http://localhost:8080/api/logs/component/my-service \
  -H "Content-Type: application/json" \
  -d '{
    "startTime": "2025-07-01T00:00:00Z",
    "endTime": "2025-07-01T23:59:59Z",
    "environmentId": "production",
    "logLevels": ["ERROR", "WARN"],
    "limit": 100
  }' | jq .
```

### Restarting Components

**Restart OTLP Gateway** (safe, no data loss):

```bash
kubectl rollout restart deployment/gateway -n openchoreo-observability-plane
kubectl rollout status deployment/gateway -n openchoreo-observability-plane
```

**Restart Observer API**:

```bash
kubectl rollout restart deployment/observer -n openchoreo-observability-plane
kubectl rollout status deployment/observer -n openchoreo-observability-plane
```

**Restart HyperDX**:

```bash
kubectl rollout restart deployment/hyperdx -n openchoreo-observability-plane
kubectl rollout status deployment/hyperdx -n openchoreo-observability-plane
```

**Restart ClickStack Pod** (⚠️ Use with caution):

```bash
# Rolling restart (recommended for multi-replica)
kubectl delete pod clickstack-0 -n openchoreo-observability-plane
# Wait for pod to be recreated and become ready
kubectl wait --for=condition=ready pod/clickstack-0 -n openchoreo-observability-plane --timeout=5m

# Then proceed with next replica
kubectl delete pod clickstack-1 -n openchoreo-observability-plane
kubectl wait --for=condition=ready pod/clickstack-1 -n openchoreo-observability-plane --timeout=5m
```

### Clearing Old Data

**Delete logs older than 30 days**:

```bash
kubectl exec clickstack-0 -n openchoreo-observability-plane -- clickhouse-client -q "
ALTER TABLE telemetry.logs_mv DELETE
WHERE timestamp < NOW() - INTERVAL 30 DAY
"
```

**Optimize tables** (reclaim space):

```bash
kubectl exec clickstack-0 -n openchoreo-observability-plane -- clickhouse-client -q "
OPTIMIZE TABLE telemetry.logs_mv FINAL
"
```

---

## Incident Response

### Incident 1: No Logs Appearing in HyperDX

**Symptoms**: HyperDX dashboard shows no logs

**Diagnosis**:

```bash
# 1. Check if applications are sending logs
kubectl logs -n dp-production-app pod/my-app-xxx --tail=20
# Verify logs are being written

# 2. Check OTLP Gateway is receiving data
kubectl logs deployment/gateway -n openchoreo-observability-plane --tail=100 | grep -i error

# 3. Check gateway metrics
kubectl port-forward deployment/gateway 8888:8888 -n openchoreo-observability-plane &
curl localhost:8888/metrics | grep otelcol_receiver_accepted_log_records
# Should show increasing count

# 4. Check if ClickStack is receiving writes
kubectl exec clickstack-0 -n openchoreo-observability-plane -- clickhouse-client -q "
SELECT count() FROM telemetry.logs_mv WHERE timestamp >= NOW() - INTERVAL 5 MINUTE
"
```

**Resolution**:

If gateway is NOT receiving logs:
```bash
# Check collector configuration
kubectl get configmap gateway-config -n openchoreo-observability-plane -o yaml

# Restart gateway
kubectl rollout restart deployment/gateway -n openchoreo-observability-plane
```

If ClickStack is NOT receiving writes:
```bash
# Check ClickStack connectivity
kubectl exec deployment/gateway -n openchoreo-observability-plane -- \
  nc -zv clickstack 9000

# Restart ClickStack StatefulSet
kubectl delete pod clickstack-0 -n openchoreo-observability-plane
```

### Incident 2: ClickStack Out of Disk Space

**Symptoms**: Pods show disk pressure, writes failing

**Diagnosis**:

```bash
# Check PVC usage
kubectl exec clickstack-0 -n openchoreo-observability-plane -- df -h /var/lib/clickhouse

# Check table sizes
kubectl exec clickstack-0 -n openchoreo-observability-plane -- clickhouse-client -q "
SELECT
    table,
    formatReadableSize(sum(bytes)) AS size
FROM system.parts
WHERE database = 'telemetry'
GROUP BY table
ORDER BY sum(bytes) DESC
"
```

**Resolution**:

**Option 1: Delete old data**:
```bash
kubectl exec clickstack-0 -n openchoreo-observability-plane -- clickhouse-client -q "
ALTER TABLE telemetry.logs_mv DELETE WHERE timestamp < NOW() - INTERVAL 7 DAY
"
```

**Option 2: Expand PVC** (if storage class supports it):
```bash
# Edit PVC
kubectl edit pvc clickstack-data-clickstack-0 -n openchoreo-observability-plane
# Increase size field

# Restart pod to pick up change
kubectl delete pod clickstack-0 -n openchoreo-observability-plane
```

**Option 3: Reduce retention**:
```bash
kubectl exec clickstack-0 -n openchoreo-observability-plane -- clickhouse-client -q "
ALTER TABLE telemetry.logs_mv MODIFY TTL timestamp + INTERVAL 7 DAY
"
```

### Incident 3: High Query Latency

**Symptoms**: HyperDX or Observer API queries taking >5 seconds

**Diagnosis**:

```bash
# Check slow queries
kubectl exec clickstack-0 -n openchoreo-observability-plane -- clickhouse-client -q "
SELECT
    query_duration_ms,
    query,
    event_time
FROM system.query_log
WHERE type = 'QueryFinish'
  AND query_duration_ms > 1000
ORDER BY query_duration_ms DESC
LIMIT 10
"

# Check CPU/memory usage
kubectl top pods -n openchoreo-observability-plane
```

**Resolution**:

```bash
# Increase ClickStack resources
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --reuse-values \
  --set clickstack.resources.limits.cpu=4000m \
  --set clickstack.resources.limits.memory=12Gi
```

### Incident 4: ClickStack Pod CrashLoopBackOff

**Symptoms**: Pod repeatedly restarting

**Diagnosis**:

```bash
# View pod events
kubectl describe pod clickstack-0 -n openchoreo-observability-plane

# View logs
kubectl logs clickstack-0 -n openchoreo-observability-plane --previous

# Check for common issues:
# - Insufficient memory (OOMKilled)
# - Corrupted data directory
# - Configuration errors
```

**Resolution**:

If corrupted data:
```bash
# ⚠️ DESTRUCTIVE - Only if data is backed up
kubectl scale statefulset clickstack --replicas=0 -n openchoreo-observability-plane
kubectl delete pvc clickstack-data-clickstack-0 -n openchoreo-observability-plane
kubectl scale statefulset clickstack --replicas=3 -n openchoreo-observability-plane
```

If configuration error:
```bash
# Review and fix values
helm get values openchoreo-observability-plane -n openchoreo-observability-plane

# Apply corrected configuration
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --values corrected-values.yaml
```

---

## Performance Tuning

### ClickStack Query Optimization

**Indexes**:

```sql
-- Check existing indexes
SELECT table, name, type
FROM system.data_skipping_indices
WHERE database = 'telemetry';

-- Add index for component_id (if missing)
ALTER TABLE telemetry.logs_mv
ADD INDEX idx_component component_id TYPE bloom_filter(0.01) GRANULARITY 1;

-- Materialize index
ALTER TABLE telemetry.logs_mv MATERIALIZE INDEX idx_component;
```

**Query Best Practices**:

```sql
-- ✓ GOOD: Use time range filters
SELECT * FROM telemetry.logs_mv
WHERE timestamp BETWEEN '2025-07-01' AND '2025-07-02'
  AND component_id = 'my-service'
LIMIT 1000;

-- ✗ BAD: Full table scan
SELECT * FROM telemetry.logs_mv
WHERE component_id = 'my-service'
LIMIT 1000;
```

### OTLP Gateway Tuning

**Increase batch size** (for high throughput):

```yaml
# values.yaml
gateway:
  config:
    processors:
      batch:
        timeout: 10s
        send_batch_size: 200000  # Increase from default 100k
```

**Increase memory limit**:

```yaml
gateway:
  resources:
    limits:
      memory: 1Gi  # Increase from 512Mi
  config:
    processors:
      memory_limiter:
        limit_mib: 900
```

---

## Backup and Recovery

### Backup ClickStack Data

**Using ClickHouse Backup Tool**:

```bash
# Install clickhouse-backup in pod
kubectl exec -it clickstack-0 -n openchoreo-observability-plane -- bash
apt-get update && apt-get install -y clickhouse-backup

# Create backup
clickhouse-backup create my-backup-$(date +%Y%m%d)

# List backups
clickhouse-backup list

# Upload to S3 (configured in clickhouse-backup config)
clickhouse-backup upload my-backup-20250714
```

**Manual Snapshot** (for testing):

```bash
# Freeze table (creates hard links, no space used)
kubectl exec clickstack-0 -n openchoreo-observability-plane -- clickhouse-client -q "
ALTER TABLE telemetry.logs_mv FREEZE WITH NAME 'backup-20250714'
"

# Backup is in /var/lib/clickhouse/shadow/backup-20250714/
```

### Restore from Backup

```bash
# Download backup from S3
clickhouse-backup download my-backup-20250714

# Restore
clickhouse-backup restore --schema --data my-backup-20250714
```

---

## Scaling Procedures

### Horizontal Scaling (Add Replicas)

```bash
# Increase ClickStack replicas (standard mode)
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --reuse-values \
  --set clickstack.replicas.standard=5

# Verify new pods
kubectl get pods -l app.kubernetes.io/component=clickstack -n openchoreo-observability-plane -w
```

### Vertical Scaling (Increase Resources)

```bash
# Increase CPU/memory
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --reuse-values \
  --set clickstack.resources.limits.cpu=4000m \
  --set clickstack.resources.limits.memory=16Gi

# Pods will restart automatically
kubectl get pods -n openchoreo-observability-plane -w
```

---

## Monitoring and Alerting

### Key Metrics to Monitor

**ClickStack**:
- Disk usage: `clickhouse_filesystem_available_bytes`
- Query duration: `clickhouse_query_duration_seconds`
- Ingestion rate: `clickhouse_insert_rows_per_second`
- Connection count: `clickhouse_tcp_connection_count`

**OTLP Gateway**:
- Logs received: `otelcol_receiver_accepted_log_records`
- Logs sent: `otelcol_exporter_sent_log_records`
- Failed sends: `otelcol_exporter_send_failed_log_records`
- Queue size: `otelcol_exporter_queue_size`

**Sample Prometheus Alerts**:

```yaml
groups:
  - name: clickstack_alerts
    rules:
      - alert: ClickStackDown
        expr: up{job="clickstack"} == 0
        for: 2m
        annotations:
          summary: "ClickStack instance is down"

      - alert: ClickStackDiskSpaceLow
        expr: (clickhouse_filesystem_available_bytes / clickhouse_filesystem_size_bytes) < 0.15
        for: 10m
        annotations:
          summary: "ClickStack disk space below 15%"

      - alert: OTLPGatewayHighDropRate
        expr: rate(otelcol_exporter_send_failed_log_records[5m]) > 100
        for: 5m
        annotations:
          summary: "OTLP Gateway dropping logs"
```

---

## Appendix

### Useful Commands Reference

```bash
# Quick pod status
kubectl get pods -n openchoreo-observability-plane -o wide

# Tail all ClickStack logs
kubectl logs -f statefulset/clickstack -n openchoreo-observability-plane --all-containers

# Execute SQL query
kubectl exec clickstack-0 -n openchoreo-observability-plane -- clickhouse-client -q "YOUR_QUERY"

# Port forward all services
kubectl port-forward svc/hyperdx 3000:3000 -n openchoreo-observability-plane &
kubectl port-forward svc/observer 8080:8080 -n openchoreo-observability-plane &
kubectl port-forward svc/gateway 4317:4317 -n openchoreo-observability-plane &
```

### Configuration Files

**Helm Values** (production example):

```yaml
global:
  installationMode: standard

clickstack:
  enabled: true
  replicas:
    standard: 3
  storage:
    size: 500Gi
  resources:
    limits:
      cpu: 4000m
      memory: 16Gi
    requests:
      cpu: 1000m
      memory: 4Gi

gateway:
  enabled: true
  replicas: 3
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi

hyperdx:
  enabled: true
  replicas: 2

observer:
  enabled: true
  replicas: 2
  telemetry:
    backend: clickstack
```

---

**Document End**

For additional support:
- GitHub Issues: https://github.com/openchoreo/openchoreo/issues
- Documentation: https://openchoreo.dev/docs/observability
- Slack: #openchoreo-support
