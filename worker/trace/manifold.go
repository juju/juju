// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	coretrace "github.com/juju/juju/core/trace"
)

// TracerNamespace is a combination of the service name and the namespace, it
// allows us to uniquely identify a tracer.
type TracerNamespace struct {
	ServiceName string
	Namespace   string
}

// Namespace returns a new namespace.
func Namespace(serviceName, namespace string) TracerNamespace {
	return TracerNamespace{
		ServiceName: serviceName,
		Namespace:   namespace,
	}
}

// ShortNamespace returns a short representation of the namespace.
func (ns TracerNamespace) ShortNamespace() string {
	return ns.Namespace[:6]
}

// String returns a short representation of the namespace.
func (ns TracerNamespace) String() string {
	return fmt.Sprintf("%s:%s", ns.ServiceName, ns.Namespace)
}

// TracerGetter is the interface that is used to get a tracer.
type TracerGetter interface {
	// GetTracer returns a tracer for the given namespace.
	GetTracer(namespace TracerNamespace) (coretrace.Tracer, error)
}

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
	Tracef(message string, args ...any)

	IsTraceEnabled() bool
	IsLevelEnabled(loggo.Level) bool
}

// TracerWorkerFunc is the function signature for creating a new tracer worker.
type TracerWorkerFunc func(ctx context.Context, namespace TracerNamespace, endpoint string, insecureSkipVerify bool, showStackTraces bool, logger Logger) (TrackedTracer, error)

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

func tracerOutput(in worker.Worker, out any) error {
	if w, ok := in.(*noopWorker); ok {
		return tracerSetOutput(w, out)
	}
	if w, ok := in.(*tracerWorker); ok {
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
		return errors.Errorf("expected output of Tracer, got %T", out)
	}
	return nil
}
