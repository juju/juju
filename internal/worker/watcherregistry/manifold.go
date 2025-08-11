// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcherregistry

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	// NewWorker creates a new watcher registry worker.
	NewWorker func(Config) (worker.Worker, error)
	// Clock is the clock used by the worker.
	Clock clock.Clock
	// Logger is the logger used by the worker.
	Logger logger.Logger
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}

	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a log sink
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Output: outputFunc,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				Clock:  config.Clock,
				Logger: config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// outputFunc extracts an API connection from a *apiConnWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Unwrap()
	}
	inWorker, _ := in.(*Worker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *WatcherRegistryGetter:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be *WatcherRegistryGetter; got %T", out)
	}
	return nil
}
