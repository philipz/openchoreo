# Project Structure

## Directory Organization
```
openchoreo/
├── cmd/                    # CLI & control-plane entrypoints (choreoctl, observer, API)
├── internal/               # Private modules (observers, collectors orchestration, controllers)
│   ├── observer/           # OpenSearch-era services to be upgraded for ClickStack
│   ├── planes/             # Plane-specific reconciliation logic
│   └── telemetry/          # (New) ClickStack adapters, OTLP routing, schema tooling
├── pkg/                    # Reusable libraries (config, auth, platform SDK)
├── api/                    # Kubernetes CRD types (Projects, Components, Pipelines)
├── config/                 # Helm/Kustomize manifests for managers, collectors, ClickStack
├── install/                # Bootstrap scripts (KinD, Terraform samples)
├── docs/                   # Architecture whitepapers, migration guides, diagrams
├── samples/                # Example workloads & platform configs, including observability recipes
├── make/                   # Modular makefiles (lint, golang, kube, helm, docker)
├── .spec-workflow/         # Steering + spec docs, implementation logs
└── tools/                  # Generators (helm-gen, licenser), developer utilities
```
- Observability plane artifacts (ClickStack schemas, collector charts, Grafana dashboards) live under `config/observability/` with overlays per environment.
- Backstage plugins and frontend assets reside in `docs/portal/` (or future `/ui`) to keep UI code isolated from Go modules.

## Naming Conventions
### Files
- **Go packages**: `lowercase` with no underscores (e.g., `internal/observer/service`).
- **Helm charts / Kustomize overlays**: `kebab-case` directories (e.g., `clickstack-collector`).
- **Backstage plugins**: `plugin-name` directories with `index.tsx`, `*-card.tsx`.
- **Tests**: Go tests use `_test.go`; UI tests use `.test.tsx`.

### Code
- **Types/Structs**: `PascalCase` (`ComponentSpec`, `ClickStackConfig`).
- **Functions/Methods**: `camelCase` (`buildIngestPipeline`, `NewCollectorChart`).
- **Constants**: `UPPER_SNAKE_CASE` for package-scoped constants, `camelCase` for locals.
- **Variables**: `camelCase` in Go, `camelCase`/`const` in TypeScript, `kebab-case` for YAML keys.

## Import Patterns
### Import Order
1. Standard library
2. Third-party deps (grouped by domain: Kubernetes, OpenTelemetry, ClickHouse, others)
3. Internal packages (`github.com/openchoreo/openchoreo/...`)

Backstage/TypeScript modules follow: React core → third-party UI libs → shared utilities → local components.

### Module/Package Organization
- Go modules under `cmd`, `internal`, and `pkg` use absolute imports from `github.com/openchoreo/openchoreo`.
- Feature boundaries (e.g., `internal/telemetry/clickstack`) expose interfaces via `pkg/observability` to keep CLI/handlers decoupled from storage-specific logic.
- Helm charts share `/charts/common` libraries for collectors; overlays import base charts via Kustomize `bases`.

## Code Structure Patterns
### Module/Class Organization
```
1. Imports
2. Constants/config structs
3. Interfaces
4. Implementations
5. Helper funcs
6. init()/exports
```

### Function/Method Organization
- Validate inputs + derive context
- Core logic (query building, reconciler steps)
- Error handling with wrapped context
- Return typed responses/DTOs

### File Organization Principles
- One primary struct/handler per file when >200 LOC
- Keep ClickStack-specific code in `telemetry/clickstack` to ease future backend swaps
- Shared schemas (SQL templates, dashboards) stored as `.sql`/`.jsonnet` assets under `config/observability/assets`

## Code Organization Principles
1. **Plane Isolation**: Control plane, data plane agents, and observability plane code live in distinct packages to prevent accidental cross-dependencies.
2. **Interface-first**: Observer APIs depend on `StorageProvider` interfaces so OpenSearch → ClickStack migration remains incremental.
3. **Declarative Config**: Helm/Kustomize remains source of truth; Go code reads structured configs rather than embedding environment logic.
4. **Spec Alignment**: Every new module documents requirements/design/tasks via `.spec-workflow` before implementation.

## Module Boundaries
- **Observer API ↔ Storage**: `internal/observer/service` consumes `pkg/telemetry/query` interfaces; implementations live in `internal/telemetry/{opensearch,clickstack}`.
- **Collectors ↔ Control Plane**: Collectors (Helm charts) expose CRDs/values consumed by `pkg/planes/observability` reconciler.
- **UI ↔ API**: Backstage plugins call Observer API; no direct ClickHouse credentials in UI code. Grafana dashboards interact via service accounts managed in `config/observability/iam`.
- **Tenant Isolation**: Query policies enforced via ClickHouse row-level security definitions stored alongside schema migrations.

## Code Size Guidelines
- **Go files**: prefer <500 LOC; split packages when exceeding.
- **Go functions**: keep under ~50 LOC; use helper funcs for query builders and reconciliation steps.
- **TypeScript components**: <200 LOC per component; break cards/widgets into hooks + presentation.
- **YAML manifests**: factor reusable snippets into `values.yaml`/`_helpers.tpl`.

## Dashboard/Monitoring Structure
```
config/observability/
├── clickstack/
│   ├── charts/
│   ├── values/
│   └── schemas/          # SQL migrations & materialized views
├── grafana/
│   ├── dashboards/       # Jsonnet or JSON exports
│   └── datasources/
└── collectors/
    ├── otel/             # OpenTelemetry Collector configs
    └── fluent-bit/       # Legacy compatibility
```
- Dashboards version-controlled via Jsonnet + CI to render + lint before release.
- Observer API exposes health/metrics endpoints scraped by Prometheus for self-monitoring.

## Documentation Standards
- Each plane/module owns a `README.md` describing purpose, inputs/outputs, deployment steps.
- ClickStack schemas documented with ER diagrams and sample queries in `docs/observability`.
- Runbooks (backup/restore, scaling) live in `docs/runbooks/`.
- Inline comments reserved for non-obvious logic (e.g., ClickHouse optimizer hints).
- Steering/spec documents updated per workflow; implementation logs recorded after each task.
