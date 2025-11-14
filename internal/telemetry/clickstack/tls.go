// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clickstack

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/openchoreo/openchoreo/internal/observer/config"
)

func buildTLSConfig(cfg config.ClickStackConfig) (*tls.Config, error) {
	if !cfg.Secure {
		return nil, nil
	}

	tlsCfg := &tls.Config{}

	if cfg.CACertPath != "" {
		data, err := os.ReadFile(cfg.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(data) {
			return nil, fmt.Errorf("invalid CA certificate")
		}
		tlsCfg.RootCAs = pool
	}

	if cfg.ClientCertPath != "" && cfg.ClientKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCertPath, cfg.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client key pair: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}
