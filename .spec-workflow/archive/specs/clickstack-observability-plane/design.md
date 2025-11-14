# Design Document

## Overview
We will refactor the `install/helm/openchoreo-observability-plane` chart and supporting services so that it deploys ClickStack (ClickHouse + HyperDX UI + OTLP gateway) instead of OpenSearch. The migration introduces a reusable telemetry module inside the Go control plane that abstracts storage providers, updates the Observer API to issue ClickHouse SQL, and adds OTLP collector assets that enrich data with OpenChoreo metadata. The work follows the architecture blueprint described in `OpenSearch2ClickStack.md`.

## Steering Document Alignment

### Technical Standards (tech.md)
- Implements the ClickStack-first technology stack (Go 1.24 services, Helm-packaged collectors, ClickHouse storage, HyperDX UI, Grafana overlays) defined in `.spec-workflow/steering/tech.md`.
- Reuses controller-runtime patterns for Kubernetes-facing automation and keeps OTLP-first pipelines with mTLS and row-level security.

### Project Structure (structure.md)
- New code lives under `internal/telemetry/clickstack` and `pkg/telemetry` to respect plane isolation.
- Helm/Kustomize assets land in `config/observability/` and the existing `install/helm/openchoreo-observability-plane` chart with overlays per environment.
- Observer API adapters and Backstage integrations consume interfaces rather than embedding ClickStack specifics, matching the module boundary guidance.

## Code Reuse Analysis
- **internal/observer/service**: Keep handler layer and request/response DTOs; swap storage interface to ClickStack implementation.
- **pkg/config + pkg/auth**: Reuse configuration loading and SPIFFE-based mTLS helpers for securing OTLP collectors and Observer API.
- **make/kind.mk + install/init/observability**: Reuse bootstrap targets, extending them with ClickStack CRDs and collectors.

### Existing Components to Leverage
- **opensearch logging service** → provides interface contracts; we will factor them into `pkg/telemetry/query`.
- **Fluent Bit / collector charts** → reuse templating patterns to deliver OTLP collector configs with minimal changes for metadata enrichment.

### Integration Points
- **Observer API** (`cmd/observer`, `internal/observer/handlers`) integrates with ClickStack via a new ClickHouse client built atop the official driver or `github.com/ClickHouse/clickhouse-go/v2`.
- **Backstage plugins** consume Observer endpoints and new signed HyperDX URLs exposed through the Observer service.
- **Grafana** dashboards ingest Prometheus metrics from ClickHouse, collectors, and the Observer API.

## Architecture
The solution deploys ClickStack components alongside OTLP collectors and updates the Observer API to query ClickHouse.

```mermaid
graph TD
    subgraph DataPlane["Data Plane Clusters"]
        FB[Fluent Bit]
        OC[OpenTelemetry Collector DaemonSet]
    end
    FB --> OC
    OC -->|OTLP gRPC/HTTP| OGW[Observability Gateway Service]
    OGW --> CH[ClickHouse Cluster]
    CH --> HDX[HyperDX UI]
    CH --> OBS[Observer API (Go)]
    OBS -->|REST/GraphQL| BP[Backstage Plugins]
    OBS --> CLI[choreoctl]
    OBS --> GRA[Grafana Datasources]
```

- Collectors capture logs/traces/metrics, enrich them with org/project labels, and push to the ClickStack ingestion service running inside the observability plane.
- ClickHouse stores raw telemetry plus materialized views (service map, RED metrics).
- HyperDX UI serves traces/logs, while Grafana pulls down-metric views.
- Observer API queries ClickHouse via SQL templates and signs dashboard URLs for Backstage.

### Modular Design Principles
- Each Go package focuses on a single concern (storage adapters, handlers, query builders).
- Helm templates split by component (clickhouse-cluster, hyperdx-ui, collectors, grafana, ops jobs).
- Service layer separation: handlers → service (`pkg/telemetry/service`) → storage adapters (`internal/telemetry/clickstack`).
- Utility modules (query builders, schema registry) grouped under `pkg/telemetry/query`.

## Components and Interfaces

### Component 1 — ClickStack Storage Adapter (`internal/telemetry/clickstack`)
- **Purpose:** Provide CRUD/query operations for logs, traces, metrics using ClickHouse SQL and wrap connection pooling, retries, and observability.
- **Interfaces:** Implements `pkg/telemetry/query.StorageProvider` with methods like `FetchComponentLogs(ctx, filters)`, `FetchTraces(ctx, spanFilters)`.
- **Dependencies:** ClickHouse Go driver, configuration from `pkg/config`, TLS credentials from `pkg/auth`.
- **Reuses:** DTOs and filter structs currently used by the OpenSearch service.

### Component 2 — Observer API Service Layer (`internal/observer/service`)
- **Purpose:** Mediate between HTTP handlers and storage adapters, handle dual-read, caching, and response shaping.
- **Interfaces:** `NewLoggingService(storage StorageProvider, opts ServiceOptions)`; exposes `GetComponentLogs`, `GetProjectLogs`, etc.
- **Dependencies:** Storage provider interface, feature flag module, metrics emitter.
- **Reuses:** Existing handlers (`internal/observer/handlers`) with minimal changes.

### Component 3 — Helm Chart Modules (`install/helm/openchoreo-observability-plane`)
- **Purpose:** Deploy ClickHouse cluster (statefulset + keeper), HyperDX UI, OTLP gateway, collectors, Grafana dashboards.
- **Interfaces:** Helm values (`profiles.standard`, `profiles.minimal`, `clickstack.storage.*`, `collectors.otel.*`).
- **Dependencies:** ClickHouse operator CRDs (optional), Kubernetes secrets/certs, cert-manager for TLS.
- **Reuses:** Chart structure, RBAC templates, helper templates, readiness jobs (updated for ClickStack health checks).

### Component 4 — Migration Jobs & Shadow Writer
- **Purpose:** Manage shadow write/cutover and cleanup of OpenSearch indices.
- **Interfaces:** Helm hooks or Kubernetes Jobs triggered via `helm upgrade --set migration.shadow=true`.
- **Dependencies:** Fluent Bit configs, OTLP collector config maps, object storage for backups.
- **Reuses:** Existing `opensearch-readiness-job` pattern, now pointing to ClickStack endpoints.

## Data Models

### Telemetry Filter DTO (Go)
```
type LogQuery struct {
    ComponentID   string
    ProjectID     string
    OrgID         string
    TimeRange     query.TimeRange
    Severity      []string
    SearchText    string
    Limit         int
    Cursor        string // for pagination
}
```

### ClickHouse Materialized View Schema
```
CREATE TABLE telemetry.logs_mv (
    Timestamp DateTime64(9),
    OrgID LowCardinality(String),
    ProjectID LowCardinality(String),
    ComponentID LowCardinality(String),
    Severity LowCardinality(String),
    Message String,
    Attributes Map(LowCardinality(String), String),
    K8sNamespace String,
    PodName String,
    TraceID String,
    SpanID String
) ENGINE = ReplacingMergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (OrgID, ProjectID, ComponentID, Timestamp)
TTL Timestamp + INTERVAL 90 DAY;
```

## Error Handling

### Error Scenario 1 — ClickHouse connectivity failure
- **Handling:** Storage adapter retries with exponential backoff, surfaces gRPC status codes to handlers, emits Prometheus metrics `telemetry_clickstack_connection_errors_total`.
- **User Impact:** Observer API returns HTTP 503 with actionable error message (`storage backend unavailable`) while dashboards show degraded banner.

### Error Scenario 2 — Query timeout / heavy scan
- **Handling:** Query builder enforces max timeout (e.g., 5s) and row limit; if exceeded, it cancels context and logs the offending filters for analysis.
- **User Impact:** API responds with 504 plus hint to tighten time range; HyperDX UI surfaces partial results.

### Error Scenario 3 — Metrics breach triggers autoscaling
- **Handling:** Helm chart installs HorizontalPodAutoscaler / ClickHouse Keeper scaling jobs; if scaling fails, alert rules notify operators.
- **User Impact:** None if auto-scaling succeeds; otherwise, status dashboard flags capacity risk.

## Testing Strategy

### Unit Testing
- Add tests for `pkg/telemetry/query` to validate SQL generation (filters, pagination, RLS hints).
- Mock ClickHouse client to ensure dual-read logic and translation functions return OpenSearch-compatible payloads.
- Validate Helm chart helper templates via `helm template` + `kubeconform` in CI.

### Integration Testing
- Extend `test/e2e` KinD suite to deploy ClickStack profile, seed synthetic telemetry via OTLP, and validate Observer API endpoints return expected data.
- Run migration shadow tests: simultaneously ingest into OpenSearch & ClickStack, diff results (tolerate <1% delta).

### End-to-End Testing
- Scenario: Developer runs `choreoctl logs component <id>` in a KinD environment; test asserts CLI output matches inserted log lines and that Grafana dashboards render.
- Scenario: FinOps dashboard collects cost metrics; verify exported CSV structure and totals.
