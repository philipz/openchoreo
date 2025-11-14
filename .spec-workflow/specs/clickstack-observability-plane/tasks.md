# Tasks Document

 - [x] 1. Introduce telemetry storage interface and ClickStack adapter
  - File: pkg/telemetry/query/provider.go; internal/telemetry/clickstack/*
  - Define `StorageProvider` interface extracted from current OpenSearch service; implement ClickStack-backed provider using clickhouse-go with TLS, retries, and SQL templates covering logs/traces queries.
  - Purpose: Decouple Observer API from storage engine and enable ClickStack queries.
  - _Leverage: internal/observer/service/logging_service.go, internal/observer/opensearch package, pkg/config_
  - _Requirements: Requirement 1, Requirement 3_
  - _Prompt: Implement the task for spec clickstack-observability-plane, first run spec-workflow-guide to get the workflow guide then implement the task: Role: Go backend engineer specializing in observability storage layers | Task: Extract a telemetry `StorageProvider` interface (logs/traces) and implement a ClickStack adapter under internal/telemetry/clickstack using the ClickHouse Go driver with TLS + retries + SQL templates, wiring it into dependency injection so Observer services can consume it | Restrictions: Do not remove the existing OpenSearch provider yet; keep feature flags for dual-read, follow pkg/telemetry structure, keep files under 500 LOC | _Leverage: internal/observer/service/logging_service.go, internal/observer/opensearch/client.go, pkg/config | _Requirements: Requirement 1, Requirement 3 | Success: Tests cover SQL generation and error handling; Observer service can be configured to use ClickStack provider without breaking OpenSearch paths; lints/tests pass_

 - [x] 2. Update Observer API handlers for ClickStack + dual-read
  - File: internal/observer/service/logging_service.go; internal/observer/handlers/*
  - Add configuration to select ClickStack provider, wire dual-read diff logging, ensure response schemas remain backward compatible, expose signed HyperDX URLs.
  - Purpose: Keep `/api/logs/*` and `/api/traces/*` endpoints stable for Backstage and CLI clients while enabling ClickStack queries.
  - _Leverage: cmd/observer/main.go, internal/observer/middleware, pkg/config/observer_
  - _Requirements: Requirement 3_
  - _Prompt: Implement the task for spec clickstack-observability-plane, first run spec-workflow-guide to get the workflow guide then implement the task: Role: Go application developer focused on API surfaces | Task: Extend Observer API to select the ClickStack storage provider, add optional dual-read comparisons vs OpenSearch, integrate HyperDX signed URL generation, and keep response contracts unchanged | Restrictions: No breaking API changes, dual-read must be flag-gated, ensure structured logging for discrepancies | _Leverage: cmd/observer/main.go, internal/observer/service/logging_service.go, internal/observer/opensearch | _Requirements: Requirement 3 | Success: Observer can switch between providers via config, dual-read logs <1% drift metrics, HyperDX link endpoint returns signed URL, unit tests updated_

 - [x] 3. Replace OpenSearch resources in Helm chart with ClickStack stack
  - File: install/helm/openchoreo-observability-plane/templates/*; values.yaml; new config/observability/*
  - Remove OpenSearch CRDs/templates, add ClickHouse cluster (statefulset or operator CR), HyperDX UI deployment, OTLP gateway service; expose Helm profiles for standard/minimal; add readiness jobs.
  - Purpose: Deploy ClickStack components via the existing observability plane chart.
  - _Leverage: existing opensearch templates, config/kustomize overlays, docs/OpenSearch2ClickStack.md_
  - _Requirements: Requirement 1, Requirement 4_
  - _Prompt: Implement the task for spec clickstack-observability-plane, first run spec-workflow-guide to get the workflow guide then implement the task: Role: Helm/Kubernetes engineer | Task: Rewrite the `openchoreo-observability-plane` chart to provision ClickHouse + HyperDX + OTLP gateway (standard/minimal profiles), removing OpenSearch manifests and ensuring readiness/health checks align with ClickStack | Restrictions: Maintain chart values compatibility where possible, keep TLS secrets/certs handling, do not break existing make deploy targets | _Leverage: install/helm/openchoreo-observability-plane/templates, config/observability assets | _Requirements: Requirement 1, Requirement 4 | Success: Helm template validates, KinD install deploys ClickStack pods that pass readiness, OpenSearch resources no longer rendered_

- [x] 4. Ship OTLP collector configurations and metadata enrichment
  - File: config/observability/collectors/otel/*; install/helm/openchoreo-observability-plane/templates/collectors.yaml; make/kind.mk
  - Provide ConfigMaps/DaemonSets for managed OTEL collectors, include org/project labels, TLS secrets, retry policies, optional Fluent Bit bridge; update `make deploy-observability` to apply them.
  - Purpose: Ensure data planes stream traces/metrics/logs into ClickStack reliably.
  - _Leverage: existing Fluent Bit configs, OpenTelemetry Collector upstream manifests, install/init/observability scripts_
  - _Requirements: Requirement 2_
  - _Prompt: Implement the task for spec clickstack-observability-plane, first run spec-workflow-guide to get the workflow guide then implement the task: Role: Observability pipeline engineer | Task: Create OTLP collector DaemonSets/configs for data plane ingestion, add metadata enrichment processors, TLS/mTLS configuration, and integrate with make targets | Restrictions: Keep CPU/RAM budgets <250m/256Mi per node, ensure configs are templatized for multi-cluster, maintain compatibility with existing logging flows | _Leverage: install/init/observability/fluent-bit configs, upstream otel collector charts | _Requirements: Requirement 2 | Success: `make deploy-observability` sets up collectors that send test telemetry to ClickStack with enriched attributes; Prometheus metrics expose ingestion lag_

- [x] 5. Implement migration tooling and shadow write orchestration
  - File: install/helm/openchoreo-observability-plane/templates/migration/*; docs/OpenSearch2ClickStack.md updates
  - Add Jobs/scripts to run shadow write (dual-output collectors), verify ClickStack health, archive old indices, and tear down OpenSearch only after validation.
  - Purpose: Enable zero-downtime migration per requirement 4.
  - _Leverage: existing readiness jobs, Fluent Bit configs, doc instructions_
  - _Requirements: Requirement 4_
  - _Prompt: Implement the task for spec clickstack-observability-plane, first run spec-workflow-guide to get the workflow guide then implement the task: Role: Platform migration engineer | Task: Provide Helm hooks/jobs and documentation to orchestrate shadow writes, validation, backup, and cleanup steps for OpenSearch → ClickStack cutover | Restrictions: Must provide rollback steps, log success/failure states, make no destructive action without explicit flag | _Leverage: install/helm/openchoreo-observability-plane/templates/*, OpenSearch2ClickStack.md | _Requirements: Requirement 4 | Success: Operators can run documented command sequence to shadow write, validate, and remove OpenSearch with zero downtime_

- [x] 6. Cost/performance observability dashboards & alerts
  - File: config/observability/grafana/dashboards/*; install/helm/openchoreo-observability-plane/templates/monitoring/*
  - Create Grafana/HyperDX dashboards and Prometheus rules for ingest throughput, query latency, compression ratio, cost estimates; expose API endpoints for monthly CSV export.
  - Purpose: Give FinOps/SRE visibility into ClickStack efficiency.
  - _Leverage: Grafana provisioning, Observer API, cost calculation formulas in OpenSearch2ClickStack.md_
  - _Requirements: Requirement 5_
  - _Prompt: Implement the task for spec clickstack-observability-plane, first run spec-workflow-guide to get the workflow guide then implement the task: Role: Observability/FinOps engineer | Task: Build dashboards and alerting covering ingestion lag, query p95, resource utilization, cost per tenant, plus an API/CSV export for monthly reports | Restrictions: Dashboards must be version-controlled, alerts follow existing Prometheus rules style, API exports reuse observer service | _Leverage: config/observability/grafana, internal/observer/service | _Requirements: Requirement 5 | Success: Grafana shows metrics with correct labels, Prometheus alerts fire on threshold breaches, API endpoint returns cost CSV_

- [x] 7. Backstage & CLI integration updates
  - File: docs/portal/backstage/plugins/*; cmd/choreoctl/logs.go; docs/user-guides/*
  - Update Backstage plugins to embed HyperDX/Grafana dashboards via signed URLs, ensure CLI commands query Observer API unchanged, document new capabilities.
  - Purpose: Provide seamless UX for developers post-migration.
  - _Leverage: existing Backstage plugin code, CLI log/traces commands, HyperDX embedding docs_
  - _Requirements: Requirement 3, Requirement 5_
  - _Prompt: Implement the task for spec clickstack-observability-plane, first run spec-workflow-guide to get the workflow guide then implement the task: Role: Frontend/CLI engineer | Task: Update Backstage widgets to render ClickStack dashboards via signed links, ensure CLI log/trace commands handle ClickStack responses, and refresh documentation | Restrictions: No breaking CLI flags, maintain RBAC/tenant isolation, follow portal coding standards | _Leverage: docs/portal/backstage, cmd/choreoctl, Observer API responses | _Requirements: Requirement 3, Requirement 5 | Success: Backstage cards load without CORS leaks, CLI outputs unchanged formats, docs explain new telemetry stack_

- [x] 8. E2E validation & documentation updates
  - File: test/e2e/*; docs/OpenSearch2ClickStack.md; docs/runbooks/*
  - Expand KinD E2E tests to cover ClickStack deployment, ingestion, query validation, failover; update migration guide with final steps.
  - Purpose: Ensure reliability and provide runbooks for operators.
  - _Leverage: existing e2e tests, KinD scripts, runbook structure_
  - _Requirements: All_
  - _Prompt: Implement the task for spec clickstack-observability-plane, first run spec-workflow-guide to get the workflow guide then implement the task: Role: QA/Docs engineer | Task: Extend e2e suite to validate ClickStack stack (ingest, query, failover), and finalize docs/runbooks per new architecture | Restrictions: Tests must run in CI within reasonable time, docs must align with steering narratives, avoid duplication | _Leverage: test/e2e, docs/OpenSearch2ClickStack.md, docs/runbooks | _Requirements: All | Success: CI e2e passes on KinD with ClickStack, docs published with updated diagrams and procedures_
