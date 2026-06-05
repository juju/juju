// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	"github.com/juju/juju/internal/services"
)

// TracerGetter is the interface that is used to get a tracer.
type TracerGetter interface {
	// GetTracer returns a tracer for the given namespace.
	GetTracer(context.Context, coretrace.TracerNamespace) (coretrace.Tracer, error)
}

// TracerWorkerFunc is the function signature for creating a new tracer worker.
type TracerWorkerFunc func(
	ctx context.Context,
	namespace coretrace.TaggedTracerNamespace,
	endpoint string,
	insecureSkipVerify bool,
	showStackTraces bool,
	sampleRatio float64,
	tailSamplingThreshold time.Duration,
	logger logger.Logger,
	newClient NewClientFunc,
) (TrackedTracer, error)

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	AgentName       string
	Clock           clock.Clock
	Logger          logger.Logger
	NewTracerWorker TracerWorkerFunc
	Kind            coretrace.Kind
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewTracerWorker == nil {
		return errors.NotValidf("nil NewTracerWorker")
	}
	if cfg.Kind == "" {
		return errors.NotValidf("empty Kind")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
		},
		Output: tracerOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			currentConfig := a.CurrentConfig()

			enabled := currentConfig.OpenTelemetryEnabled()
			endpoint := currentConfig.OpenTelemetryEndpoint()
			if enabled {
				config.Logger.Infof(ctx, "OpenTelemetry enabled, starting trace worker using endpoint %q", endpoint)
			} else {
				config.Logger.Infof(ctx, "OpenTelemetry disabled, starting trace worker in disabled mode")
			}

			w, err := NewWorker(WorkerConfig{
				Clock:                 config.Clock,
				Logger:                config.Logger,
				NewTracerWorker:       config.NewTracerWorker,
				Tag:                   currentConfig.Tag(),
				Kind:                  config.Kind,
				SampleRatio:           defaultOpenTelemetrySampleRatio,
				TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
				RuntimeConfigProvider: unitRuntimeConfigProvider{
					config: currentConfig,
				},
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w, nil
		},
	}
}

// GetTracingServiceFunc returns the controller tracing service from the
// dependency getter.
type GetTracingServiceFunc func(getter dependency.Getter, name string) (TracingService, error)

// ControllerManifoldConfig defines the configuration for the controller
// trace manifold.
type ControllerManifoldConfig struct {
	AgentName          string
	DomainServicesName string
	ChangeStreamName   string
	Clock              clock.Clock
	Logger             logger.Logger
	GetTracingService  GetTracingServiceFunc
	NewTracerWorker    TracerWorkerFunc
}

// Validate validates the controller manifold configuration.
func (cfg ControllerManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
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
			config.AgentName,
			config.ChangeStreamName,
		},
		Output: tracerOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}
			currentConfig := a.CurrentConfig()

			var dbGetter changestream.WatchableDBGetter
			if err := getter.Get(config.ChangeStreamName, &dbGetter); err != nil {
				return nil, errors.Trace(err)
			}

			tracingService, err := config.GetTracingService(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			config.Logger.Infof(ctx, "starting controller trace worker using workload tracing config")

			w, err := NewWorker(WorkerConfig{
				Clock:                 config.Clock,
				Logger:                config.Logger,
				NewTracerWorker:       config.NewTracerWorker,
				Tag:                   currentConfig.Tag(),
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

func tracerOutput(in worker.Worker, out any) error {
	if w, ok := in.(*tracerWorker); ok {
		return tracerSetOutput(w, out)
	}
	if w, ok := in.(*noopWorker); ok {
		return tracerSetOutput(w, out)
	}
	return errors.Errorf("expected input of type TracerGetter, got %T", in)
}

func tracerSetOutput(in TracerGetter, out any) error {
	switch out := out.(type) {
	case *TracerGetter:
		var target TracerGetter = in
		*out = target
	default:
		return errors.Errorf("expected output of TracerGetter, got %T", out)
	}
	return nil
}

type unitRuntimeConfigProvider struct {
	config agent.Config
}

// CurrentRuntimeConfig returns the current runtime config for the unit trace
// worker.
func (p unitRuntimeConfigProvider) CurrentRuntimeConfig(context.Context) (RuntimeConfig, error) {
	return RuntimeConfig{
		Enabled:               p.config.OpenTelemetryEnabled(),
		Endpoint:              p.config.OpenTelemetryEndpoint(),
		InsecureSkipVerify:    p.config.OpenTelemetryInsecure(),
		StackTracesEnabled:    p.config.OpenTelemetryStackTraces(),
		SampleRatio:           p.config.OpenTelemetrySampleRatio(),
		TailSamplingThreshold: p.config.OpenTelemetryTailSamplingThreshold(),
	}, nil
}

// WatchRuntimeConfig returns an empty watcher, as the unit trace worker does
// not support hot-reloading of the tracing configuration.
func (p unitRuntimeConfigProvider) WatchRuntimeConfig(context.Context) (watcher.NotifyWatcher, error) {
	return emptyNotifyWatcher(), nil
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
	var controllerServices services.ControllerDomainServices
	if err := getter.Get(name, &controllerServices); err != nil {
		return nil, err
	}
	return controllerServices.Tracing(), nil
}

// emptyNotifyWatcher is a watcher that will just prime the watcher as a notify
// watcher. This will broadcast an initial empty struct{} value to ensure that
// any watchers that are waiting for changes will receive an initial
// notification.
func emptyNotifyWatcher() watcher.NotifyWatcher {
	var empty struct{}
	ch := make(chan struct{}, 1)
	ch <- empty
	w := &emptyWatcher{
		ch: ch,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		close(w.ch)
		return tomb.ErrDying
	})
	return w
}

type emptyWatcher struct {
	tomb tomb.Tomb
	ch   chan struct{}
}

func (w *emptyWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *emptyWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *emptyWatcher) Changes() <-chan struct{} {
	return w.ch
}

func (w *emptyWatcher) Report(_ context.Context) map[string]any {
	return map[string]any{
		"type": "emptyNotifyWatcher",
	}
}
