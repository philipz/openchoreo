// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"context"
	"time"
)

// SortOrder represents sort direction for log queries
type SortOrder string

const (
	// SortAsc orders by ascending timestamp
	SortAsc SortOrder = "asc"
	// SortDesc orders by descending timestamp
	SortDesc SortOrder = "desc"
)

// TimeRange defines the time window for queries
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// BaseLogQuery captures the shared filters for log queries
type BaseLogQuery struct {
	TimeRange    TimeRange
	SearchPhrase string
	LogLevels    []string
	Limit        int
	SortOrder    SortOrder
	Namespace    string
	Versions     []string
	VersionIDs   []string
	LogType      string
}

// ComponentLogQuery filters component level logs
type ComponentLogQuery struct {
	BaseLogQuery
	ComponentID   string
	EnvironmentID string
	BuildID       string
	BuildUUID     string
}

// ProjectLogQuery filters project level logs
type ProjectLogQuery struct {
	BaseLogQuery
	ProjectID     string
	ComponentIDs  []string
	EnvironmentID string
}

// OrganizationLogQuery filters organization wide logs
type OrganizationLogQuery struct {
	BaseLogQuery
	OrganizationID string
	EnvironmentID  string
	PodLabels      map[string]string
}

// GatewayLogQuery filters gateway traffic logs
type GatewayLogQuery struct {
	BaseLogQuery
	OrganizationID    string
	APIIDToVersionMap map[string]string
	GatewayVHosts     []string
}

// ComponentTraceQuery filters spans for a component/service
type ComponentTraceQuery struct {
	ServiceName string
	TimeRange   TimeRange
	Limit       int
}

// LogRecord represents a row returned by storage providers
type LogRecord struct {
	Timestamp     time.Time         `json:"timestamp"`
	Log           string            `json:"log"`
	LogLevel      string            `json:"logLevel"`
	ComponentID   string            `json:"componentId"`
	EnvironmentID string            `json:"environmentId"`
	ProjectID     string            `json:"projectId"`
	Version       string            `json:"version"`
	VersionID     string            `json:"versionId"`
	Namespace     string            `json:"namespace"`
	PodID         string            `json:"podId"`
	ContainerName string            `json:"containerName"`
	Labels        map[string]string `json:"labels"`
}

// TraceRecord represents a span row
type TraceRecord struct {
	DurationInNanos int64     `json:"durationInNanos"`
	EndTime         time.Time `json:"endTime"`
	Name            string    `json:"name"`
	SpanID          string    `json:"spanId"`
	StartTime       time.Time `json:"startTime"`
	TraceID         string    `json:"traceId"`
}

// LogResult wraps log rows along with metadata
type LogResult struct {
	Logs       []LogRecord `json:"logs"`
	TotalCount int         `json:"totalCount"`
	Took       int         `json:"tookMs"`
}

// TraceResult wraps span rows
type TraceResult struct {
	Spans      []TraceRecord `json:"spans"`
	TotalCount int           `json:"totalCount"`
	Took       int           `json:"tookMs"`
}

// CostReportQuery defines the interval for cost aggregation
type CostReportQuery struct {
	Start time.Time
	End   time.Time
}

// CostReportRow captures per-tenant/project usage
type CostReportRow struct {
	OrganizationID    string  `json:"organizationId"`
	ProjectID         string  `json:"projectId"`
	ComponentID       string  `json:"componentId"`
	LogCount          int     `json:"logCount"`
	EstimatedStorageB float64 `json:"estimatedStorageBytes"`
	EstimatedCostUSD  float64 `json:"estimatedCostUsd"`
}

// CostReport aggregates rows for a time window
type CostReport struct {
	Start time.Time       `json:"start"`
	End   time.Time       `json:"end"`
	Rows  []CostReportRow `json:"rows"`
	Total float64         `json:"totalCostUsd"`
}

// StorageProvider exposes telemetry query operations
type StorageProvider interface {
	GetComponentLogs(ctx context.Context, params ComponentLogQuery) (*LogResult, error)
	GetProjectLogs(ctx context.Context, params ProjectLogQuery) (*LogResult, error)
	GetGatewayLogs(ctx context.Context, params GatewayLogQuery) (*LogResult, error)
	GetOrganizationLogs(ctx context.Context, params OrganizationLogQuery) (*LogResult, error)
	GetComponentTraces(ctx context.Context, params ComponentTraceQuery) (*TraceResult, error)
	GetCostReport(ctx context.Context, params CostReportQuery) (*CostReport, error)
	HealthCheck(ctx context.Context) error
}
