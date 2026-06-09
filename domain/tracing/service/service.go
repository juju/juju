// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strconv"
	"time"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
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

	// NamespaceForWatchWorkloadTracingConfig returns the namespace identifier
	// used for watching workload tracing configuration changes.
	NamespaceForWatchWorkloadTracingConfig() string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. Change-log events will be emitted
	// only if the filter accepts them, and dispatching the notifications
	// via the Changes channel. A filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
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

// WatchableService defines a service for interacting with the underlying
// state and the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new Service for interacting with the
// underlying state and the ability to create watchers.
func NewWatchableService(st State, wf WatcherFactory) *WatchableService {
	return &WatchableService{
		Service: Service{
			st: st,
		},
		watcherFactory: wf,
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

	if err := validateWorkloadTracingConfig(config); err != nil {
		return err
	}

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

// WatchWorkloadTracingConfig returns a watcher that emits notifications
// when the workload tracing configuration changes.
func (s *WatchableService) WatchWorkloadTracingConfig(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	namespace := s.st.NamespaceForWatchWorkloadTracingConfig()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"workload tracing config watcher",
		eventsource.NamespaceFilter(namespace, changestream.All),
	)
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

func validateWorkloadTracingConfig(config WorkloadTracingConfig) error {
	if err := validateOpenTelemetrySampleRatio(config.OpenTelemetrySampleRatio); err != nil {
		return err
	}
	if err := validateOpenTelemetryTailSamplingThreshold(config.OpenTelemetryTailSamplingThreshold); err != nil {
		return err
	}
	return nil
}

func validateOpenTelemetrySampleRatio(sampleRatio *float64) error {
	if sampleRatio == nil {
		return nil
	}
	if *sampleRatio < 0 || *sampleRatio > 1 {
		return errors.Errorf("%s value %f must be a ratio between 0 and 1",
			openTelemetrySampleRatioKey, *sampleRatio).Add(coreerrors.NotValid)
	}
	return nil
}

func validateOpenTelemetryTailSamplingThreshold(tailSamplingThreshold *string) error {
	if tailSamplingThreshold == nil || *tailSamplingThreshold == "" {
		return nil
	}

	v, err := time.ParseDuration(*tailSamplingThreshold)
	if err != nil {
		return errors.Errorf("%s value %q must be a valid duration",
			openTelemetryTailSamplingThresholdKey, *tailSamplingThreshold).Add(coreerrors.NotValid)
	}
	if v < 0 {
		return errors.Errorf("%s value %q must be a positive duration",
			openTelemetryTailSamplingThresholdKey, v).Add(coreerrors.NotValid)
	}
	return nil
}
