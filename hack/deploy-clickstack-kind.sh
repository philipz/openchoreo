#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}" )/.." && pwd)"
cd "${REPO_ROOT}"

RELEASE_NAME="${CLICKSTACK_RELEASE_NAME:-openchoreo-observability-plane}"
NAMESPACE="${CLICKSTACK_NAMESPACE:-openchoreo-observability-plane}"
CLICKSTACK_PASSWORD="${CLICKSTACK_PASSWORD:-KindClickStackP@55w0rd}"
HYPERDX_ENABLED="${HYPERDX_ENABLED:-true}"
HYPERDX_SIGNING_KEY="${HYPERDX_SIGNING_KEY:-kind-hdx-signing-secret}"
EXTRA_HELM_ARGS=()

export HELM_CACHE_HOME="${HELM_CACHE_HOME:-/tmp/helmcache}"
export HELM_CONFIG_HOME="${HELM_CONFIG_HOME:-/tmp/helmconfig}"
export HELM_DATA_HOME="${HELM_DATA_HOME:-/tmp/helmdata}"

mkdir -p "$HELM_CACHE_HOME" "$HELM_CONFIG_HOME" "$HELM_DATA_HOME"

ensure_cilium() {
  if kubectl get daemonset -n cilium cilium >/dev/null 2>&1; then
    echo "Cilium already installed"
    return
  fi
  echo "Installing Cilium via make kind.install.cilium ..."
  make kind.install.cilium
}

helm_deploy() {
  EXTRA_HELM_ARGS+=("--set" "global.installationMode=minimal")
  EXTRA_HELM_ARGS+=("--set" "clickstack.credentials.password=${CLICKSTACK_PASSWORD}")
  EXTRA_HELM_ARGS+=("--set" "openSearch.enabled=false")
  EXTRA_HELM_ARGS+=("--set" "openSearchCluster.enabled=false")
  EXTRA_HELM_ARGS+=("--set" "openSearchClusterSetup.enabled=false")
  EXTRA_HELM_ARGS+=("--set" "hyperdx.signing.key=${HYPERDX_SIGNING_KEY}")
  EXTRA_HELM_ARGS+=("--set" "gateway.config.exporters.clickhouse.endpoint=tcp://clickstack:9000?dial_timeout=10s")
  EXTRA_HELM_ARGS+=("--set" "observer.telemetry.backend=clickstack")
  EXTRA_HELM_ARGS+=("--set" "observer.telemetry.dualRead=false")
  EXTRA_HELM_ARGS+=("--set" "hyperdx.mongodb.enabled=true")

  if [[ "${HYPERDX_ENABLED}" != "true" ]]; then
    EXTRA_HELM_ARGS+=("--set" "hyperdx.enabled=false")
  fi

  echo "Deploying ClickStack Helm release '${RELEASE_NAME}' into namespace '${NAMESPACE}' ..."
  helm upgrade --install "${RELEASE_NAME}" \
    "${REPO_ROOT}/install/helm/openchoreo-observability-plane" \
    --namespace "${NAMESPACE}" \
    --create-namespace \
    --wait \
    --timeout 15m \
    "${EXTRA_HELM_ARGS[@]}"
}

main() {
  ensure_cilium
  helm_deploy

  echo "ClickStack pods:"
  kubectl get pods -n "${NAMESPACE}"

  echo "Waiting for ClickStack core components to become Ready..."
  kubectl rollout status statefulset/clickstack -n "${NAMESPACE}" --timeout=5m || true
  kubectl rollout status deployment/otlp-gateway -n "${NAMESPACE}" --timeout=5m || true
  kubectl rollout status deployment/observer -n "${NAMESPACE}" --timeout=5m || true
  if [[ "${HYPERDX_ENABLED}" == "true" ]]; then
    kubectl rollout status deployment/hyperdx -n "${NAMESPACE}" --timeout=5m || true
    if kubectl get statefulset hyperdx-mongodb -n "${NAMESPACE}" >/dev/null 2>&1; then
      kubectl rollout status statefulset/hyperdx-mongodb -n "${NAMESPACE}" --timeout=5m || true
    fi
  fi

  cat <<EOF

🎯 Deployment summary (namespace: ${NAMESPACE})
- HyperDX enabled: ${HYPERDX_ENABLED}
- MongoDB service: $(if kubectl get svc hyperdx-mongodb -n "${NAMESPACE}" >/dev/null 2>&1; then echo "available"; else echo "not deployed"; fi)
- ClickStack endpoint: clickstack.${NAMESPACE}.svc.cluster.local:9000
- OTLP Gateway endpoint: otlp-gateway.${NAMESPACE}.svc.cluster.local:4317/4318

Next steps:
1. Port-forward HyperDX UI:
   kubectl port-forward -n ${NAMESPACE} svc/hyperdx 3000:3000
   Open http://localhost:3000 to complete onboarding.
2. Send telemetry (logs/traces/metrics) to otlp-gateway.${NAMESPACE}.svc.cluster.local:4317 (gRPC) or :4318 (HTTP).
3. Inspect collector output:
   kubectl logs -n ${NAMESPACE} deploy/otlp-gateway
   kubectl logs -n ${NAMESPACE} ds/otel-collector
4. Check MongoDB state:
   kubectl logs -n ${NAMESPACE} statefulset/hyperdx-mongodb

EOF
}

main "$@"
