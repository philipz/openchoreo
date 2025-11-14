// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
)

// Config holds all configuration for the logging service
type Config struct {
	Server     ServerConfig     `koanf:"server"`
	OpenSearch OpenSearchConfig `koanf:"opensearch"`
	ClickStack ClickStackConfig `koanf:"clickstack"`
	Telemetry  TelemetryConfig  `koanf:"telemetry"`
	Auth       AuthConfig       `koanf:"auth"`
	Logging    LoggingConfig    `koanf:"logging"`
	LogLevel   string           `koanf:"loglevel"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port            int           `koanf:"port"`
	ReadTimeout     time.Duration `koanf:"read.timeout"`
	WriteTimeout    time.Duration `koanf:"write.timeout"`
	ShutdownTimeout time.Duration `koanf:"shutdown.timeout"`
}

// OpenSearchConfig holds OpenSearch connection configuration
type OpenSearchConfig struct {
	Address       string        `koanf:"address"`
	Username      string        `koanf:"username"`
	Password      string        `koanf:"password"`
	Timeout       time.Duration `koanf:"timeout"`
	MaxRetries    int           `koanf:"max.retries"`
	IndexPrefix   string        `koanf:"index.prefix"`
	IndexPattern  string        `koanf:"index.pattern"`
	LegacyPattern string        `koanf:"legacy.pattern"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret    string `koanf:"jwt.secret"`
	EnableAuth   bool   `koanf:"enable.auth"`
	RequiredRole string `koanf:"required.role"`
}

// LoggingConfig holds application logging configuration
type LoggingConfig struct {
	MaxLogLimit          int `koanf:"max.log.limit"`
	DefaultLogLimit      int `koanf:"default.log.limit"`
	DefaultBuildLogLimit int `koanf:"default.build.log.limit"`
	MaxLogLinesPerFile   int `koanf:"max.log.lines.per.file"`
}

// TelemetryConfig controls backend selection and integrations
type TelemetryConfig struct {
	Backend        string        `koanf:"backend"`
	DualRead       bool          `koanf:"dual.read"`
	DualSampleRate float64       `koanf:"dual.sample.rate"`
	HyperDX        HyperDXConfig `koanf:"hyperdx"`
}

// HyperDXConfig stores URL signing settings
type HyperDXConfig struct {
	BaseURL    string        `koanf:"base.url"`
	SigningKey string        `koanf:"signing.key"`
	TTL        time.Duration `koanf:"ttl"`
}

// ClickStackConfig holds ClickHouse/HyperDX connection configuration
type ClickStackConfig struct {
	Hosts             []string      `koanf:"hosts"`
	Database          string        `koanf:"database"`
	Username          string        `koanf:"username"`
	Password          string        `koanf:"password"`
	Secure            bool          `koanf:"secure"`
	CACertPath        string        `koanf:"ca.cert"`
	ClientCertPath    string        `koanf:"client.cert"`
	ClientKeyPath     string        `koanf:"client.key"`
	Timeout           time.Duration `koanf:"timeout"`
	QueryTimeout      time.Duration `koanf:"query.timeout"`
	ReadTimeout       time.Duration `koanf:"read.timeout"`
	WriteTimeout      time.Duration `koanf:"write.timeout"`
	RetryAttempts     int           `koanf:"retry.attempts"`
	LogsTable         string        `koanf:"logs.table"`
	TracesTable       string        `koanf:"traces.table"`
	MaxOpenConns      int           `koanf:"max.open.conns"`
	MaxIdleConns      int           `koanf:"max.idle.conns"`
	ConnMaxLifetime   time.Duration `koanf:"conn.max.lifetime"`
	CompressionMethod string        `koanf:"compression.method"`
}

// Load loads configuration from environment variables and defaults
func Load() (*Config, error) {
	k := koanf.New(".")

	// Load defaults first
	if err := k.Load(confmap.Provider(getDefaults(), "."), nil); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	// Load environment variables for specific keys we care about
	envOverrides := make(map[string]interface{})

	// Define environment variable mappings
	envMappings := map[string]string{
		"SERVER_PORT":                     "server.port",
		"SERVER_READ_TIMEOUT":             "server.read.timeout",
		"SERVER_WRITE_TIMEOUT":            "server.write.timeout",
		"SERVER_SHUTDOWN_TIMEOUT":         "server.shutdown.timeout",
		"OPENSEARCH_ADDRESS":              "opensearch.address",
		"OPENSEARCH_USERNAME":             "opensearch.username",
		"OPENSEARCH_PASSWORD":             "opensearch.password",
		"OPENSEARCH_TIMEOUT":              "opensearch.timeout",
		"OPENSEARCH_MAX_RETRIES":          "opensearch.max.retries",
		"OPENSEARCH_INDEX_PREFIX":         "opensearch.index.prefix",
		"OPENSEARCH_INDEX_PATTERN":        "opensearch.index.pattern",
		"OPENSEARCH_LEGACY_PATTERN":       "opensearch.legacy.pattern",
		"AUTH_JWT_SECRET":                 "auth.jwt.secret",
		"AUTH_ENABLE_AUTH":                "auth.enable.auth",
		"AUTH_REQUIRED_ROLE":              "auth.required.role",
		"LOGGING_MAX_LOG_LIMIT":           "logging.max.log.limit",
		"LOGGING_DEFAULT_LOG_LIMIT":       "logging.default.log.limit",
		"LOGGING_DEFAULT_BUILD_LOG_LIMIT": "logging.default.build.log.limit",
		"LOGGING_MAX_LOG_LINES_PER_FILE":  "logging.max.log.lines.per.file",
		"LOG_LEVEL":                       "loglevel",
		"PORT":                            "server.port",           // Common alias
		"JWT_SECRET":                      "auth.jwt.secret",       // Common alias
		"ENABLE_AUTH":                     "auth.enable.auth",      // Common alias
		"MAX_LOG_LIMIT":                   "logging.max.log.limit", // Common alias
		"CLICKSTACK_HOSTS":                "clickstack.hosts",
		"CLICKSTACK_DATABASE":             "clickstack.database",
		"CLICKSTACK_USERNAME":             "clickstack.username",
		"CLICKSTACK_PASSWORD":             "clickstack.password",
		"CLICKSTACK_SECURE":               "clickstack.secure",
		"CLICKSTACK_CA_CERT":              "clickstack.ca.cert",
		"CLICKSTACK_CLIENT_CERT":          "clickstack.client.cert",
		"CLICKSTACK_CLIENT_KEY":           "clickstack.client.key",
		"CLICKSTACK_TIMEOUT":              "clickstack.timeout",
		"CLICKSTACK_QUERY_TIMEOUT":        "clickstack.query.timeout",
		"CLICKSTACK_READ_TIMEOUT":         "clickstack.read.timeout",
		"CLICKSTACK_WRITE_TIMEOUT":        "clickstack.write.timeout",
		"CLICKSTACK_RETRY_ATTEMPTS":       "clickstack.retry.attempts",
		"CLICKSTACK_LOGS_TABLE":           "clickstack.logs.table",
		"CLICKSTACK_TRACES_TABLE":         "clickstack.traces.table",
		"CLICKSTACK_MAX_OPEN_CONNS":       "clickstack.max.open.conns",
		"CLICKSTACK_MAX_IDLE_CONNS":       "clickstack.max.idle.conns",
		"CLICKSTACK_CONN_MAX_LIFETIME":    "clickstack.conn.max.lifetime",
		"CLICKSTACK_COMPRESSION_METHOD":   "clickstack.compression.method",
		"TELEMETRY_BACKEND":               "telemetry.backend",
		"TELEMETRY_DUAL_READ":             "telemetry.dual.read",
		"TELEMETRY_DUAL_SAMPLE_RATE":      "telemetry.dual.sample.rate",
		"HYPERDX_BASE_URL":                "telemetry.hyperdx.base.url",
		"HYPERDX_SIGNING_KEY":             "telemetry.hyperdx.signing.key",
		"HYPERDX_TTL":                     "telemetry.hyperdx.ttl",
	}

	// Check for environment variables and map them to nested structure
	for envKey, configKey := range envMappings {
		if value := os.Getenv(envKey); value != "" {
			var processed interface{} = value
			if configKey == "clickstack.hosts" {
				processed = splitAndTrim(value)
			}
			// Split the config key and create nested structure
			parts := strings.Split(configKey, ".")
			if len(parts) == 1 {
				// Top-level key
				envOverrides[configKey] = processed
			} else if len(parts) == 2 {
				// Nested key like "server.port"
				section := parts[0]
				key := parts[1]
				if envOverrides[section] == nil {
					envOverrides[section] = make(map[string]interface{})
				}
				envOverrides[section].(map[string]interface{})[key] = processed
			} else if len(parts) >= 3 {
				// Handle multi-part keys like "logging.max.log.limit"
				section := parts[0]
				key := strings.Join(parts[1:], ".")
				if envOverrides[section] == nil {
					envOverrides[section] = make(map[string]interface{})
				}
				envOverrides[section].(map[string]interface{})[key] = processed
			}
		}
	}

	// Load environment overrides
	if len(envOverrides) > 0 {
		if err := k.Load(confmap.Provider(envOverrides, "."), nil); err != nil {
			return nil, fmt.Errorf("failed to load environment overrides: %w", err)
		}
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// getDefaults returns the default configuration values
func getDefaults() map[string]interface{} {
	return map[string]interface{}{
		"server": map[string]interface{}{
			"port":             9097,
			"read.timeout":     "30s",
			"write.timeout":    "30s",
			"shutdown.timeout": "10s",
		},
		"opensearch": map[string]interface{}{
			"address":        "http://localhost:9200",
			"username":       "admin",
			"password":       "admin",
			"timeout":        "180s",
			"max.retries":    3,
			"index.prefix":   "kubernetes-",
			"index.pattern":  "kubernetes-*",
			"legacy.pattern": "choreo*",
		},
		"auth": map[string]interface{}{
			"enable.auth":   false,
			"jwt.secret":    "default-secret",
			"required.role": "user",
		},
		"logging": map[string]interface{}{
			"max.log.limit":           10000,
			"default.log.limit":       100,
			"default.build.log.limit": 3000,
			"max.log.lines.per.file":  600000,
		},
		"clickstack": map[string]interface{}{
			"hosts":              []string{"localhost:9000"},
			"database":           "telemetry",
			"username":           "default",
			"password":           "",
			"secure":             false,
			"timeout":            "30s",
			"query.timeout":      "10s",
			"read.timeout":       "30s",
			"write.timeout":      "30s",
			"retry.attempts":     3,
			"logs.table":         "telemetry.logs_mv",
			"traces.table":       "telemetry.traces_mv",
			"max.open.conns":     10,
			"max.idle.conns":     5,
			"conn.max.lifetime":  "5m",
			"compression.method": "lz4",
		},
		"telemetry": map[string]interface{}{
			"backend":          "opensearch",
			"dual.read":        false,
			"dual.sample.rate": 0.05,
			"hyperdx": map[string]interface{}{
				"base.url":    "",
				"signing.key": "",
				"ttl":         "15m",
			},
		},
		"loglevel": "info",
	}
}

func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.OpenSearch.Address == "" {
		return fmt.Errorf("opensearch address is required")
	}

	if c.OpenSearch.Timeout <= 0 {
		return fmt.Errorf("opensearch timeout must be positive")
	}

	if c.Logging.MaxLogLimit <= 0 {
		return fmt.Errorf("max log limit must be positive")
	}

	if len(c.ClickStack.Hosts) == 0 {
		return fmt.Errorf("at least one clickstack host is required")
	}

	if c.ClickStack.LogsTable == "" || c.ClickStack.TracesTable == "" {
		return fmt.Errorf("clickstack logs and traces table names are required")
	}

	if c.Telemetry.DualSampleRate < 0 || c.Telemetry.DualSampleRate > 1 {
		return fmt.Errorf("telemetry dual sample rate must be between 0 and 1")
	}

	if c.Telemetry.HyperDX.SigningKey != "" && c.Telemetry.HyperDX.BaseURL == "" {
		return fmt.Errorf("hyperdx base url is required when signing is enabled")
	}

	if c.Telemetry.HyperDX.SigningKey != "" && c.Telemetry.HyperDX.TTL <= 0 {
		return fmt.Errorf("hyperdx ttl must be positive when signing is enabled")
	}

	return nil
}

func splitAndTrim(value string) []string {
	raw := strings.Split(value, ",")
	hosts := make([]string, 0, len(raw))
	for _, h := range raw {
		h = strings.TrimSpace(h)
		if h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}
