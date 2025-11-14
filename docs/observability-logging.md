# ClickStack Observability Guide

OpenChoreo’s observability plane now runs on **ClickStack** (ClickHouse + HyperDX + OTLP Gateway). ClickStack replaces OpenSearch and Fluent Bit while keeping the same API contracts your developers rely on. This guide covers how to deploy the new stack, validate it on a Kind cluster, and surface data in Grafana/HyperDX.

> **Key Benefits**
> - 10‑30x faster queries and <1s p95 for 24h log windows
> - 70‑90% lower storage cost with columnar compression
> - Unified logs/traces/metrics via OpenTelemetry

---

## 1. Deploy the Observability Plane

### Helm deployment (recommended)
```bash
helm upgrade --install openchoreo-observability-plane \
  ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --create-namespace \
  --set clickstack.credentials.password="<strong-password>" \
  --set hyperdx.signing.key="<32-byte-secret>"
```

Important values:

| Value | Purpose |
| --- | --- |
| `clickstack.*` | ClickHouse StatefulSet sizing, TLS, credentials |
| `gateway.*` | OTLP gateway (OpenTelemetry Collector) ingress |
| `hyperdx.*` | HyperDX UI configuration + signing secret |
| `collectors.*` | Node-level OTLP collectors (enabled by default) |
| `monitoring.*` | Prometheus alerts and Grafana dashboards for ClickStack metrics/cost |

### Local testing helper
For quick validation against a Kind cluster with `kubectl` access:
```bash
make deploy-observability
```
This command applies the reusable OTLP collector kustomization located at `config/observability/collectors/otel/base`.

---

## 2. Verify the Installation

```bash
kubectl get pods -n openchoreo-observability-plane
kubectl get daemonset otel-collector -n openchoreo-observability-plane
kubectl get configmap -n openchoreo-observability-plane -l grafana_dashboard=1
```

You should see:
- `clickstack-0..N` StatefulSet pods
- `otlp-gateway` deployment
- `hyperdx` deployment
- `otel-collector` DaemonSet with READY count equal to desired node count
- Grafana dashboard ConfigMaps labeled `grafana_dashboard=1`

Prometheus rules and alerts are installed automatically when `prometheus.enabled=true`. Review them with:
```bash
kubectl get prometheusrule -n openchoreo-observability-plane
```

---

## 3. Viewing Dashboards & Alerts

### Grafana
1. Expose Grafana (from your existing monitoring stack) or import the dashboard JSON files in `config/observability/grafana/dashboards/`.
2. Key panels:
   - **ClickStack Overview**: ingestion throughput, query p95, compression ratio, storage cost
   - **ClickStack Cost & Health**: cost per project, ingestion lag, query spend

### HyperDX
```bash
kubectl port-forward deploy/hyperdx 3000:3000 -n openchoreo-observability-plane
open http://localhost:3000
```
Use the `/api/hyperdx/link` endpoint to generate signed embeds for Backstage widgets.

### Prometheus Alerts
Alerts fire for:
- Ingestion lag > 3s (`ClickStackIngestionLagHigh`)
- Query p95 > 1s (`ClickStackQueryLatencyHigh`)
- Compression ratio < 10 (`ClickStackCompressionLow`)

Adjust thresholds in `values.yaml -> monitoring.alerts`.

---

## 4. E2E / CI Validation

Set `E2E_CLICKSTACK=true` before running `go test ./test/e2e -v` to execute the ClickStack integration suite (manager specs auto-skip under this flag). The suite now:
- Installs the `openchoreo-observability-plane` Helm chart in minimal mode and waits for ClickStack, OTLP gateway, Observer, HyperDX, and collector DaemonSets to become ready.
- Verifies Grafana dashboards ConfigMaps are published.
- Sends a synthetic OTLP/HTTP log through the gateway, then queries ClickStack to confirm the record is indexed.
- Deletes the `clickstack-0` pod and ensures the StatefulSet recreates it without losing the ingested record.

Without the env var the test automatically skips.

---

## 5. Cost Export API

FinOps teams can pull monthly CSV reports:
```bash
kubectl port-forward svc/observer 8080 -n openchoreo-observability-plane
curl "http://127.0.0.1:8080/api/costs/export?month=2025-11" \
  -H "Authorization: Bearer <token>" \
  -o clickstack-cost-2025-11.csv
```
Behind the scenes the observer service aggregates telemetry using ClickStack SQL and applies the cost coefficients defined in `monitoring.cost`.

---

## 6. Migration Checklist (OpenSearch → ClickStack)

1. Deploy ClickStack plane (Section 1).
2. Enable dual-write in observer via `observer.telemetry.dualRead=true` until parity confirmed.
3. Run `make deploy-observability` to confirm collectors run on each node.
4. Verify Grafana dashboards and Prometheus alerts.
5. Use `/api/costs/export` to validate billing data before disabling OpenSearch.

Runbook details live in `docs/runbooks/clickstack-migration.md`.
