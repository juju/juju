// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/worker/common"
)

// TracerGetter is the interface that is used to get a tracer.
type TracerGetter interface {
	// GetTracer returns a tracer for the given namespace.
	GetTracer(namespace string) (Tracer, error)
}

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})

	IsTraceEnabled() bool
}

// TracerWorkerFunc is the function signature for creating a new tracer worker.
type TracerWorkerFunc func(context.Context, string) (TrackedTracer, error)

// ManifoldConfig defines the configuration for the tracing manifold.
type ManifoldConfig struct {
	Clock           clock.Clock
	Logger          Logger
	NewTracerWorker TracerWorkerFunc
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
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

// Manifold returns a dependency manifold that runs the tracing worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Output: tracerOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				Clock:           config.Clock,
				Logger:          config.Logger,
				NewTracerWorker: config.NewTracerWorker,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return common.NewCleanupWorker(w, func() {

			}), nil
		},
	}
}

func tracerOutput(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*tracerWorker)
	if !ok {
		return errors.Errorf("expected input of type tracingW, got %T", in)
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
