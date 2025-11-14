// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/pkg/telemetry/query"
)

// OpenSearchClient interface for testing
type OpenSearchClient interface {
	Search(ctx context.Context, indices []string, query map[string]interface{}) (*opensearch.SearchResponse, error)
	GetIndexMapping(ctx context.Context, index string) (*opensearch.MappingResponse, error)
	HealthCheck(ctx context.Context) error
}

// LoggingService provides logging functionality
type LoggingService struct {
	osClient     OpenSearchClient
	storage      query.StorageProvider
	queryBuilder *opensearch.QueryBuilder
	config       *config.Config
	logger       *slog.Logger
	dualSampler  *dualSampler
	hyperdx      *HyperDXSigner
}

// LogResponse represents the response structure for log queries
type LogResponse struct {
	Logs       []opensearch.LogEntry `json:"logs"`
	TotalCount int                   `json:"totalCount"`
	Took       int                   `json:"tookMs"`
}

// NewLoggingService creates a new logging service instance
func NewLoggingService(storage query.StorageProvider, osClient OpenSearchClient, cfg *config.Config, logger *slog.Logger) *LoggingService {
	if logger == nil {
		logger = slog.Default()
	}

	return &LoggingService{
		osClient:     osClient,
		storage:      storage,
		queryBuilder: opensearch.NewQueryBuilder(cfg.OpenSearch.IndexPrefix),
		config:       cfg,
		logger:       logger,
		dualSampler:  newDualSampler(cfg.Telemetry, logger),
		hyperdx:      newHyperDXSigner(cfg.Telemetry.HyperDX),
	}
}

// GetComponentLogs retrieves logs for a specific component using V2 wildcard search
func (s *LoggingService) GetComponentLogs(ctx context.Context, params opensearch.ComponentQueryParams) (*LogResponse, error) {
	if s.useClickStack() {
		resp, err := s.getComponentLogsFromClickStack(ctx, params)
		if err != nil {
			return nil, err
		}
		s.maybeDualReadComponentLogs(ctx, params, resp)
		return resp, nil
	}
	return s.getComponentLogsFromOpenSearch(ctx, params)
}

// GetProjectLogs retrieves logs for a specific project using V2 wildcard search
func (s *LoggingService) GetProjectLogs(ctx context.Context, params opensearch.QueryParams, componentIDs []string) (*LogResponse, error) {
	if s.useClickStack() {
		resp, err := s.getProjectLogsFromClickStack(ctx, params, componentIDs)
		if err != nil {
			return nil, err
		}
		s.maybeDualReadProjectLogs(ctx, params, componentIDs, resp)
		return resp, nil
	}
	return s.getProjectLogsFromOpenSearch(ctx, params, componentIDs)
}

// GetGatewayLogs retrieves gateway logs using V2 wildcard search
func (s *LoggingService) GetGatewayLogs(ctx context.Context, params opensearch.GatewayQueryParams) (*LogResponse, error) {
	if s.useClickStack() {
		resp, err := s.getGatewayLogsFromClickStack(ctx, params)
		if err != nil {
			return nil, err
		}
		s.maybeDualReadGatewayLogs(ctx, params, resp)
		return resp, nil
	}
	return s.getGatewayLogsFromOpenSearch(ctx, params)
}

// GetOrganizationLogs retrieves logs for an organization with custom filters
func (s *LoggingService) GetOrganizationLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (*LogResponse, error) {
	if s.useClickStack() {
		resp, err := s.getOrganizationLogsFromClickStack(ctx, params, podLabels)
		if err != nil {
			return nil, err
		}
		s.maybeDualReadOrganizationLogs(ctx, params, podLabels, resp)
		return resp, nil
	}
	return s.getOrganizationLogsFromOpenSearch(ctx, params, podLabels)
}

func (s *LoggingService) GetComponentTraces(ctx context.Context, params opensearch.ComponentTracesRequestParams) (*opensearch.TraceResponse, error) {
	if s.useClickStack() {
		resp, err := s.getComponentTracesFromClickStack(ctx, params)
		if err != nil {
			return nil, err
		}
		s.maybeDualReadComponentTraces(ctx, params, resp)
		return resp, nil
	}
	return s.getComponentTracesFromOpenSearch(ctx, params)
}

// HealthCheck performs a health check on the service
func (s *LoggingService) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if s.useClickStack() {
		if err := s.storage.HealthCheck(ctx); err != nil {
			s.logger.Error("ClickStack health check failed", "error", err)
			return fmt.Errorf("clickstack health check failed: %w", err)
		}
		if s.dualSampler.enabled && s.osClient != nil {
			if err := s.osClient.HealthCheck(ctx); err != nil {
				s.logger.Warn("OpenSearch dual-read health check failed", "error", err)
			}
		}
	} else if s.osClient != nil {
		if err := s.osClient.HealthCheck(ctx); err != nil {
			s.logger.Error("Health check failed", "error", err)
			return fmt.Errorf("opensearch health check failed: %w", err)
		}
	}

	s.logger.Debug("Health check passed")
	return nil
}

// GenerateHyperDXLink returns a signed HyperDX URL if signing is configured.
func (s *LoggingService) GenerateHyperDXLink(path string, params map[string]string) (string, error) {
	if s.hyperdx == nil {
		return "", errors.New("hyperdx signing is not configured")
	}
	return s.hyperdx.Generate(path, params)
}

// GenerateCostReportCSV aggregates usage data and returns CSV formatted results.
func (s *LoggingService) GenerateCostReportCSV(ctx context.Context, params query.CostReportQuery) (string, error) {
	if !s.supportsCostReporting() {
		return "", errors.New("cost reporting requires ClickStack telemetry backend")
	}

	report, err := s.storage.GetCostReport(ctx, params)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{"organization_id", "project_id", "component_id", "log_count", "estimated_storage_bytes", "estimated_cost_usd"}
	if err := writer.Write(header); err != nil {
		return "", err
	}

	for _, row := range report.Rows {
		record := []string{
			row.OrganizationID,
			row.ProjectID,
			row.ComponentID,
			strconv.Itoa(row.LogCount),
			fmt.Sprintf("%.0f", row.EstimatedStorageB),
			fmt.Sprintf("%.4f", row.EstimatedCostUSD),
		}
		if err := writer.Write(record); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// ExportLogsToCSV retrieves logs and exports them as CSV
func (s *LoggingService) ExportLogsToCSV(ctx context.Context, params opensearch.QueryParams, componentIDs []string, podLabels map[string]string) ([]byte, error) {
	var (
		logResponse *LogResponse
		err         error
	)

	if len(componentIDs) > 0 {
		logResponse, err = s.GetProjectLogs(ctx, params, componentIDs)
	} else if len(podLabels) > 0 {
		logResponse, err = s.GetOrganizationLogs(ctx, params, podLabels)
	} else {
		// Default to project logs if no specific filters are provided, or handle as an error
		return nil, errors.New("either componentIDs or podLabels must be provided for CSV export")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve logs for CSV export: %w", err)
	}

	if logResponse == nil || len(logResponse.Logs) == 0 {
		return []byte(""), nil // Return empty CSV if no logs
	}

	var b bytes.Buffer
	writer := csv.NewWriter(&b)

	// Write CSV header
	header := []string{"Timestamp", "LogLevel", "ComponentID", "EnvironmentID", "ProjectID", "Namespace", "PodID", "ContainerName", "Log"}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write log entries
	for _, logEntry := range logResponse.Logs {
		record := []string{
			logEntry.Timestamp.Format(time.RFC3339),
			logEntry.LogLevel,
			logEntry.ComponentID,
			logEntry.EnvironmentID,
			logEntry.ProjectID,
			logEntry.Namespace,
			logEntry.PodID,
			logEntry.ContainerName,
			logEntry.Log,
		}
		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	return b.Bytes(), nil
}

func (s *LoggingService) useClickStack() bool {
	return s.storage != nil && strings.EqualFold(s.config.Telemetry.Backend, "clickstack")
}
func (s *LoggingService) supportsCostReporting() bool {
	return s.storage != nil && s.useClickStack()
}

// --- OpenSearch helpers remain mostly unchanged ---

func (s *LoggingService) getComponentLogsFromOpenSearch(ctx context.Context, params opensearch.ComponentQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting component logs",
		"component_id", params.ComponentID,
		"environment_id", params.EnvironmentID,
		"search_phrase", params.SearchPhrase)

	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	queryBody := s.queryBuilder.BuildComponentLogsQuery(params)
	response, err := s.osClient.Search(ctx, indices, queryBody)
	if err != nil {
		s.logger.Error("Failed to execute component logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	return parseOpenSearchLogs(response), nil
}

func (s *LoggingService) getProjectLogsFromOpenSearch(ctx context.Context, params opensearch.QueryParams, componentIDs []string) (*LogResponse, error) {
	s.logger.Info("Getting project logs",
		"project_id", params.ProjectID,
		"environment_id", params.EnvironmentID,
		"component_ids", componentIDs,
		"search_phrase", params.SearchPhrase)

	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	queryBody := s.queryBuilder.BuildProjectLogsQuery(params, componentIDs)
	response, err := s.osClient.Search(ctx, indices, queryBody)
	if err != nil {
		s.logger.Error("Failed to execute project logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	return parseOpenSearchLogs(response), nil
}

func (s *LoggingService) getGatewayLogsFromOpenSearch(ctx context.Context, params opensearch.GatewayQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting gateway logs",
		"organization_id", params.OrganizationID,
		"gateway_vhosts", params.GatewayVHosts,
		"search_phrase", params.SearchPhrase)

	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	queryBody := s.queryBuilder.BuildGatewayLogsQuery(params)
	response, err := s.osClient.Search(ctx, indices, queryBody)
	if err != nil {
		s.logger.Error("Failed to execute gateway logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	return parseOpenSearchLogs(response), nil
}

func (s *LoggingService) getOrganizationLogsFromOpenSearch(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (*LogResponse, error) {
	s.logger.Info("Getting organization logs",
		"organization_id", params.OrganizationID,
		"environment_id", params.EnvironmentID,
		"pod_labels", podLabels,
		"search_phrase", params.SearchPhrase)

	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	queryBody := s.queryBuilder.BuildOrganizationLogsQuery(params, podLabels)
	response, err := s.osClient.Search(ctx, indices, queryBody)
	if err != nil {
		s.logger.Error("Failed to execute organization logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	return parseOpenSearchLogs(response), nil
}

func (s *LoggingService) getComponentTracesFromOpenSearch(ctx context.Context, params opensearch.ComponentTracesRequestParams) (*opensearch.TraceResponse, error) {
	s.logger.Info("Getting component traces",
		"serviceName", params.ServiceName)

	queryBody := s.queryBuilder.BuildComponentTracesQuery(params)
	response, err := s.osClient.Search(ctx, []string{"otel-v1-apm-span"}, queryBody)
	if err != nil {
		s.logger.Error("Failed to execute component traces search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	return parseOpenSearchTraces(response), nil
}

func parseOpenSearchLogs(response *opensearch.SearchResponse) *LogResponse {
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}
}

func parseOpenSearchTraces(response *opensearch.SearchResponse) *opensearch.TraceResponse {
	traces := make([]opensearch.Span, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		span := opensearch.ParseSpanEntry(hit)
		traces = append(traces, span)
	}

	return &opensearch.TraceResponse{
		Spans:      traces,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}
}

// --- ClickStack helpers ---

func (s *LoggingService) getComponentLogsFromClickStack(ctx context.Context, params opensearch.ComponentQueryParams) (*LogResponse, error) {
	queryParams, err := buildComponentLogQuery(params)
	if err != nil {
		return nil, err
	}
	result, err := s.storage.GetComponentLogs(ctx, queryParams)
	if err != nil {
		return nil, err
	}
	return convertLogResult(result), nil
}

func (s *LoggingService) getProjectLogsFromClickStack(ctx context.Context, params opensearch.QueryParams, componentIDs []string) (*LogResponse, error) {
	queryParams, err := buildProjectLogQuery(params, componentIDs)
	if err != nil {
		return nil, err
	}
	result, err := s.storage.GetProjectLogs(ctx, queryParams)
	if err != nil {
		return nil, err
	}
	return convertLogResult(result), nil
}

func (s *LoggingService) getGatewayLogsFromClickStack(ctx context.Context, params opensearch.GatewayQueryParams) (*LogResponse, error) {
	queryParams, err := buildGatewayLogQuery(params)
	if err != nil {
		return nil, err
	}
	result, err := s.storage.GetGatewayLogs(ctx, queryParams)
	if err != nil {
		return nil, err
	}
	return convertLogResult(result), nil
}

func (s *LoggingService) getOrganizationLogsFromClickStack(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (*LogResponse, error) {
	queryParams, err := buildOrganizationLogQuery(params, podLabels)
	if err != nil {
		return nil, err
	}
	result, err := s.storage.GetOrganizationLogs(ctx, queryParams)
	if err != nil {
		return nil, err
	}
	return convertLogResult(result), nil
}

func (s *LoggingService) getComponentTracesFromClickStack(ctx context.Context, params opensearch.ComponentTracesRequestParams) (*opensearch.TraceResponse, error) {
	traceQuery, err := buildComponentTraceQuery(params)
	if err != nil {
		return nil, err
	}
	result, err := s.storage.GetComponentTraces(ctx, traceQuery)
	if err != nil {
		return nil, err
	}
	return convertTraceResult(result), nil
}

func convertLogResult(result *query.LogResult) *LogResponse {
	logs := make([]opensearch.LogEntry, 0, len(result.Logs))
	for _, record := range result.Logs {
		entry := opensearch.LogEntry{
			Timestamp:     record.Timestamp,
			Log:           record.Log,
			LogLevel:      record.LogLevel,
			ComponentID:   record.ComponentID,
			EnvironmentID: record.EnvironmentID,
			ProjectID:     record.ProjectID,
			Version:       record.Version,
			VersionID:     record.VersionID,
			Namespace:     record.Namespace,
			PodID:         record.PodID,
			ContainerName: record.ContainerName,
			Labels:        record.Labels,
		}
		logs = append(logs, entry)
	}

	return &LogResponse{
		Logs:       logs,
		TotalCount: result.TotalCount,
		Took:       result.Took,
	}
}

func convertTraceResult(result *query.TraceResult) *opensearch.TraceResponse {
	spans := make([]opensearch.Span, 0, len(result.Spans))
	for _, span := range result.Spans {
		spans = append(spans, opensearch.Span{
			DurationInNanos: span.DurationInNanos,
			EndTime:         span.EndTime,
			Name:            span.Name,
			SpanID:          span.SpanID,
			StartTime:       span.StartTime,
			TraceID:         span.TraceID,
		})
	}

	return &opensearch.TraceResponse{
		Spans:      spans,
		TotalCount: result.TotalCount,
		Took:       result.Took,
	}
}

// Dual read helpers ----------------------------------------------------------

func (s *LoggingService) maybeDualReadComponentLogs(ctx context.Context, params opensearch.ComponentQueryParams, primary *LogResponse) {
	if !s.shouldDualRead() {
		return
	}
	secondary, err := s.getComponentLogsFromOpenSearch(ctx, params)
	s.emitDualReadResult("component", err, primary, secondary)
}

func (s *LoggingService) maybeDualReadProjectLogs(ctx context.Context, params opensearch.QueryParams, componentIDs []string, primary *LogResponse) {
	if !s.shouldDualRead() {
		return
	}
	secondary, err := s.getProjectLogsFromOpenSearch(ctx, params, componentIDs)
	s.emitDualReadResult("project", err, primary, secondary)
}

func (s *LoggingService) maybeDualReadGatewayLogs(ctx context.Context, params opensearch.GatewayQueryParams, primary *LogResponse) {
	if !s.shouldDualRead() {
		return
	}
	secondary, err := s.getGatewayLogsFromOpenSearch(ctx, params)
	s.emitDualReadResult("gateway", err, primary, secondary)
}

func (s *LoggingService) maybeDualReadOrganizationLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string, primary *LogResponse) {
	if !s.shouldDualRead() {
		return
	}
	secondary, err := s.getOrganizationLogsFromOpenSearch(ctx, params, podLabels)
	s.emitDualReadResult("organization", err, primary, secondary)
}

func (s *LoggingService) maybeDualReadComponentTraces(ctx context.Context, params opensearch.ComponentTracesRequestParams, primary *opensearch.TraceResponse) {
	if !s.shouldDualRead() {
		return
	}
	secondary, err := s.getComponentTracesFromOpenSearch(ctx, params)
	if err != nil || secondary == nil {
		if err != nil {
			s.logger.Warn("dual-read trace fetch failed", "error", err)
		}
		return
	}
	if primary.TotalCount != secondary.TotalCount {
		s.logger.Warn("dual-read trace mismatch",
			"primary_total", primary.TotalCount,
			"secondary_total", secondary.TotalCount)
	}
}

func (s *LoggingService) emitDualReadResult(scope string, err error, primary, secondary *LogResponse) {
	if err != nil {
		s.logger.Warn("dual-read OpenSearch fetch failed", "scope", scope, "error", err)
		return
	}
	if secondary == nil {
		return
	}
	if primary.TotalCount != secondary.TotalCount || len(primary.Logs) != len(secondary.Logs) {
		s.logger.Warn("dual-read mismatch",
			"scope", scope,
			"primary_total", primary.TotalCount,
			"secondary_total", secondary.TotalCount,
			"primary_len", len(primary.Logs),
			"secondary_len", len(secondary.Logs))
	}
}

func (s *LoggingService) shouldDualRead() bool {
	return s.dualSampler != nil && s.dualSampler.enabled && s.osClient != nil && s.dualSampler.shouldSample()
}

type dualSampler struct {
	enabled bool
	rate    float64
	rng     *rand.Rand
	mu      sync.Mutex
	logger  *slog.Logger
}

func newDualSampler(cfg config.TelemetryConfig, logger *slog.Logger) *dualSampler {
	if !cfg.DualRead || cfg.DualSampleRate <= 0 {
		return &dualSampler{enabled: false}
	}
	source := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &dualSampler{
		enabled: true,
		rate:    cfg.DualSampleRate,
		rng:     source,
		logger:  logger,
	}
}

func (d *dualSampler) shouldSample() bool {
	if !d.enabled {
		return false
	}
	if d.rate >= 1 {
		return true
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rng.Float64() < d.rate
}

// --- Query conversion helpers ------------------------------------------------

func buildComponentLogQuery(params opensearch.ComponentQueryParams) (query.ComponentLogQuery, error) {
	base, err := buildBaseLogQuery(params.QueryParams)
	if err != nil {
		return query.ComponentLogQuery{}, err
	}
	return query.ComponentLogQuery{
		BaseLogQuery:  base,
		ComponentID:   params.ComponentID,
		EnvironmentID: params.EnvironmentID,
		BuildID:       params.BuildID,
		BuildUUID:     params.BuildUUID,
	}, nil
}

func buildProjectLogQuery(params opensearch.QueryParams, componentIDs []string) (query.ProjectLogQuery, error) {
	base, err := buildBaseLogQuery(params)
	if err != nil {
		return query.ProjectLogQuery{}, err
	}
	return query.ProjectLogQuery{
		BaseLogQuery:  base,
		ProjectID:     params.ProjectID,
		ComponentIDs:  componentIDs,
		EnvironmentID: params.EnvironmentID,
	}, nil
}

func buildGatewayLogQuery(params opensearch.GatewayQueryParams) (query.GatewayLogQuery, error) {
	base, err := buildBaseLogQuery(params.QueryParams)
	if err != nil {
		return query.GatewayLogQuery{}, err
	}
	return query.GatewayLogQuery{
		BaseLogQuery:      base,
		OrganizationID:    params.OrganizationID,
		APIIDToVersionMap: params.APIIDToVersionMap,
		GatewayVHosts:     params.GatewayVHosts,
	}, nil
}

func buildOrganizationLogQuery(params opensearch.QueryParams, podLabels map[string]string) (query.OrganizationLogQuery, error) {
	base, err := buildBaseLogQuery(params)
	if err != nil {
		return query.OrganizationLogQuery{}, err
	}
	return query.OrganizationLogQuery{
		BaseLogQuery:   base,
		OrganizationID: params.OrganizationID,
		EnvironmentID:  params.EnvironmentID,
		PodLabels:      podLabels,
	}, nil
}

func buildComponentTraceQuery(params opensearch.ComponentTracesRequestParams) (query.ComponentTraceQuery, error) {
	timeRange, err := buildTimeRange(params.StartTime, params.EndTime)
	if err != nil {
		return query.ComponentTraceQuery{}, err
	}
	return query.ComponentTraceQuery{
		ServiceName: params.ServiceName,
		TimeRange:   timeRange,
		Limit:       params.Limit,
	}, nil
}

func buildBaseLogQuery(params opensearch.QueryParams) (query.BaseLogQuery, error) {
	timeRange, err := buildTimeRange(params.StartTime, params.EndTime)
	if err != nil {
		return query.BaseLogQuery{}, err
	}

	order := query.SortDesc
	if strings.EqualFold(params.SortOrder, string(query.SortAsc)) {
		order = query.SortAsc
	}

	return query.BaseLogQuery{
		TimeRange:    timeRange,
		SearchPhrase: params.SearchPhrase,
		LogLevels:    params.LogLevels,
		Limit:        params.Limit,
		SortOrder:    order,
		Namespace:    params.Namespace,
		Versions:     params.Versions,
		VersionIDs:   params.VersionIDs,
		LogType:      params.LogType,
	}, nil
}

func buildTimeRange(start, end string) (query.TimeRange, error) {
	startTime, err := parseTimestamp(start)
	if err != nil {
		return query.TimeRange{}, fmt.Errorf("invalid startTime: %w", err)
	}
	endTime, err := parseTimestamp(end)
	if err != nil {
		return query.TimeRange{}, fmt.Errorf("invalid endTime: %w", err)
	}
	return query.TimeRange{
		Start: startTime,
		End:   endTime,
	}, nil
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05.000Z07:00",
}

func parseTimestamp(value string) (time.Time, error) {
	for _, layout := range timeLayouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse timestamp %q", value)
}
