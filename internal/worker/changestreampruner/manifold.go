// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
)

// NewWorkerFunc function that allows the creation of ChangeStreamPruner.
type NewWorkerFunc func(WorkerConfig) (worker.Worker, error)

// NewModelPrunerFunc is a function that creates a ModelPruner for a given model
type NewModelPrunerFunc func(
	db coredatabase.TxnRunner,
	namespace string,
	initialWindow window,
	updateWindow WindowUpdaterFunc,
	clock clock.Clock,
	logger logger.Logger,
) worker.Worker

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	DBAccessor string

	Clock          clock.Clock
	Logger         logger.Logger
	NewWorker      NewWorkerFunc
	NewModelPruner NewModelPrunerFunc
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.DBAccessor == "" {
		return errors.NotValidf("empty DBAccessorName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if cfg.NewModelPruner == nil {
		return errors.NotValidf("nil NewModelPruner")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the changestream
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DBAccessor,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var dbGetter DBGetter
			if err := getter.Get(config.DBAccessor, &dbGetter); err != nil {
				return nil, errors.Trace(err)
			}

			cfg := WorkerConfig{
				DBGetter:       dbGetter,
				NewModelPruner: config.NewModelPruner,
				Clock:          config.Clock,
				Logger:         config.Logger,
			}

			w, err := config.NewWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

func NewWorker(cfg WorkerConfig) (worker.Worker, error) {
	return newWorker(cfg)
}
