// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/worker/common"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	LogSinkName string

	// NewWorker creates a status history worker.
	NewWorker func(cfg Config) (worker.Worker, error)
	Clock     clock.Clock
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.LogSinkName == "" {
		return errors.NotValidf("empty LogSinkName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a log sink
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.LogSinkName,
		},
		Output: outputFunc,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var modelLogger logger.ModelLogger
			if err := getter.Get(config.LogSinkName, &modelLogger); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				ModelLogger: modelLogger,
				Clock:       config.Clock,
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
	inWorker, _ := in.(*statusHistoryWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *status.StatusHistoryFactory:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be *status.StatusHistoryFactory; got %T", out)
	}
	return nil
}
