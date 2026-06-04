// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strconv"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

const (
	httpEndpointKey                       = "http-endpoint"
	grpcEndpointKey                       = "grpc-endpoint"
	caCertificateKey                      = "ca-certificate"
	openTelemetryStackTracesKey           = "open-telemetry-stack-traces"
	openTelemetrySampleRatioKey           = "open-telemetry-sample-ratio"
	openTelemetryTailSamplingThresholdKey = "open-telemetry-tail-sampling-threshold"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// SetCharmTracingConfig sets the charm tracing config in the state.
	SetCharmTracingConfig(ctx context.Context, insertions map[string]string, deletions []string) error

	// GetCharmTracingConfig returns the charm tracing config from the state.
	GetCharmTracingConfig(ctx context.Context) (map[string]string, error)

	// SetWorkloadTracingConfig sets the workload tracing config in the state.
	SetWorkloadTracingConfig(ctx context.Context, insertions map[string]string, deletions []string) error

	// GetWorkloadTracingConfig returns the workload tracing config from the state.
	GetWorkloadTracingConfig(ctx context.Context) (map[string]string, error)
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
	GRPCEndpoint  string
	CACertificate string
}

// WorkloadTracingConfig defines the tracing configuration for workload
// telemetry.
type WorkloadTracingConfig struct {
	HTTPEndpoint  string
	GRPCEndpoint  string
	CACertificate string

	OpenTelemetryStackTraces           *bool
	OpenTelemetrySampleRatio           *float64
	OpenTelemetryTailSamplingThreshold *string
}

// SetCharmTracingConfig sets the charm tracing config. This method will
// insert any non-empty fields and delete any empty fields from the state.
func (s *Service) SetCharmTracingConfig(ctx context.Context, config CharmTracingConfig) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	insertions, deletions := splitTracingConfig(config.HTTPEndpoint, config.GRPCEndpoint, config.CACertificate)

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
		GRPCEndpoint:  configMap[grpcEndpointKey],
		CACertificate: configMap[caCertificateKey],
	}, nil
}

// SetWorkloadTracingConfig sets the workload tracing config. This method will
// insert any non-empty fields and delete any empty fields from the state.
func (s *Service) SetWorkloadTracingConfig(ctx context.Context, config WorkloadTracingConfig) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	insertions, deletions := splitTracingConfig(config.HTTPEndpoint, config.GRPCEndpoint, config.CACertificate)
	if config.OpenTelemetryStackTraces != nil {
		insertions[openTelemetryStackTracesKey] = strconv.FormatBool(*config.OpenTelemetryStackTraces)
	} else {
		deletions = append(deletions, openTelemetryStackTracesKey)
	}
	if config.OpenTelemetrySampleRatio != nil {
		insertions[openTelemetrySampleRatioKey] = strconv.FormatFloat(*config.OpenTelemetrySampleRatio, 'g', -1, 64)
	} else {
		deletions = append(deletions, openTelemetrySampleRatioKey)
	}
	if config.OpenTelemetryTailSamplingThreshold != nil &&
		*config.OpenTelemetryTailSamplingThreshold != "" {
		insertions[openTelemetryTailSamplingThresholdKey] = *config.OpenTelemetryTailSamplingThreshold
	} else {
		deletions = append(deletions, openTelemetryTailSamplingThresholdKey)
	}

	return s.st.SetWorkloadTracingConfig(ctx, insertions, deletions)
}

// GetWorkloadTracingConfig returns the workload tracing config from the state.
func (s *Service) GetWorkloadTracingConfig(ctx context.Context) (WorkloadTracingConfig, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	configMap, err := s.st.GetWorkloadTracingConfig(ctx)
	if err != nil {
		return WorkloadTracingConfig{}, err
	}

	var (
		openTelemetryStackTraces           *bool
		openTelemetrySampleRatio           *float64
		openTelemetryTailSamplingThreshold *string
	)

	if value, ok := configMap[openTelemetryStackTracesKey]; ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return WorkloadTracingConfig{}, errors.Errorf("parsing %q: %w", openTelemetryStackTracesKey, err)
		}
		openTelemetryStackTraces = &parsed
	}
	if value, ok := configMap[openTelemetrySampleRatioKey]; ok {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return WorkloadTracingConfig{}, errors.Errorf("parsing %q: %w", openTelemetrySampleRatioKey, err)
		}
		openTelemetrySampleRatio = &parsed
	}
	if value, ok := configMap[openTelemetryTailSamplingThresholdKey]; ok {
		openTelemetryTailSamplingThreshold = &value
	}

	return WorkloadTracingConfig{
		HTTPEndpoint:                       configMap[httpEndpointKey],
		GRPCEndpoint:                       configMap[grpcEndpointKey],
		CACertificate:                      configMap[caCertificateKey],
		OpenTelemetryStackTraces:           openTelemetryStackTraces,
		OpenTelemetrySampleRatio:           openTelemetrySampleRatio,
		OpenTelemetryTailSamplingThreshold: openTelemetryTailSamplingThreshold,
	}, nil
}

func splitTracingConfig(httpEndpoint, grpcEndpoint, caCertificate string) (map[string]string, []string) {
	insertions := make(map[string]string)
	deletions := make([]string, 0, 4)

	if httpEndpoint != "" {
		insertions[httpEndpointKey] = httpEndpoint
	} else {
		deletions = append(deletions, httpEndpointKey)
	}
	if grpcEndpoint != "" {
		insertions[grpcEndpointKey] = grpcEndpoint
	} else {
		deletions = append(deletions, grpcEndpointKey)
	}
	if caCertificate != "" {
		insertions[caCertificateKey] = caCertificate
	} else {
		deletions = append(deletions, caCertificateKey)
	}

	return insertions, deletions
}
