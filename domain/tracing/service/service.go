// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// SetTracingConfig sets the tracing config in the state.
	SetTracingConfig(ctx context.Context, insert map[string]string, delete []string) error

	// GetTracingConfig returns the tracing config from the state.
	GetTracingConfig(ctx context.Context) (map[string]string, error)
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

// TracingConfig defines the tracing configuration for the model.
type TracingConfig struct {
	HTTPEndpoint  string
	HTTPSEndpoint string
	GRPCEndpoint  string
	CACertificate string
}

// SetTracingConfig sets the tracing config in the state.
func (s *Service) SetTracingConfig(ctx context.Context, config TracingConfig) error {
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

	return s.st.SetTracingConfig(ctx, insertions, deletions)
}

// GetTracingConfig returns the tracing config from the state.
func (s *Service) GetTracingConfig(ctx context.Context) (TracingConfig, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	configMap, err := s.st.GetTracingConfig(ctx)
	if err != nil {
		return TracingConfig{}, err
	}

	return TracingConfig{
		HTTPEndpoint:  configMap[httpEndpointKey],
		HTTPSEndpoint: configMap[httpsEndpointKey],
		GRPCEndpoint:  configMap[grpcEndpointKey],
		CACertificate: configMap[caCertificateKey],
	}, nil
}

const (
	httpEndpointKey  = "http-endpoint"
	httpsEndpointKey = "https-endpoint"
	grpcEndpointKey  = "grpc-endpoint"
	caCertificateKey = "ca-certificate"
)
