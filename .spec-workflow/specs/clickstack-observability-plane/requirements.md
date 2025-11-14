# Requirements Document

## Introduction
OpenChoreo must replace the OpenSearch-based observability backend deployed via `install/helm/openchoreo-observability-plane` with ClickStack (ClickHouse + HyperDX UI) so that traces, metrics, and logs flow through an OpenTelemetry-native stack described in `OpenSearch2ClickStack.md`. The new plane should offer faster queries, unified telemetry ingestion, and lower total cost of ownership without forcing application teams to retool their workflows.

## Alignment with Product Vision
This migration directly enables the product goals captured in `.spec-workflow/steering/product.md`: a unified telemetry lake, HyperDX experience layer, and OpenChoreo-native integrations that cut observability cost by ≥70% and drive <1s troubleshooting queries. By grounding the work in the steering documents, we ensure the observability plane advances the ClickStack product narrative and prepares every OpenChoreo installation for OTLP-first operations.

## Requirements

### Requirement 1 — ClickStack replaces OpenSearch in the observability plane Helm chart
**User Story:** As a platform engineer deploying OpenChoreo, I want the `openchoreo-observability-plane` Helm release to provision ClickStack (ClickHouse cluster + HyperDX UI) instead of OpenSearch so that clusters gain high-performance telemetry storage out of the box.

#### Acceptance Criteria
1. WHEN I install the chart with the `standard` profile THEN the rendered manifests SHALL create ClickHouse pods, HyperDX (or ClickStack UI), and supporting services, and SHALL NOT deploy OpenSearch pods.
2. IF the operator toggles the `minimal` profile THEN the chart SHALL deploy a single-node ClickStack stack sized for edge clusters while still exposing OTLP ingestion endpoints.
3. WHEN a user runs `kubectl get pods -n openchoreo-observability-plane` AFTER installation THEN all components SHALL become Ready within 10 minutes or emit status conditions indicating the exact blocking dependency.

### Requirement 2 — OTLP ingestion pipeline with managed collectors
**User Story:** As an SRE responsible for telemetry ingestion, I want OpenChoreo to ship opinionated OTLP collector configs that forward traces, metrics, and logs into ClickStack so that teams can enable end-to-end observability without bespoke wiring.

#### Acceptance Criteria
1. WHEN `make deploy-observability` runs against KinD THEN the resulting OpenTelemetry Collector SHALL expose OTLP gRPC (4317) and HTTP (4318) endpoints and SHALL ship data into the ClickStack ingestion service with TLS.
2. IF a data plane cluster is registered via OpenChoreo APIs THEN the platform SHALL template a collector ConfigMap that labels data with organization/project metadata (matching `OpenSearch2ClickStack.md` metadata map).
3. WHEN collectors encounter backpressure from ClickStack THEN they SHALL retry with exponential backoff and emit Prometheus metrics so the platform can alert on ingestion lag >3 seconds.

### Requirement 3 — Observer API & Backstage compatibility layer
**User Story:** As an application developer using Backstage widgets and the Observer API, I want the same endpoints (`/api/logs/*`, `/api/traces/*`) to work after migration so that I can adopt ClickStack without changing UIs or CLI workflows.

#### Acceptance Criteria
1. WHEN clients call existing Observer API endpoints THEN the service SHALL translate requests into ClickHouse SQL queries and respond with the same schema currently produced by OpenSearch handlers.
2. IF a feature flag enables “dual-read” mode during migration THEN the Observer service SHALL query both OpenSearch and ClickStack and log discrepancies when payloads diverge by >1%.
3. WHEN Backstage loads observability widgets THEN dashboards SHALL embed HyperDX or Grafana views using signed URLs without exposing ClickHouse credentials to the browser.

### Requirement 4 — Zero-downtime migration & data lifecycle controls
**User Story:** As a program manager overseeing the migration, I want a controlled cutover plan so that existing OpenSearch deployments can transition to ClickStack without data loss or tenant downtime.

#### Acceptance Criteria
1. WHEN the migration playbook runs THEN it SHALL support a “shadow write” phase where Fluent Bit / OTEL collectors send data to both OpenSearch and ClickStack for at least 7 days.
2. IF the ClickStack cluster breaches capacity thresholds (CPU >70% or disk >80%) THEN the platform SHALL scale replicas or offload cold data to object storage automatically, as outlined in `OpenSearch2ClickStack.md`.
3. WHEN operators finalize cutover THEN the chart SHALL provide a job or script that archives residual OpenSearch indices and deletes OpenSearch resources only after verifying ClickStack retention and backup policies are active.

### Requirement 5 — Cost and performance observability
**User Story:** As a FinOps stakeholder, I need visibility into ClickStack ingestion cost, retention, and performance so that I can validate the promised 70–90% TCO reduction.

#### Acceptance Criteria
1. WHEN the observability plane deploys THEN it SHALL publish ClickHouse resource metrics (CPU, disk, compression ratio) plus ingestion throughput to Prometheus and Grafana dashboards.
2. IF ingestion latency exceeds 3 seconds OR query p95 exceeds 1 second for 24h log searches THEN alerting rules SHALL fire to the platform’s incident channel.
3. WHEN monthly reports run THEN the system SHALL export cost estimates (storage TB, compute hours) per tenant or organization via API or CSV.

## Non-Functional Requirements

### Code Architecture and Modularity
- Migration code SHALL respect the plane boundaries defined in `.spec-workflow/steering/structure.md`, isolating ClickStack-specific logic under a new telemetry module.
- Interfaces (e.g., storage providers) SHALL abstract query engines so future backends can plug in without rewriting API handlers.

### Performance
- ClickStack ingest path SHALL sustain ≥2 million log events/sec with <3s lag and return 24h log searches (1B rows) in <1s p95, as promised in `OpenSearch2ClickStack.md`.
- Collector CPU/memory footprint per node SHALL remain within 250m CPU / 256Mi RAM to avoid impacting workloads.

### Security
- All traffic between collectors, Observer API, and ClickStack SHALL use mutual TLS with SPIFFE identities.
- Tenant isolation SHALL be enforced via ClickHouse row-level security and API-level RBAC aligned with organization/project scopes.

### Reliability
- The observability plane SHALL operate with ≥99.9% availability, featuring ClickHouse replica sets and automated backups to object storage.
- Migration tooling SHALL offer automated rollback steps if ClickStack health checks fail for more than 15 minutes.

### Usability
- Platform engineers SHALL configure the new stack via Helm values only, without editing templates.
- Developers SHALL continue to use Backstage cards and `choreoctl logs/traces` commands without learning new query languages; advanced SQL is optional.
