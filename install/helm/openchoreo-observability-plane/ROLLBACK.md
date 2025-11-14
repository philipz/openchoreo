# Migration Rollback Guide

This document provides detailed procedures for rolling back the OpenSearch → ClickStack migration at each phase.

## Table of Contents

1. [Rollback Decision Criteria](#rollback-decision-criteria)
2. [Phase-Specific Rollback Procedures](#phase-specific-rollback-procedures)
3. [Emergency Recovery](#emergency-recovery)
4. [Post-Rollback Validation](#post-rollback-validation)

## Rollback Decision Criteria

Consider rollback if you observe:

- **Data Loss**: Missing logs/traces when comparing OpenSearch vs ClickStack
- **High Error Rate**: >5% query errors for >5 minutes
- **Performance Degradation**: P95 latency >3x baseline for >10 minutes
- **ClickStack Outage**: Cluster unavailable for >15 minutes
- **Data Drift**: Validation shows >5% overall drift consistently
- **Stakeholder Request**: Product/engineering leadership requires rollback

## Phase-Specific Rollback Procedures

### Phase 1: Shadow Write Enabled (Dual-Write Active)

**Current State**: Both OpenSearch and ClickStack receiving data via dual-write.

**Rollback Steps**:

```bash
# 1. Disable shadow write
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set migration.shadowWrite.enabled=false \
  --wait

# 2. Verify only OpenSearch receiving data
kubectl logs -n openchoreo-observability-plane \
  deployment/openchoreo-observability-plane-gateway --tail=100 | grep exporter

# Should only see OpenSearch exporter active
```

**Impact**: None. OpenSearch remains primary backend throughout Phase 1.

**Recovery Time**: <5 minutes

**Validation**:
- ✅ OTLP gateway logs show only OpenSearch exporter
- ✅ No new data flowing to ClickStack (query counts stable)
- ✅ Queries continue working via Observer API

---

### Phase 2: Validation Running

**Current State**: Shadow write active, validation job sampling data consistency.

**Rollback Steps**:

Same as Phase 1 - simply disable shadow write.

```bash
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set migration.shadowWrite.enabled=false \
  --set migration.validation.enabled=false \
  --wait
```

**Impact**: None. OpenSearch still primary.

**Recovery Time**: <5 minutes

**Note**: Validation job will complete its current run but won't restart.

---

### Phase 3: Traffic Cutover (Observer Using ClickStack)

**Current State**: Observer API queries ClickStack, dual-write still active.

**Rollback Steps**:

```bash
# 1. Switch Observer back to OpenSearch
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set observer.telemetry.backend=opensearch \
  --set observer.telemetry.dualRead=false \
  --wait

# 2. Verify Observer is querying OpenSearch
kubectl exec -n openchoreo-observability-plane deployment/observer -it -- \
  curl localhost:8080/metrics | grep telemetry_backend

# Should show: telemetry_backend{type="opensearch"}

# 3. Test a query
kubectl exec -n openchoreo-observability-plane deployment/observer -it -- \
  curl localhost:8080/api/logs?component=gateway&tail=10
```

**Impact**:
- Brief API latency spike during pod restart (~30s)
- Queries may return cached results temporarily
- Backstage/CLI tools unaffected (backward compatible API)

**Recovery Time**: <2 minutes

**Keep Monitoring**:
- Leave dual-write active for 24-48 hours in case re-cutover is needed
- Monitor OpenSearch query performance

---

### Phase 4: Stability Monitoring (2-4 Weeks)

**Current State**: ClickStack primary for weeks, dual-write may be disabled.

**Rollback Steps**:

```bash
# 1. Re-enable dual-write if disabled
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set migration.shadowWrite.enabled=true \
  --wait

# 2. Wait for OpenSearch to catch up (5-10 minutes)
# Monitor OpenSearch index size growth

# 3. Switch Observer back to OpenSearch
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set observer.telemetry.backend=opensearch \
  --set observer.telemetry.dualRead=false \
  --wait

# 4. Disable ClickStack queries (optional safety)
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set clickstack.enabled=true \
  --set observer.telemetry.backend=opensearch \
  --wait
```

**Impact**:
- Data gap in OpenSearch if dual-write was off (requires backfill)
- Query performance returns to pre-migration baseline
- ClickStack resources still running (consuming resources)

**Recovery Time**: 10-30 minutes (depends on data gap)

**Backfill Procedure** (if dual-write was off):
```bash
# Export recent data from ClickStack
clickhouse-client --host clickstack --port 9000 \
  --query="SELECT * FROM telemetry.logs_mv WHERE timestamp >= NOW() - INTERVAL 24 HOUR FORMAT JSONEachRow" \
  > /tmp/clickstack_export.jsonl

# Transform and load into OpenSearch
# (Custom script needed - not automated)
./scripts/clickstack-to-opensearch.sh /tmp/clickstack_export.jsonl
```

---

### Phase 5: Post-Cleanup (OpenSearch Deleted)

**Current State**: OpenSearch resources removed, ClickStack is sole backend.

**⚠️ CRITICAL**: This is the most complex rollback scenario.

**Rollback Steps**:

```bash
# 1. Restore OpenSearch from Helm revision history
helm rollback openchoreo-observability-plane <previous-revision> \
  --namespace openchoreo-observability-plane \
  --wait

# Find revision before cleanup:
helm history openchoreo-observability-plane -n openchoreo-observability-plane

# 2. Verify OpenSearch pods are running
kubectl get pods -n openchoreo-observability-plane -l app.kubernetes.io/component=opensearch

# 3. If PVCs were deleted, restore from backup
# (Assumes archiveIndices=true was used)
kubectl apply -f /backup/opensearch/pvc-manifests.yaml

# 4. Restore data from archive
# Method depends on backup strategy (snapshot repository, S3, etc.)
curl -X POST "opensearch:9200/_snapshot/migration_backup/_all/_restore"

# 5. Wait for indices to restore (may take hours)
curl -X GET "opensearch:9200/_cat/indices?v"

# 6. Re-enable dual-write
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set migration.shadowWrite.enabled=true \
  --set observer.telemetry.backend=opensearch \
  --wait

# 7. Verify data completeness
# Compare record counts between backup and restored OpenSearch
```

**Impact**:
- **SEVERE**: Multi-hour outage for observability queries
- Data gap from cleanup time to restoration (~2-24 hours)
- Potential data loss if backups incomplete
- Requires manual intervention

**Recovery Time**: 2-8 hours (depends on data volume)

**Prevention**:
- Always set `migration.cleanup.confirmed=false` initially
- Keep `migration.cleanup.deletePVCs=false` for 4+ weeks
- Verify backups before confirming cleanup

---

## Emergency Recovery

### Scenario: Complete ClickStack Failure (All Replicas Down)

**Symptoms**:
- ClickStack pods in CrashLoopBackOff
- Query errors 100%
- No logs/traces visible in HyperDX

**Immediate Actions**:

```bash
# 1. Check ClickStack pod status
kubectl get pods -n openchoreo-observability-plane -l app.kubernetes.io/component=clickstack

# 2. View recent logs
kubectl logs -n openchoreo-observability-plane \
  statefulset/clickstack --tail=100 --all-containers=true

# 3. If pods can't start, immediately switch to OpenSearch
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set observer.telemetry.backend=opensearch \
  --wait --timeout=2m

# 4. Verify queries work via OpenSearch
kubectl exec -n openchoreo-observability-plane deployment/observer -it -- \
  curl localhost:8080/api/logs?tail=5
```

**Recovery Time**: 2-5 minutes

---

### Scenario: Data Corruption in ClickStack

**Symptoms**:
- Queries return incorrect results
- Missing time ranges
- Duplicate entries

**Immediate Actions**:

```bash
# 1. Switch to OpenSearch immediately
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set observer.telemetry.backend=opensearch \
  --wait

# 2. Stop writes to ClickStack
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --namespace openchoreo-observability-plane \
  --set migration.shadowWrite.enabled=false \
  --wait

# 3. Investigate corruption
kubectl exec -n openchoreo-observability-plane statefulset/clickstack-0 -it -- clickhouse-client

# Run diagnostic queries:
SELECT
  toStartOfHour(timestamp) as hour,
  count() as records,
  uniq(component_id) as unique_components
FROM telemetry.logs_mv
WHERE timestamp >= NOW() - INTERVAL 24 HOUR
GROUP BY hour
ORDER BY hour;

# 4. If corruption confirmed, drop and rebuild tables
# WARNING: Data loss! Only if OpenSearch has all data.
DROP TABLE IF EXISTS telemetry.logs_mv;
# Re-run schema init job
kubectl delete job clickstack-init -n openchoreo-observability-plane
helm upgrade openchoreo-observability-plane ./install/helm/openchoreo-observability-plane \
  --set clickstack.initSQL.enabled=true \
  --wait
```

---

## Post-Rollback Validation

After any rollback, validate the following:

### 1. Data Availability
```bash
# Check recent logs exist
kubectl exec -n openchoreo-observability-plane deployment/observer -it -- \
  curl 'localhost:8080/api/logs?tail=100' | jq '.logs | length'

# Should return 100
```

### 2. Query Performance
```bash
# Measure query latency (should be <2s)
time kubectl exec -n openchoreo-observability-plane deployment/observer -it -- \
  curl 'localhost:8080/api/logs?component=gateway&since=1h&tail=1000'
```

### 3. Data Completeness
```bash
# Compare counts (within 1% acceptable)
OS_COUNT=$(curl -s "opensearch:9200/kubernetes-*/_count" | jq '.count')
echo "OpenSearch records: $OS_COUNT"

# If ClickStack still running:
CH_COUNT=$(clickhouse-client --query="SELECT count() FROM telemetry.logs_mv")
echo "ClickStack records: $CH_COUNT"
```

### 4. Integration Health
```bash
# Test Backstage portal (manual browser check)
# Test CLI tool
choreoctl logs --component gateway --tail 10

# Verify HyperDX/Grafana (if applicable)
```

### 5. Alert Validation
```bash
# Ensure no active alerts
kubectl exec -n openchoreo-observability-plane deployment/prometheus -it -- \
  curl localhost:9090/api/v1/alerts | jq '.data.alerts[] | select(.state=="firing")'
```

---

## Rollback Decision Matrix

| Phase | Rollback Complexity | Recovery Time | Data Loss Risk | Recommended Action |
|-------|-------------------|---------------|----------------|-------------------|
| 1-2 (Shadow Write) | ⭐ Low | <5 min | None | Safe to rollback anytime |
| 3 (Cutover) | ⭐⭐ Medium | <2 min | None (dual-write active) | Rollback if errors >5% |
| 4 (Stability) | ⭐⭐⭐ Medium-High | 10-30 min | Possible if dual-write off | Rollback if critical issue |
| 5 (Post-Cleanup) | ⭐⭐⭐⭐⭐ Very High | 2-8 hours | High risk | Avoid if possible, restore from backup |

---

## Support and Escalation

If rollback fails or data loss occurs:

1. **Preserve Evidence**: Capture all logs and metrics immediately
2. **Stop Changes**: Do not apply further Helm upgrades
3. **Escalate**: Contact platform engineering team
4. **Document**: Record timeline of events and actions taken

**Emergency Contacts**:
- Platform Team: platform-team@openchoreo.dev
- On-Call: Use PagerDuty escalation policy

---

## Appendix: Pre-Rollback Checklist

Before executing any rollback:

- [ ] Identify the current migration phase
- [ ] Determine root cause of issues (if known)
- [ ] Verify OpenSearch cluster is healthy
- [ ] Check available disk space (>30% free)
- [ ] Notify stakeholders of planned rollback
- [ ] Backup current ClickStack data (if needed)
- [ ] Have Helm history available (`helm history ...`)
- [ ] Ensure cluster access (kubectl working)
- [ ] Have monitoring dashboards open
- [ ] Assign operator to monitor rollback execution

---

**Last Updated**: 2025-11-13
**Maintained By**: OpenChoreo Platform Team
