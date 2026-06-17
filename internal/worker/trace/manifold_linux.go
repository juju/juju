//go:build dqlite && linux

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	"github.com/juju/juju/internal/services"
)

// GetTracingServiceFunc returns the controller tracing service from the
// dependency getter.
type GetTracingServiceFunc func(getter dependency.Getter, name string) (TracingService, error)

// ControllerManifoldConfig defines the configuration for the controller
// trace manifold.
type ControllerManifoldConfig struct {
	Tag               names.Tag
	TraceServicesName string
	Clock             clock.Clock
	Logger            logger.Logger
	GetTracingService GetTracingServiceFunc
	NewTracerWorker   TracerWorkerFunc
}

// Validate validates the controller manifold configuration.
func (cfg ControllerManifoldConfig) Validate() error {
	if cfg.Tag == nil {
		return errors.NotValidf("nil Tag")
	}
	if cfg.TraceServicesName == "" {
		return errors.NotValidf("empty TraceServicesName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.GetTracingService == nil {
		return errors.NotValidf("nil GetTracingService")
	}
	if cfg.NewTracerWorker == nil {
		return errors.NotValidf("nil NewTracerWorker")
	}
	return nil
}

// ControllerManifold returns a dependency manifold that runs the controller
// trace worker with workload tracing hot-reload.
func ControllerManifold(config ControllerManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.TraceServicesName,
		},
		Output: tracerOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			tracingService, err := config.GetTracingService(getter, config.TraceServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			config.Logger.Infof(ctx, "starting controller trace worker using workload tracing config")

			w, err := NewWorker(WorkerConfig{
				Clock:                 config.Clock,
				Logger:                config.Logger,
				NewTracerWorker:       config.NewTracerWorker,
				Tag:                   config.Tag,
				Kind:                  coretrace.KindController,
				SampleRatio:           defaultOpenTelemetrySampleRatio,
				TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
				RuntimeConfigProvider: controllerRuntimeConfigProvider{
					tracingService: tracingService,
				},
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w, nil
		},
	}
}

// TracingService is the interface that defines the methods required from the
// tracing service.
type TracingService interface {
	// GetWorkloadTracingConfig returns the workload tracing config from the state.
	GetWorkloadTracingConfig(ctx context.Context) (tracingservice.WorkloadTracingConfig, error)

	// WatchWorkloadTracingConfig returns a watcher that emits notifications
	// when the workload tracing configuration changes.
	WatchWorkloadTracingConfig(ctx context.Context) (watcher.NotifyWatcher, error)
}

type controllerRuntimeConfigProvider struct {
	tracingService TracingService
}

// CurrentRuntimeConfig returns the current runtime config for the controller
// trace worker, which is derived from the workload tracing config in the state.
func (p controllerRuntimeConfigProvider) CurrentRuntimeConfig(ctx context.Context) (RuntimeConfig, error) {
	cfg, err := p.tracingService.GetWorkloadTracingConfig(ctx)
	if err != nil {
		return RuntimeConfig{}, errors.Trace(err)
	}
	return runtimeConfigFromWorkloadTracingConfig(cfg)
}

// WatchRuntimeConfig returns a watcher that emits notifications when the
// workload tracing configuration changes.
func (p controllerRuntimeConfigProvider) WatchRuntimeConfig(ctx context.Context) (watcher.NotifyWatcher, error) {
	return p.tracingService.WatchWorkloadTracingConfig(ctx)
}

func runtimeConfigFromWorkloadTracingConfig(cfg tracingservice.WorkloadTracingConfig) (RuntimeConfig, error) {
	runtimeCfg := RuntimeConfig{
		SampleRatio:           defaultOpenTelemetrySampleRatio,
		TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
		CACertificate:         cfg.CACertificate,
	}

	endpoint := cfg.GRPCEndpoint
	if endpoint == "" {
		endpoint = cfg.HTTPEndpoint
	}
	if endpoint != "" {
		runtimeCfg.Enabled = true
		runtimeCfg.Endpoint = endpoint
	}

	if cfg.OpenTelemetryStackTraces != nil {
		runtimeCfg.StackTracesEnabled = *cfg.OpenTelemetryStackTraces
	}

	if cfg.OpenTelemetrySampleRatio != nil {
		if *cfg.OpenTelemetrySampleRatio < 0 || *cfg.OpenTelemetrySampleRatio > 1 {
			return RuntimeConfig{}, errors.NotValidf("open telemetry sample ratio %.4f", *cfg.OpenTelemetrySampleRatio)
		}
		runtimeCfg.SampleRatio = *cfg.OpenTelemetrySampleRatio
	}

	if cfg.OpenTelemetryTailSamplingThreshold != nil {
		d, err := time.ParseDuration(*cfg.OpenTelemetryTailSamplingThreshold)
		if err != nil {
			return RuntimeConfig{}, errors.Annotatef(err, "parsing open telemetry tail sampling threshold %q", *cfg.OpenTelemetryTailSamplingThreshold)
		}
		if d < 0 {
			return RuntimeConfig{}, errors.NotValidf("open telemetry tail sampling threshold %q", *cfg.OpenTelemetryTailSamplingThreshold)
		}
		runtimeCfg.TailSamplingThreshold = d
	}

	return runtimeCfg, nil
}

// GetTracingService returns the controller tracing service from the
// dependency getter.
func GetTracingService(getter dependency.Getter, name string) (TracingService, error) {
	var traceServices services.TraceServices
	if err := getter.Get(name, &traceServices); err != nil {
		return nil, err
	}
	return traceServices.Tracing(), nil
}
