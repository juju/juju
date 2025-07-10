// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl

import (
	"context"
	"io"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/dbreplaccessor"
)

// ManifoldConfig contains:
// - The names of other manifolds on which the DB accessor depends.
// - Other dependencies from ManifoldsConfig required by the worker.
type ManifoldConfig struct {
	DBReplAccessorName string
	Logger             logger.Logger
	Stdout             io.Writer
	Stderr             io.Writer
	Stdin              io.Reader
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.DBReplAccessorName == "" {
		return errors.NotValidf("empty DBReplAccessorName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.Stdout == nil {
		return errors.NotValidf("nil Stdout")
	}
	if cfg.Stderr == nil {
		return errors.NotValidf("nil Stderr")
	}
	if cfg.Stdin == nil {
		return errors.NotValidf("nil Stdin")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the dbaccessor
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DBReplAccessorName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var dbGetter coredatabase.DBGetter
			if err := getter.Get(config.DBReplAccessorName, &dbGetter); err != nil {
				return nil, err
			}

			var clusterIntrospector dbreplaccessor.ClusterIntrospector
			if err := getter.Get(config.DBReplAccessorName, &clusterIntrospector); err != nil {
				return nil, errors.Trace(err)
			}

			cfg := WorkerConfig{
				DBGetter:            dbGetter,
				ClusterIntrospector: clusterIntrospector,
				Logger:              config.Logger,
				Stdout:              config.Stdout,
				Stderr:              config.Stderr,
				Stdin:               config.Stdin,
			}

			return NewWorker(cfg)
		},
	}
}
