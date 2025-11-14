# Product Overview

## Product Purpose
Transform OpenChoreo's observability plane into a ClickStack-powered telemetry product that natively ingests traces, metrics, and logs over OpenTelemetry, delivers 10-30x faster query times, and cuts storage cost by 70-90% while remaining fully self-hosted for enterprise governance.

## Target Users
- **Platform Engineering & SRE teams** who need a scalable, multi-tenant observability backend that matches OpenChoreo's cell-based architecture.
- **Application developers** consuming OpenChoreo who require low-latency troubleshooting, dependency insights, and curated dashboards without learning OpenSearch internals.
- **Engineering leadership & FinOps partners** responsible for cost efficiency, compliance, and reporting on platform health across environments.

## Key Features
1. **Unified Telemetry Lake**: A ClickHouse-backed store that ingests OTLP traces, metrics, and logs with automatic schema evolution, Kubernetes metadata enrichment, and long-retention tiers.
2. **HyperDX Experience Layer**: ClickStack UI plus Grafana boards that surface service maps, RED/SLO views, cost/perf KPIs, and drill-down log search in <1s for p95 queries.
3. **OpenChoreo-native Integrations**: Opinionated collectors, Backstage widgets, and API adapters that let projects inherit dashboards, alerts, and RBAC from platform blueprints.

## Business Objectives
- Reduce observability TCO by at least 70% within 12 months while keeping >180 days of hot telemetry.
- Shrink mean time to detect (MTTD) and mean time to resolve (MTTR) by 40% through richer traces and cross-slice correlation.
- Provide a productized migration path that any OpenChoreo deployment can apply within 2 quarters, expanding the addressable market for the IDP.
- Offer differentiated enterprise value (compliance, tenancy guardrails, curated insights) to support subscription and support revenue.

## Success Metrics
- **Query Performance**: <1s p95 for 24h log search with 1B+ rows; <200ms service dependency queries.
- **Cost Efficiency**: ≤$0.50/TB/month effective storage cost and ≤$0.10M annual run-rate for reference scale (6 clusters, 600 components).
- **Adoption & Coverage**: ≥90% of OpenChoreo projects emitting OTLP data via managed collectors; ≥30 platform teams running ClickStack in production by FY+1.

## Product Principles
1. **OpenTelemetry-First**: Every ingest, transform, and API contract speaks native OTLP to avoid vendor lock-in while enabling third-party tooling.
2. **Opinionated but Extensible**: Ship with batteries-included pipelines, dashboards, and alerts, yet allow advanced users to fork schemas, add table engines, or plug custom retention tiers.
3. **Operational Transparency**: Provide platform and tenant-level health, capacity, and cost insights directly in ClickStack UI and via APIs so FinOps and SREs never operate blind.

## Monitoring & Visibility (if applicable)
- **Dashboard Type**: HyperDX web UI for traces/logs + curated Grafana boards for metrics and cost telemetry; Backstage widgets for project-level overviews.
- **Real-time Updates**: OTLP streaming via optimized collectors feeding ClickHouse materialized views; optional WebSocket push for alerting surfaces.
- **Key Metrics Displayed**: RED/SLO indicators, ingestion lag, retention utilization, cost per tenant, error budget burn, and collector health across clusters.
- **Sharing Capabilities**: Signed, time-bound dashboard links; scheduled PDF exports; API/webhook integration for incident bots.

## Future Vision
Position ClickStack as the canonical observability stack for OpenChoreo, enabling marketplace add-ons (AIOps, anomaly detection) and hosted/SaaS deployment options while keeping the core open-source.

### Potential Enhancements
- **Remote Access**: Managed tunnel service so customers can share read-only dashboards with auditors or partners without exposing clusters.
- **Analytics**: Historical trend lakehouses (Iceberg on object storage) plus ML-driven cost simulators to predict retention and capacity.
- **Collaboration**: Incident timelines, per-trace annotations, and chat integrations that sync comments back to ClickStack views and Backstage incidents.
