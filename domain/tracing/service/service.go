// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
)

const (
	httpEndpointKey  = "http-endpoint"
	httpsEndpointKey = "https-endpoint"
	grpcEndpointKey  = "grpc-endpoint"
	caCertificateKey = "ca-certificate"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// SetCharmTracingConfig sets the charm tracing config in the state.
	SetCharmTracingConfig(ctx context.Context, insertions map[string]string, deletions []string) error

	// GetCharmTracingConfig returns the charm tracing config from the state.
	GetCharmTracingConfig(ctx context.Context) (map[string]string, error)
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st State
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// CharmTracingConfig defines the tracing configuration for an OTEL collector.
type CharmTracingConfig struct {
	HTTPEndpoint  string
	HTTPSEndpoint string
	GRPCEndpoint  string
	CACertificate string
}

// SetCharmTracingConfig sets the charm tracing config. This method will
// insert any non-empty fields and delete any empty fields from the state.
func (s *Service) SetCharmTracingConfig(ctx context.Context, config CharmTracingConfig) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	insertions := make(map[string]string)
	deletions := make([]string, 0, 4)
	if config.HTTPEndpoint != "" {
		insertions[httpEndpointKey] = config.HTTPEndpoint
	} else {
		deletions = append(deletions, httpEndpointKey)
	}
	if config.HTTPSEndpoint != "" {
		insertions[httpsEndpointKey] = config.HTTPSEndpoint
	} else {
		deletions = append(deletions, httpsEndpointKey)
	}
	if config.GRPCEndpoint != "" {
		insertions[grpcEndpointKey] = config.GRPCEndpoint
	} else {
		deletions = append(deletions, grpcEndpointKey)
	}
	if config.CACertificate != "" {
		insertions[caCertificateKey] = config.CACertificate
	} else {
		deletions = append(deletions, caCertificateKey)
	}

	return s.st.SetCharmTracingConfig(ctx, insertions, deletions)
}

// GetCharmTracingConfig returns the charm tracing config from the state.
func (s *Service) GetCharmTracingConfig(ctx context.Context) (CharmTracingConfig, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	configMap, err := s.st.GetCharmTracingConfig(ctx)
	if err != nil {
		return CharmTracingConfig{}, err
	}

	return CharmTracingConfig{
		HTTPEndpoint:  configMap[httpEndpointKey],
		HTTPSEndpoint: configMap[httpsEndpointKey],
		GRPCEndpoint:  configMap[grpcEndpointKey],
		CACertificate: configMap[caCertificateKey],
	}, nil
}
