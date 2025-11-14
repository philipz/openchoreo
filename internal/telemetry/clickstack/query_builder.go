// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clickstack

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/pkg/telemetry/query"
)

// QueryBuilder builds ClickHouse SQL statements for telemetry queries.
type QueryBuilder struct {
	logTable   string
	traceTable string
	maxLimit   int
}

// NewQueryBuilder creates a new builder instance.
func NewQueryBuilder(cfg config.ClickStackConfig) *QueryBuilder {
	const defaultMaxLimit = 2000
	max := defaultMaxLimit
	if cfg.MaxOpenConns > 0 && cfg.MaxOpenConns*100 > max {
		max = cfg.MaxOpenConns * 100
	}
	return &QueryBuilder{
		logTable:   cfg.LogsTable,
		traceTable: cfg.TracesTable,
		maxLimit:   max,
	}
}

// ComponentLogs builds the SQL for component log queries.
func (qb *QueryBuilder) ComponentLogs(params query.ComponentLogQuery) (string, []any, error) {
	if params.ComponentID == "" {
		return "", nil, errors.New("component id is required")
	}

	cb := newConditionBuilder()
	if err := qb.applyBaseFilters(cb, params.BaseLogQuery); err != nil {
		return "", nil, err
	}

	cb.add("component_id = ?", params.ComponentID)
	if params.EnvironmentID != "" {
		cb.add("environment_id = ?", params.EnvironmentID)
	}
	if params.BuildID != "" {
		cb.add("build_id = ?", params.BuildID)
	}
	if params.BuildUUID != "" {
		cb.add("build_uuid = ?", params.BuildUUID)
	}

	return qb.assembleLogQuery(cb, params.BaseLogQuery.SortOrder, qb.resolveLimit(params.BaseLogQuery.Limit))
}

// ProjectLogs builds the SQL for project level log queries.
func (qb *QueryBuilder) ProjectLogs(params query.ProjectLogQuery) (string, []any, error) {
	if params.ProjectID == "" {
		return "", nil, errors.New("project id is required")
	}

	cb := newConditionBuilder()
	if err := qb.applyBaseFilters(cb, params.BaseLogQuery); err != nil {
		return "", nil, err
	}
	cb.add("project_id = ?", params.ProjectID)

	if params.EnvironmentID != "" {
		cb.add("environment_id = ?", params.EnvironmentID)
	}

	if len(params.ComponentIDs) > 0 {
		qb.addInClause(cb, "component_id", params.ComponentIDs)
	}

	return qb.assembleLogQuery(cb, params.BaseLogQuery.SortOrder, qb.resolveLimit(params.BaseLogQuery.Limit))
}

// GatewayLogs builds SQL for gateway traffic queries.
func (qb *QueryBuilder) GatewayLogs(params query.GatewayLogQuery) (string, []any, error) {
	if params.OrganizationID == "" {
		return "", nil, errors.New("organization id is required")
	}

	cb := newConditionBuilder()
	if err := qb.applyBaseFilters(cb, params.BaseLogQuery); err != nil {
		return "", nil, err
	}
	cb.add("organization_id = ?", params.OrganizationID)
	if len(params.GatewayVHosts) > 0 {
		qb.addInClause(cb, "gateway_vhost", params.GatewayVHosts)
	}

	if len(params.APIIDToVersionMap) > 0 {
		var clauses []string
		var args []any
		keys := sortedKeys(params.APIIDToVersionMap)
		for _, apiID := range keys {
			version := params.APIIDToVersionMap[apiID]
			if version == "" {
				clauses = append(clauses, "(api_id = ?)")
				args = append(args, apiID)
			} else {
				clauses = append(clauses, "(api_id = ? AND api_version = ?)")
				args = append(args, apiID, version)
			}
		}
		cb.add("("+strings.Join(clauses, " OR ")+")", args...)
	}

	return qb.assembleLogQuery(cb, params.BaseLogQuery.SortOrder, qb.resolveLimit(params.BaseLogQuery.Limit))
}

// OrganizationLogs builds SQL for organization-wide queries.
func (qb *QueryBuilder) OrganizationLogs(params query.OrganizationLogQuery) (string, []any, error) {
	if params.OrganizationID == "" {
		return "", nil, errors.New("organization id is required")
	}

	cb := newConditionBuilder()
	if err := qb.applyBaseFilters(cb, params.BaseLogQuery); err != nil {
		return "", nil, err
	}
	cb.add("organization_id = ?", params.OrganizationID)
	if params.EnvironmentID != "" {
		cb.add("environment_id = ?", params.EnvironmentID)
	}

	if len(params.PodLabels) > 0 {
		keys := sortedKeys(params.PodLabels)
		for _, k := range keys {
			v := params.PodLabels[k]
			cb.add(fmt.Sprintf("JSONExtractString(labels_json, '%s') = ?", escapeJSONPath(k)), v)
		}
	}

	return qb.assembleLogQuery(cb, params.BaseLogQuery.SortOrder, qb.resolveLimit(params.BaseLogQuery.Limit))
}

// ComponentTraces builds SQL for component trace queries.
func (qb *QueryBuilder) ComponentTraces(params query.ComponentTraceQuery) (string, []any, error) {
	if params.ServiceName == "" {
		return "", nil, errors.New("service name is required")
	}
	if params.TimeRange.Start.IsZero() || params.TimeRange.End.IsZero() {
		return "", nil, errors.New("time range is required")
	}
	if qb.traceTable == "" {
		return "", nil, errors.New("trace table is not configured")
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > qb.maxLimit {
		limit = qb.maxLimit
	}

	sql := fmt.Sprintf(`SELECT
    start_time,
    end_time,
    span_name,
    span_id,
    trace_id,
    duration_in_nanos,
    count() OVER () AS total_count
FROM %s
WHERE service_name = ?
  AND start_time BETWEEN ? AND ?
ORDER BY start_time DESC
LIMIT ?`, qb.traceTable)

	args := []any{
		params.ServiceName,
		params.TimeRange.Start.UTC(),
		params.TimeRange.End.UTC(),
		limit,
	}

	return sql, args, nil
}

func (qb *QueryBuilder) applyBaseFilters(cb *conditionBuilder, base query.BaseLogQuery) error {
	if base.TimeRange.Start.IsZero() || base.TimeRange.End.IsZero() {
		return errors.New("time range is required")
	}

	cb.add("timestamp BETWEEN ? AND ?", base.TimeRange.Start.UTC(), base.TimeRange.End.UTC())

	if base.SearchPhrase != "" {
		cb.add("positionCaseInsensitive(log, ?) > 0", base.SearchPhrase)
	}

	if len(base.LogLevels) > 0 {
		qb.addInClause(cb, "log_level", base.LogLevels)
	}

	if base.Namespace != "" {
		cb.add("namespace = ?", base.Namespace)
	}

	if len(base.Versions) > 0 {
		qb.addInClause(cb, "version", base.Versions)
	}

	if len(base.VersionIDs) > 0 {
		qb.addInClause(cb, "version_id", base.VersionIDs)
	}

	if base.LogType != "" {
		cb.add("log_type = ?", base.LogType)
	}

	return nil
}

// CostReport builds SQL for cost aggregation
func (qb *QueryBuilder) CostReport(params query.CostReportQuery) (string, []any, error) {
	if qb.logTable == "" {
		return "", nil, errors.New("log table is not configured")
	}
	if params.Start.IsZero() || params.End.IsZero() {
		return "", nil, errors.New("cost report requires start and end time")
	}

	sql := fmt.Sprintf(`SELECT
    coalesce(JSONExtractString(labels_json, 'organization-name'), 'unknown') AS organization_id,
    coalesce(project_id, 'unknown') AS project_id,
    coalesce(component_id, 'unknown') AS component_id,
    count() AS log_count,
    sum(lengthUTF8(log)) AS raw_bytes
FROM %s
WHERE timestamp BETWEEN ? AND ?
GROUP BY organization_id, project_id, component_id
ORDER BY organization_id, project_id, component_id`, qb.logTable)

	args := []any{
		params.Start.UTC(),
		params.End.UTC(),
	}
	return sql, args, nil
}

func (qb *QueryBuilder) addInClause(cb *conditionBuilder, column string, values []string) {
	if len(values) == 0 {
		return
	}
	clause := fmt.Sprintf("%s IN (%s)", column, placeholders(len(values)))
	args := make([]any, len(values))
	for i, v := range values {
		args[i] = v
	}
	cb.add(clause, args...)
}

func (qb *QueryBuilder) assembleLogQuery(cb *conditionBuilder, order query.SortOrder, limit int) (string, []any, error) {
	if qb.logTable == "" {
		return "", nil, errors.New("log table is not configured")
	}
	where := cb.whereClause()
	sql := fmt.Sprintf(`SELECT
    timestamp,
    log,
    log_level,
    component_id,
    environment_id,
    project_id,
    version,
    version_id,
    namespace,
    pod_id,
    container_name,
    labels_json,
    count() OVER () AS total_count
FROM %s
WHERE %s
ORDER BY timestamp %s
LIMIT ?`, qb.logTable, where, qb.order(order))

	args := append(cb.args, limit)
	return sql, args, nil
}

func (qb *QueryBuilder) resolveLimit(limit int) int {
	if limit <= 0 {
		limit = 100
	}
	if limit > qb.maxLimit {
		return qb.maxLimit
	}
	return limit
}

func (qb *QueryBuilder) order(o query.SortOrder) string {
	if strings.EqualFold(string(o), string(query.SortAsc)) {
		return "ASC"
	}
	return "DESC"
}

type conditionBuilder struct {
	clauses []string
	args    []any
}

func newConditionBuilder() *conditionBuilder {
	return &conditionBuilder{
		clauses: make([]string, 0),
		args:    make([]any, 0),
	}
}

func (c *conditionBuilder) add(clause string, args ...any) {
	if clause == "" {
		return
	}
	c.clauses = append(c.clauses, clause)
	if len(args) > 0 {
		c.args = append(c.args, args...)
	}
}

func (c *conditionBuilder) whereClause() string {
	if len(c.clauses) == 0 {
		return "1=1"
	}
	return strings.Join(c.clauses, " AND ")
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

func sortedKeys[K ~string, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return keys
}

func escapeJSONPath(key string) string {
	return strings.ReplaceAll(key, `'`, `\'`)
}
