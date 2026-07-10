// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
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
	httpEndpoint, grpcEndpoint, caCertificate string,
	insecureSkipVerify bool,
	showStackTraces bool,
	sampleRatio float64,
	tailSamplingThreshold time.Duration,
	logger logger.Logger,
	newClient NewClientFunc,
) (TrackedTracer, error)

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	AgentName          string
	AgentConfigChanged *voyeur.Value
	Clock              clock.Clock
	Logger             logger.Logger
	NewTracerWorker    TracerWorkerFunc
	Kind               coretrace.Kind
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.AgentConfigChanged == nil {
		return errors.NotValidf("nil AgentConfigChanged")
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
			if enabled {
				config.Logger.Infof(ctx,
					"OpenTelemetry enabled, starting trace worker using HTTP endpoint %q and gRPC endpoint %q",
					currentConfig.OpenTelemetryHTTPEndpoint(), currentConfig.OpenTelemetryGRPCEndpoint())
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
					agent:              a,
					agentConfigChanged: config.AgentConfigChanged,
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
	agent              agent.Agent
	agentConfigChanged *voyeur.Value
}

// CurrentRuntimeConfig returns the current runtime config for the unit trace
// worker.
func (p unitRuntimeConfigProvider) CurrentRuntimeConfig(context.Context) (RuntimeConfig, error) {
	config := p.agent.CurrentConfig()
	return RuntimeConfig{
		Enabled:               config.OpenTelemetryEnabled(),
		HTTPEndpoint:          config.OpenTelemetryHTTPEndpoint(),
		GRPCEndpoint:          config.OpenTelemetryGRPCEndpoint(),
		InsecureSkipVerify:    config.OpenTelemetryInsecure(),
		StackTracesEnabled:    config.OpenTelemetryStackTraces(),
		SampleRatio:           config.OpenTelemetrySampleRatio(),
		TailSamplingThreshold: config.OpenTelemetryTailSamplingThreshold(),
	}, nil
}

// WatchRuntimeConfig returns a watcher for local agent config changes.
func (p unitRuntimeConfigProvider) WatchRuntimeConfig(context.Context) (watcher.NotifyWatcher, error) {
	return newAgentConfigChangedWatcher(p.agentConfigChanged), nil
}

func newAgentConfigChangedWatcher(value *voyeur.Value) watcher.NotifyWatcher {
	w := &agentConfigChangedWatcher{
		watch: value.Watch(),
		ch:    make(chan struct{}, 1),
	}
	w.tomb.Go(w.loop)
	return w
}

type agentConfigChangedWatcher struct {
	tomb  tomb.Tomb
	watch *voyeur.Watcher
	ch    chan struct{}
}

func (w *agentConfigChangedWatcher) Kill() {
	w.watch.Close()
	w.tomb.Kill(nil)
}

func (w *agentConfigChangedWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *agentConfigChangedWatcher) Changes() <-chan struct{} {
	return w.ch
}

func (w *agentConfigChangedWatcher) Report(_ context.Context) map[string]any {
	return map[string]any{
		"type": "agentConfigChangedWatcher",
	}
}

func (w *agentConfigChangedWatcher) loop() error {
	defer close(w.ch)
	defer w.watch.Close()

	if err := w.notify(); err != nil {
		return err
	}

	for {
		if !w.watch.Next() {
			return nil
		}
		if err := w.notify(); err != nil {
			return err
		}
	}
}

func (w *agentConfigChangedWatcher) notify() error {
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.ch <- struct{}{}:
		return nil
	}
}
