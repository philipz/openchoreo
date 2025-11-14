# Technology Stack

## Project Type
Modular internal developer platform focused on the observability plane: Kubernetes-native microservices, CLI tooling, and UI integrations that expose ClickStack (ClickHouse + HyperDX) as a first-class telemetry backend for OpenChoreo tenants.

## Core Technologies

### Primary Language(s)
- **Go 1.24.2**: Control-plane services, Observer/Telemetry APIs, collectors automation, CLI (`choreoctl`) and operators built on controller-runtime.
- **TypeScript/React (Backstage 1.30+)**: Developer portal plugins and dashboards that surface ClickStack data.
- **YAML/Helm/Kustomize**: Platform configuration, pipelines, and deployment manifests for Kubernetes environments.

### Key Dependencies/Libraries
- **sigs.k8s.io/controller-runtime v0.20**: Operator patterns for control-plane reconciliation.
- **github.com/envoyproxy/gateway v1.3**: Manages Envoy-based ingress for telemetry APIs.
- **k8s.io/client-go / API machinery v0.32**: Kubernetes integration for multi-cluster orchestration.
- **OpenTelemetry Collector (custom build)**: OTLP ingestion, tail sampling, attribute enrichment, ClickHouse exporter.
- **ClickStack (HyperDX UI + ClickHouse 24.x)**: Storage, query, and visualization stack for traces/metrics/logs.
- **Grafana 11.x**: Complementary dashboards (SLO, cost, platform KPIs).
- **Backstage plugins + choreoctl CLI**: Integrations that expose observability data to engineers.

### Application Architecture
Cell-based multi-plane architecture: collectors run per data plane, forward telemetry via OTLP to a shared ClickStack cluster hosted in the observability plane. A Go-based Observer API exposes curated query surfaces, while HyperDX UI and Grafana provide direct visualization. Backstage consumes Observer API plus signed dashboard embeds. All services are packaged as Helm charts and reconciled by OpenChoreo operators.

### Data Storage (if applicable)
- **Primary storage**: ClickHouse MergeTree tables in ClickStack (hot tier, 90–180 days). Optional S3/MinIO object storage for cold tier via tiered storage and Iceberg integration.
- **Caching**: ClickHouse query cache and Keeper-based metadata cache; Envoy local response caching for common API aggregations.
- **Data formats**: Native OTLP protobuf payloads on ingest, stored as columnar data with materialized maps for Kubernetes metadata; downstream exports available as JSON/Parquet.

### External Integrations (if applicable)
- **APIs**: Kubernetes API servers, Backstage Catalog, Grafana HTTP API, identity providers (OIDC/SAML), billing systems for cost reporting.
- **Protocols**: OTLP gRPC/HTTP (4317/4318), HTTPS/REST for Observer API, WebSocket/SSE for live tail, gRPC between control-plane services.
- **Authentication**: SPIFFE/SPIRE-based mTLS between agents, OIDC/JWT for user-facing APIs, service tokens scoped per organization.

### Monitoring & Dashboard Technologies (if applicable)
- **Dashboard Framework**: HyperDX (ClickStack UI) + Backstage widgets + Grafana dashboards.
- **Real-time Communication**: WebSocket live tail for logs, OTLP streaming to collectors, Kafka optional for buffering.
- **Visualization Libraries**: Grafana panels (TimeSeries, Flamegraph, Traces), HyperDX native charts, Backstage uses PatternFly/Material UI.
- **State Management**: Backstage uses Redux Toolkit; Observer API cache uses Redis or ClickHouse dictionaries for derived views.

## Development Environment

### Build & Development Tools
- **Build System**: Makefile + Go toolchain + Docker/Buildx images for operators and collectors; Helm for packaging.
- **Package Management**: `go mod` for Go services; `pnpm`/`yarn` for Backstage plugins; OCI registries for Helm and container images.
- **Development workflow**: DevContainer/KinD-based local clusters (`make kind-up`, `make deploy-observability`), hot reload via `air` for Go services, Backstage hot module reload for UI work.

### Code Quality Tools
- **Static Analysis**: `golangci-lint`, `govulncheck`, `kubelinter` for manifests.
- **Formatting**: `gofmt`, `goimports`, `prettier`/`eslint` for TypeScript.
- **Testing Framework**: `ginkgo`/`gomega` for Go integration tests, `jest` for Backstage plugins, end-to-end smoke tests via `make test-e2e` against KinD.
- **Documentation**: `mdbook`/MkDocs for architecture notes, inline API docs generated via `swag` and Backstage TechDocs.

### Version Control & Collaboration
- **VCS**: Git on GitHub (`openchoreo/openchoreo`), signed commits encouraged.
- **Branching Strategy**: Trunk-based with short-lived feature branches and protected `main`; release branches cut per milestone.
- **Code Review Process**: Mandatory PR reviews with CI (lint, unit, integration) plus spec-workflow approvals for major architecture work.

### Dashboard Development (if applicable)
- **Live Reload**: Backstage dev server with HMR; Grafana dashboards managed via Jsonnet + `grizzly`.
- **Port Management**: Default ports 3000 (Backstage), 3001 (HyperDX proxy), 4317/4318 (collector) with overrides via `values.yaml`.
- **Multi-Instance Support**: Namespaced Helm releases per environment; Backstage dev instances can run concurrently via `PORT` env overrides.

## Deployment & Distribution (if applicable)
- **Target Platform(s)**: Kubernetes 1.28+ clusters (control plane + multiple data planes). Supports self-hosted bare metal, cloud managed K8s, and air-gapped installations.
- **Distribution Method**: OCI-hosted Helm charts, container images in GHCR, Terraform blueprints for infrastructure, sample `make` recipes for bootstrap.
- **Installation Requirements**: ClickHouse cluster (3+ nodes, NVMe), object storage for backups, Ingress/Gateway API, cert-manager, OpenTelemetry Collectors per cluster.
- **Update Mechanism**: Helm upgrade pipelines with canaries, schema migrations managed via ClickHouse migrations, collector rolling updates using surge strategy.

## Technical Requirements & Constraints

### Performance Requirements
- Sustain ≥2 million log events/sec ingest with <3s ingestion lag.
- Trace joins across 1B span rows in <1s (p95) for 24h windows.
- Metrics retention precision: 15s resolution for 30 days, downsampled thereafter.

### Compatibility Requirements
- **Platform Support**: Linux/amd64 primary, ARM64 preview; Kubernetes 1.28–1.31; ClickHouse 24.x.
- **Dependency Versions**: Go 1.24.2+, Helm 3.15+, OpenTelemetry Collector 0.103+, Backstage 1.30+.
- **Standards Compliance**: OTLP 1.0, OpenMetrics, Gateway API v1.0, CNCF SIG Observability guidelines.

### Security & Compliance
- **Security Requirements**: Mutual TLS between collectors and ClickStack, encrypted-at-rest disks (LUKS, EBS), role-based query scopes enforced via ClickHouse row policies.
- **Compliance Standards**: SOC2 readiness, GDPR data residency (per-tenant partitions), optional HIPAA mode with PHI scrubbing pipelines.
- **Threat Model**: Protect against multi-tenant data exfiltration, collector credential leakage, and query amplification attacks via rate limiting and token scoping.

### Scalability & Reliability
- **Expected Load**: Reference deployment = 600 services, 20 clusters, 8TB/day telemetry.
- **Availability Requirements**: 99.9% Observer API SLA; ClickStack HA with quorum replicas and cross-AZ storage; disaster recovery via object-storage snapshots + schema replay.
- **Growth Projections**: Horizontal sharding of ClickHouse clusters, auto-scaling collectors, tiered storage for >1PB/year footprints.

## Technical Decisions & Rationale
1. **ClickStack (ClickHouse) over OpenSearch**: Columnar engine delivers 10–30x faster queries, 70–90% lower storage cost, and native OTLP schemas; sacrifices full-text relevance but offsets via SQL expressiveness.
2. **OpenTelemetry-first pipelines**: Standardizing on OTLP simplifies instrumentation across OpenChoreo components and unlocks vendor-neutral tooling.
3. **Backstage + Observer API**: Keeps developer experience consistent inside the OpenChoreo portal, enabling RBAC-aware surfacing of telemetry without exposing raw ClickHouse credentials.
4. **Helm-packaged Plane**: Helm/Kustomize alignment with rest of platform enables GitOps promotion, consistent templating, and environment overlays.

## Known Limitations
- **Full-text search parity**: ClickStack’s SQL-focused workflow lacks some Lucene operators; complex regex searches require materialized views or fallback to light-weight OpenSearch sidecar.
- **Operational expertise**: Running ClickHouse at scale demands knowledge of MergeTree tuning, storage balancing, and keeper operations; requires dedicated runbooks.
- **Cold storage query lag**: Queries hitting S3-based tiered storage incur >5s latency, so true long-term forensic analysis may need asynchronous exports or Presto/Trino adapters.
