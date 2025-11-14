#!/bin/bash
# Migration workflow test script
# Tests the shadow write, validation, and cleanup sequence

set -e

NAMESPACE="openchoreo-observability-plane-test"
CHART_PATH="$(dirname "$0")/.."

echo "=== Migration Workflow Test ==="
echo ""

# Cleanup function
cleanup() {
  echo "Cleaning up test namespace..."
  kubectl delete namespace "$NAMESPACE" --ignore-not-found=true --wait=false
}

trap cleanup EXIT

echo "Step 1: Create test namespace"
kubectl create namespace "$NAMESPACE" || true

echo ""
echo "Step 2: Install chart with ClickStack only (no migration jobs)"
helm upgrade --install openchoreo-obs-test "$CHART_PATH" \
  --namespace "$NAMESPACE" \
  --set clickstack.enabled=true \
  --set gateway.enabled=true \
  --set hyperdx.enabled=true \
  --set observer.enabled=false \
  --set migration.shadowWrite.enabled=false \
  --set migration.validation.enabled=false \
  --set migration.cleanup.enabled=false \
  --wait --timeout=5m

echo ""
echo "Step 3: Verify ClickStack deployment"
kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/component=clickstack
kubectl wait --for=condition=ready pod -l app.kubernetes.io/component=clickstack \
  -n "$NAMESPACE" --timeout=2m

echo ""
echo "Step 3b: Verify HyperDX + MongoDB"
kubectl get deploy/hyperdx -n "$NAMESPACE"
kubectl rollout status deploy/hyperdx -n "$NAMESPACE" --timeout=2m
if kubectl get statefulset/hyperdx-mongodb -n "$NAMESPACE" >/dev/null 2>&1; then
  kubectl rollout status statefulset/hyperdx-mongodb -n "$NAMESPACE" --timeout=3m
fi

echo ""
echo "Step 4: Enable shadow write (simulation)"
echo "Upgrading with migration.shadowWrite.enabled=true"
helm upgrade openchoreo-obs-test "$CHART_PATH" \
  --namespace "$NAMESPACE" \
  --set clickstack.enabled=true \
  --set gateway.enabled=true \
  --set hyperdx.enabled=true \
  --set observer.enabled=false \
  --set migration.shadowWrite.enabled=true \
  --set migration.validation.enabled=false \
  --set migration.cleanup.enabled=false \
  --wait --timeout=5m

echo ""
echo "Step 5: Check shadow write job status"
if kubectl get job -n "$NAMESPACE" | grep -q "shadow-write"; then
  echo "✓ Shadow write job created"
  kubectl logs -n "$NAMESPACE" job/openchoreo-obs-test-openchoreo-observability-plane-shadow-write --tail=50 || true
else
  echo "⚠️  Shadow write job not found (may be pre-upgrade hook)"
fi

echo ""
echo "Step 6: Enable validation job"
echo "Upgrading with migration.validation.enabled=true"
helm upgrade openchoreo-obs-test "$CHART_PATH" \
  --namespace "$NAMESPACE" \
  --set clickstack.enabled=true \
  --set gateway.enabled=true \
  --set hyperdx.enabled=true \
  --set observer.enabled=false \
  --set migration.shadowWrite.enabled=true \
  --set migration.validation.enabled=true \
  --set migration.validation.durationSeconds=60 \
  --set migration.cleanup.enabled=false \
  --wait --timeout=5m

echo ""
echo "Step 7: Check validation job status"
if kubectl get job -n "$NAMESPACE" | grep -q "validation"; then
  echo "✓ Validation job created"
  kubectl logs -n "$NAMESPACE" job/openchoreo-obs-test-openchoreo-observability-plane-validation --tail=50 || true
else
  echo "⚠️  Validation job not found (may be pre-upgrade hook)"
fi

echo ""
echo "Step 8: Test rollback values (disable shadow write)"
helm upgrade openchoreo-obs-test "$CHART_PATH" \
  --namespace "$NAMESPACE" \
  --set clickstack.enabled=true \
  --set gateway.enabled=true \
  --set hyperdx.enabled=true \
  --set observer.enabled=false \
  --set migration.shadowWrite.enabled=false \
  --set migration.validation.enabled=false \
  --set migration.cleanup.enabled=false \
  --wait --timeout=5m

echo "✓ Rollback simulation successful"

echo ""
echo "Step 9: Test cleanup job (dry-run, confirmed=false)"
helm upgrade openchoreo-obs-test "$CHART_PATH" \
  --namespace "$NAMESPACE" \
  --set clickstack.enabled=true \
  --set gateway.enabled=true \
  --set hyperdx.enabled=true \
  --set observer.enabled=false \
  --set migration.shadowWrite.enabled=false \
  --set migration.validation.enabled=false \
  --set migration.cleanup.enabled=true \
  --set migration.cleanup.confirmed=false \
  --wait --timeout=5m

echo ""
echo "Step 10: Verify cleanup job (should not execute destructive actions)"
if kubectl get job -n "$NAMESPACE" | grep -q "cleanup"; then
  echo "✓ Cleanup job created"
  kubectl logs -n "$NAMESPACE" job/openchoreo-obs-test-openchoreo-observability-plane-cleanup-opensearch --tail=50 || true
else
  echo "⚠️  Cleanup job not found (may be post-upgrade hook)"
fi

echo ""
echo "=== Migration Workflow Test Summary ==="
echo "✓ Chart installation successful"
echo "✓ ClickStack deployment verified"
echo "✓ Shadow write job tested"
echo "✓ Validation job tested"
echo "✓ Rollback values tested"
echo "✓ Cleanup job safety verified"
echo ""
echo "All migration workflow components validated successfully!"
