# ClickStack Migration & Cost Reporting Runbook

This runbook guides platform teams through migrating from the legacy OpenSearch plane to ClickStack, validating ingestion health, enabling dual-write cutovers, and producing monthly cost exports.

## Prerequisites
- Access to the OpenChoreo repo and cluster kubectl context.
- Helm 3.14+, kubectl 1.29+, and `make deploy-observability` available.
- ClickStack credentials + HyperDX signing secret stored securely (e.g., external secret).

## 1. Deploy ClickStack Plane
1. Set values for `clickstack.credentials.*`, `gateway.*`, `hyperdx.*`, and `monitoring.*`.
2. Install/upgrade:
   ```bash
   helm upgrade --install openchoreo-observability-plane \
     ./install/helm/openchoreo-observability-plane \
     --namespace openchoreo-observability-plane \
     --create-namespace
   ```
3. Validate pods:
   ```bash
   kubectl get pods -n openchoreo-observability-plane
   ```

## 2. Enable Dual-Write (Shadow Phase)
1. In `values.yaml`, keep `observer.telemetry.dualRead=true`.
2. Run `make deploy-observability` to install OTLP collectors on every node.
3. Monitor dashboards/alerts to ensure ingestion lag stays <3s and compression ratio >10.

## 3. Verification Checklist
| Check | Command |
| --- | --- |
| ClickStack pods ready | `kubectl get sts,deploy -n openchoreo-observability-plane` |
| Collector coverage | `kubectl get ds otel-collector -n openchoreo-observability-plane` |
| Grafana dashboards present | `kubectl get cm -n openchoreo-observability-plane -l grafana_dashboard=1` |
| Prometheus rules loaded | `kubectl get prometheusrule -n openchoreo-observability-plane` |
| HyperDX reachable | `kubectl port-forward deploy/hyperdx 3000:3000 -n openchoreo-observability-plane` |

## 4. Cost Export
1. Port-forward the observer service:
   ```bash
   kubectl port-forward svc/observer 8080 -n openchoreo-observability-plane
   ```
2. Download monthly CSV:
   ```bash
   curl "http://127.0.0.1:8080/api/costs/export?month=2025-11" \
        -H "Authorization: Bearer <token>" \
        -o clickstack-cost-2025-11.csv
   ```
3. Upload the CSV to your FinOps tooling or share with finance stakeholders.

## 5. Cutover & Rollback
1. Once data matches (<1% drift), set `observer.telemetry.backend=clickstack` and disable OpenSearch pods (`openSearch.enabled=false`).
2. If issues arise, flip the backend back to `opensearch` and redeploy; collectors and dashboards remain intact.

## 6. Post-Migration Operations
- Keep `make deploy-observability` in CI to ensure collectors stay applied.
- Add Grafana alerts to on-call rotations (ingestion lag, compression, query p95).
- Run E2E ClickStack tests with `E2E_CLICKSTACK=true go test ./test/e2e -v` before every release. Under this flag the manager specs skip automatically; the ClickStack suite installs the observability plane Helm chart (minimal mode), validates component readiness, sends synthetic OTLP logs through the gateway, confirms ClickStack queries succeed, and restarts the StatefulSet to verify failover.
