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
| `openSearchCluster.*` | Legacy OpenSearch automation toggles (default disabled) |
| `collectors.*` | Node-level OTLP collectors (enabled by default) |
| `monitoring.*` | Prometheus alerts and Grafana dashboards for ClickStack metrics/cost |

### Local testing helper
Spin up (or reuse) the Kind cluster via `make kind` and then run:
```bash
make kind.deploy-observability-plane CLICKSTACK_PASSWORD=<strong-password>
```
The helper now:

- Ensures Cilium is present and reuses the KinD control-plane DNS/CNI tweaks from `install/dev/kind.sh` (no more CoreDNS deletion).
- Deploys the Helm chart in `minimal` mode with **HyperDX + MongoDB enabled by default**. A lightweight MongoDB StatefulSet (`hyperdx-mongodb`) is installed for local runs; provide `HYPERDX_ENABLED=false` if you want to skip the UI entirely.
- Forces `observer.telemetry.dualRead=false` and `openSearch.*=false` so the observer never attempts to resolve the legacy OpenSearch service.
- Prints a summary with ready checks plus the commands for port-forwarding HyperDX (`kubectl port-forward -n openchoreo-observability-plane svc/hyperdx 3000:3000`), viewing OTLP gateway logs, and checking MongoDB.

Environment knobs:

| Variable | Default | Description |
| --- | --- | --- |
| `CLICKSTACK_PASSWORD` | `KindClickStackP@55w0rd` | Admin password for the ClickHouse cluster |
| `HYPERDX_ENABLED` | `true` | Toggle HyperDX + MongoDB deployment |
| `HYPERDX_SIGNING_KEY` | `kind-hdx-signing-secret` | Secret for `/api/hyperdx/link` signatures |
| `HELM_CACHE_HOME` et al. | `/tmp/helm*` | Override Helm cache directories when sandboxed |

The legacy `make deploy-observability` target still applies only the node-level OTLP collectors and is kept for backwards compatibility tests.

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

## 6. Inspect Greeter telemetry in HyperDX

After following [Deploy your first component](https://openchoreo.dev/docs/getting-started/deploy-first-component/) the Greeter workload emits OTLP spans/logs. Use these steps to confirm the data lands in ClickStack + HyperDX:

1. **Confirm collectors and gateway see Greeter traffic**
   ```bash
   kubectl logs -n openchoreo-observability-plane deploy/otlp-gateway | grep greeter || true
   kubectl logs -n openchoreo-observability-plane ds/otel-collector | grep greeter || true
   ```
   A healthy pipeline shows HTTP 200 ingest logs plus `k8s.namespace.name=dp-...-greeter`. If nothing appears, check that the Greeter workload exports OTLP to `otlp-gateway.openchoreo-observability-plane.svc.cluster.local:4317`.

2. **Port-forward HyperDX** (if not already running via the Kind helper summary):
   ```bash
   kubectl port-forward -n openchoreo-observability-plane svc/hyperdx 3000:3000
   open http://localhost:3000
   ```

3. **View Logs** – In HyperDX, open *Search → Logs* and add filters:
   - `resourceAttributes.k8s.namespace.name` equals your Greeter namespace (`dp-default-default-development-...`).
   - `service.name` equals `greeter` (or the OTLP service name you configured).
   The collector changes now preserve the original log body, so you should see the `message` and `attributes.component.id` fields inline.

4. **View Traces** – Switch to *Traces* and filter by `service.name=greeter`. The OTLP gateway automatically writes spans into `telemetry.traces_mv`; HyperDX surfaces them through the default trace explorer.

5. **Metrics / Health** – Use Grafana (`config/observability/grafana/dashboards`) or click `Alerts` inside HyperDX to confirm ingestion lag < 3s and that ClickStack’s RED metrics stay green.

6. **Live troubleshooting** – For quick CLI checks you can query ClickHouse directly:
   ```bash
   kubectl exec -n openchoreo-observability-plane statefulset/clickstack -- \
     clickhouse-client --query "SELECT count() FROM telemetry.logs_mv WHERE namespace='dp-default-default-development' AND service='greeter' AND timestamp > now() - INTERVAL 15 MINUTE"
   ```
   To automate the health sweep, run `scripts/verify-hyperdx.sh --data-namespace dp-default-default-development` – it inspects pod status, OTLP gateway logs, and optional ClickHouse counts in a single command.

## 7. Migration Checklist (OpenSearch → ClickStack)

1. Deploy ClickStack plane (Section 1).
2. Enable dual-write in observer via `observer.telemetry.dualRead=true` until parity confirmed.
3. Run `make deploy-observability` to confirm collectors run on each node.
4. Verify Grafana dashboards and Prometheus alerts.
5. Use `/api/costs/export` to validate billing data before disabling OpenSearch.

Runbook details live in `docs/runbooks/clickstack-migration.md`.
