// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clickstack

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/pkg/telemetry/query"
)

// Provider implements query.StorageProvider backed by ClickStack (ClickHouse).
type Provider struct {
	db      *sql.DB
	cfg     config.ClickStackConfig
	builder *QueryBuilder
	logger  *slog.Logger
}

const (
	storageCostPerTBUSD         = 0.50
	logProcessingCostPerMillion = 2.0
)

// NewProvider creates a new ClickStack provider instance.
func NewProvider(cfg config.ClickStackConfig, logger *slog.Logger) (*Provider, error) {
	if len(cfg.Hosts) == 0 {
		return nil, errors.New("clickstack hosts are required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	opts, err := buildClickHouseOptions(cfg)
	if err != nil {
		return nil, err
	}

	db := clickhouse.OpenDB(opts)
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect to clickstack: %w", err)
	}

	return &Provider{
		db:      db,
		cfg:     cfg,
		builder: NewQueryBuilder(cfg),
		logger:  logger,
	}, nil
}

func buildClickHouseOptions(cfg config.ClickStackConfig) (*clickhouse.Options, error) {
	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	opts := &clickhouse.Options{
		Addr: cfg.Hosts,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout: cfg.Timeout,
		Settings: clickhouse.Settings{
			"max_execution_time": cfg.QueryTimeout.Seconds(),
		},
		Compression: &clickhouse.Compression{
			Method: compressionMethod(cfg.CompressionMethod),
		},
		TLS: tlsCfg,
	}

	return opts, nil
}

func compressionMethod(value string) clickhouse.CompressionMethod {
	switch strings.ToLower(value) {
	case "zstd":
		return clickhouse.CompressionZSTD
	case "none":
		return clickhouse.CompressionNone
	default:
		return clickhouse.CompressionLZ4
	}
}

// Close releases the underlying DB resources.
func (p *Provider) Close() error {
	if p.db == nil {
		return nil
	}
	return p.db.Close()
}

func (p *Provider) withQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := p.cfg.QueryTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

// GetComponentLogs queries ClickStack for component logs.
func (p *Provider) GetComponentLogs(ctx context.Context, params query.ComponentLogQuery) (*query.LogResult, error) {
	sqlStmt, args, err := p.builder.ComponentLogs(params)
	if err != nil {
		return nil, err
	}
	return p.executeLogQuery(ctx, sqlStmt, args...)
}

// GetProjectLogs queries ClickStack for project logs.
func (p *Provider) GetProjectLogs(ctx context.Context, params query.ProjectLogQuery) (*query.LogResult, error) {
	sqlStmt, args, err := p.builder.ProjectLogs(params)
	if err != nil {
		return nil, err
	}
	return p.executeLogQuery(ctx, sqlStmt, args...)
}

// GetGatewayLogs queries ClickStack for gateway logs.
func (p *Provider) GetGatewayLogs(ctx context.Context, params query.GatewayLogQuery) (*query.LogResult, error) {
	sqlStmt, args, err := p.builder.GatewayLogs(params)
	if err != nil {
		return nil, err
	}
	return p.executeLogQuery(ctx, sqlStmt, args...)
}

// GetOrganizationLogs queries ClickStack for org logs.
func (p *Provider) GetOrganizationLogs(ctx context.Context, params query.OrganizationLogQuery) (*query.LogResult, error) {
	sqlStmt, args, err := p.builder.OrganizationLogs(params)
	if err != nil {
		return nil, err
	}
	return p.executeLogQuery(ctx, sqlStmt, args...)
}

// GetComponentTraces queries ClickStack for component traces.
func (p *Provider) GetComponentTraces(ctx context.Context, params query.ComponentTraceQuery) (*query.TraceResult, error) {
	sqlStmt, args, err := p.builder.ComponentTraces(params)
	if err != nil {
		return nil, err
	}
	return p.executeTraceQuery(ctx, sqlStmt, args...)
}

// HealthCheck verifies connectivity with ClickHouse.
func (p *Provider) HealthCheck(ctx context.Context) error {
	ctx, cancel := p.withQueryTimeout(ctx)
	defer cancel()
	return p.db.PingContext(ctx)
}

func (p *Provider) executeLogQuery(ctx context.Context, sqlStmt string, args ...any) (*query.LogResult, error) {
	ctx, cancel := p.withQueryTimeout(ctx)
	defer cancel()

	start := time.Now()
	rows, err := p.db.QueryContext(ctx, sqlStmt, args...)
	if err != nil {
		return nil, fmt.Errorf("clickstack log query failed: %w", err)
	}
	defer rows.Close()

	result := &query.LogResult{
		Logs: make([]query.LogRecord, 0, 64),
	}

	for rows.Next() {
		var record query.LogRecord
		var labelsJSON sql.NullString
		var totalCount sql.NullInt64
		if err := rows.Scan(
			&record.Timestamp,
			&record.Log,
			&record.LogLevel,
			&record.ComponentID,
			&record.EnvironmentID,
			&record.ProjectID,
			&record.Version,
			&record.VersionID,
			&record.Namespace,
			&record.PodID,
			&record.ContainerName,
			&labelsJSON,
			&totalCount,
		); err != nil {
			return nil, fmt.Errorf("scan log row: %w", err)
		}

		if labelsJSON.Valid {
			if err := json.Unmarshal([]byte(labelsJSON.String), &record.Labels); err != nil {
				p.logger.Warn("Failed to decode labels JSON", "error", err)
				record.Labels = map[string]string{}
			}
		}

		if totalCount.Valid {
			result.TotalCount = int(totalCount.Int64)
		}

		result.Logs = append(result.Logs, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate log rows: %w", err)
	}

	result.Took = int(time.Since(start).Milliseconds())
	return result, nil
}

func (p *Provider) executeTraceQuery(ctx context.Context, sqlStmt string, args ...any) (*query.TraceResult, error) {
	ctx, cancel := p.withQueryTimeout(ctx)
	defer cancel()

	start := time.Now()
	rows, err := p.db.QueryContext(ctx, sqlStmt, args...)
	if err != nil {
		return nil, fmt.Errorf("clickstack trace query failed: %w", err)
	}
	defer rows.Close()

	result := &query.TraceResult{
		Spans: make([]query.TraceRecord, 0, 64),
	}
	for rows.Next() {
		var record query.TraceRecord
		var totalCount sql.NullInt64
		if err := rows.Scan(
			&record.StartTime,
			&record.EndTime,
			&record.Name,
			&record.SpanID,
			&record.TraceID,
			&record.DurationInNanos,
			&totalCount,
		); err != nil {
			return nil, fmt.Errorf("scan trace row: %w", err)
		}

		if totalCount.Valid {
			result.TotalCount = int(totalCount.Int64)
		}

		result.Spans = append(result.Spans, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace rows: %w", err)
	}

	result.Took = int(time.Since(start).Milliseconds())
	return result, nil
}

// GetCostReport aggregates usage data for a billing window.
func (p *Provider) GetCostReport(ctx context.Context, params query.CostReportQuery) (*query.CostReport, error) {
	sqlStmt, args, err := p.builder.CostReport(params)
	if err != nil {
		return nil, err
	}

	ctx, cancel := p.withQueryTimeout(ctx)
	defer cancel()

	rows, err := p.db.QueryContext(ctx, sqlStmt, args...)
	if err != nil {
		return nil, fmt.Errorf("clickstack cost report query failed: %w", err)
	}
	defer rows.Close()

	report := &query.CostReport{
		Start: params.Start,
		End:   params.End,
		Rows:  make([]query.CostReportRow, 0),
	}

	for rows.Next() {
		var (
			org       sql.NullString
			project   sql.NullString
			component sql.NullString
			logCount  int64
			rawBytes  float64
		)
		if err := rows.Scan(&org, &project, &component, &logCount, &rawBytes); err != nil {
			return nil, fmt.Errorf("scan cost report row: %w", err)
		}

		storageTB := rawBytes / (1024 * 1024 * 1024 * 1024)
		storageCost := storageTB * storageCostPerTBUSD
		processingCost := float64(logCount) / 1_000_000 * logProcessingCostPerMillion
		total := storageCost + processingCost

		report.Rows = append(report.Rows, query.CostReportRow{
			OrganizationID:    nullString(org, "unknown"),
			ProjectID:         nullString(project, "unknown"),
			ComponentID:       nullString(component, "unknown"),
			LogCount:          int(logCount),
			EstimatedStorageB: rawBytes,
			EstimatedCostUSD:  total,
		})
		report.Total += total
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cost rows: %w", err)
	}

	return report, nil
}

func nullString(v sql.NullString, fallback string) string {
	if v.Valid && v.String != "" {
		return v.String
	}
	return fallback
}
