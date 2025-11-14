#!/usr/bin/env bash

set -euo pipefail

NAMESPACE="openchoreo-observability-plane"
DATA_NAMESPACE=""

print_usage() {
  cat <<USAGE
Usage: $(basename "$0") [-n namespace] [--data-namespace ns]

Options:
  -n, --namespace         Namespace where the observability plane lives (default: openchoreo-observability-plane)
  --data-namespace        Optional application namespace (e.g., dp-default-default-development) to filter sample ClickHouse queries

Example:
  $(basename "$0") --data-namespace dp-default-default-development
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -n|--namespace)
      NAMESPACE="$2"
      shift 2
      ;;
    --data-namespace)
      DATA_NAMESPACE="$2"
      shift 2
      ;;
    -h|--help)
      print_usage
      exit 0
      ;;
    *)
      echo "Unknown flag: $1" >&2
      print_usage >&2
      exit 1
      ;;
  esac
done

echo "🔍 Verifying ClickStack + HyperDX in namespace '${NAMESPACE}'"

kubectl get pods -n "$NAMESPACE"

echo "\nChecking core components..."
kubectl get statefulset/clickstack -n "$NAMESPACE"
kubectl get deployment/otlp-gateway -n "$NAMESPACE"
kubectl get deployment/observer -n "$NAMESPACE"

if kubectl get deployment/hyperdx -n "$NAMESPACE" >/dev/null 2>&1; then
  kubectl get deployment/hyperdx -n "$NAMESPACE"
  kubectl get statefulset/hyperdx-mongodb -n "$NAMESPACE"
  echo "HyperDX service is available; port-forward with: kubectl port-forward -n ${NAMESPACE} svc/hyperdx 3000:3000"
else
  echo "⚠️  HyperDX deployment not found in namespace ${NAMESPACE}"
fi

echo "\nRecent OTLP gateway logs (grep for HTTP ingest):"
kubectl logs -n "$NAMESPACE" deploy/otlp-gateway --tail=40 || true

echo "\nRecent collector logs (node agent):"
kubectl logs -n "$NAMESPACE" ds/otel-collector --tail=20 || true

if kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=clickstack -o name >/dev/null 2>&1; then
  CLICKSTACK_POD=$(kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=clickstack -o jsonpath='{.items[0].metadata.name}')
  echo "\nClickStack pod detected: $CLICKSTACK_POD"
  if kubectl exec -n "$NAMESPACE" "$CLICKSTACK_POD" -- which clickhouse-client >/dev/null 2>&1; then
    echo "Running health query against telemetry.logs_mv ..."
    kubectl exec -n "$NAMESPACE" "$CLICKSTACK_POD" -- \
      clickhouse-client --query "SELECT count() AS total_logs FROM telemetry.logs_mv LIMIT 1" || true
    if [[ -n "$DATA_NAMESPACE" ]]; then
      echo "\nFiltering recent records for namespace '${DATA_NAMESPACE}' (last 15 minutes)"
      kubectl exec -n "$NAMESPACE" "$CLICKSTACK_POD" -- \
        clickhouse-client --query "SELECT count() FROM telemetry.logs_mv WHERE namespace LIKE '${DATA_NAMESPACE}%' AND timestamp > now() - INTERVAL 15 MINUTE" || true
      kubectl exec -n "$NAMESPACE" "$CLICKSTACK_POD" -- \
        clickhouse-client --query "SELECT count() FROM telemetry.traces_mv WHERE service_name LIKE '${DATA_NAMESPACE}%' AND end_time > now() - INTERVAL 15 MINUTE" || true
    fi
  else
    echo "clickhouse-client binary not found in $CLICKSTACK_POD"
  fi
else
  echo "⚠️  Unable to find clickstack pod in namespace ${NAMESPACE}"
fi

echo "\n✅ Verification complete"
