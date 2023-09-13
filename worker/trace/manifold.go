// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	coretrace "github.com/juju/juju/core/trace"
)

// TracerGetter is the interface that is used to get a tracer.
type TracerGetter interface {
	// GetTracer returns a tracer for the given namespace.
	GetTracer(namespace string) (coretrace.Tracer, error)
}

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})

	IsTraceEnabled() bool
	IsLevelEnabled(loggo.Level) bool
}

// TracerWorkerFunc is the function signature for creating a new tracer worker.
type TracerWorkerFunc func(context.Context, string, string, bool, bool, Logger) (TrackedTracer, error)

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	AgentName       string
	Clock           clock.Clock
	Logger          Logger
	NewTracerWorker TracerWorkerFunc
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
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
		},
		Output: tracerOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := context.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			currentConfig := a.CurrentConfig()

			// For the current implementation, if trace is disabled, return
			// a noop worker. If the open telemetry does change, then we will
			// bounce the world and this will be re-evaluated.
			if !currentConfig.OpenTelemetryEnabled() {
				return NewNoopWorker(), nil
			}

			w, err := NewWorker(WorkerConfig{
				Clock:              config.Clock,
				Logger:             config.Logger,
				NewTracerWorker:    config.NewTracerWorker,
				Endpoint:           currentConfig.OpenTelemetryEndpoint(),
				InsecureSkipVerify: currentConfig.OpenTelemetryInsecure(),
				StackTracesEnabled: currentConfig.OpenTelemetryStackTraces(),
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w, nil
		},
	}
}

func tracerOutput(in worker.Worker, out interface{}) error {
	w, ok := in.(*tracerWorker)
	if !ok {
		return errors.Errorf("expected input of type traceW, got %T", in)
	}

	switch out := out.(type) {
	case *TracerGetter:
		var target TracerGetter = w
		*out = target
	default:
		return errors.Errorf("expected output of Tracer, got %T", out)
	}
	return nil
}
