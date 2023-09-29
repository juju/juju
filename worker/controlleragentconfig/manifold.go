// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
}

// ManifoldConfig defines the configuration for the agent controller config
// manifold.
type ManifoldConfig struct {
	Logger Logger
	Clock  clock.Clock
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Output: configOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				Logger: config.Logger,
				Notify: Notify,
				Clock:  config.Clock,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w, nil
		},
	}
}

func configOutput(in worker.Worker, out any) error {
	return nil
}

// Notify sets up the signal handler for the worker.
func Notify(ctx context.Context, ch chan os.Signal) {
	if ctx.Err() != nil {
		return
	}

	signal.Notify(ch, syscall.SIGHUP)
}
