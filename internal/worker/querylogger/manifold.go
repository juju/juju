// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package querylogger

import (
	"context"
	"runtime/debug"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/common"
)

// ManifoldConfig contains:
// - The names of other manifolds on which the DB accessor depends.
// - Other dependencies from ManifoldsConfig required by the worker.
type ManifoldConfig struct {
	LogDir string
	Clock  clock.Clock
	Logger logger.Logger
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.LogDir == "" {
		return errors.NotValidf("empty LogDir")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the query logger
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Output: output,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			cfg := &WorkerConfig{
				LogDir: config.LogDir,
				Clock:  config.Clock,
				Logger: config.Logger,
				StackGatherer: func() []byte {
					// TODO (stickupkid): Drop the first frames that don't
					// include the slow query logger.
					return debug.Stack()
				},
			}

			w, err := newWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

func output(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*loggerWorker)
	if !ok {
		return errors.Errorf("expected input of type dbWorker, got %T", in)
	}

	switch out := out.(type) {
	case *coredatabase.SlowQueryLogger:
		var target coredatabase.SlowQueryLogger = w
		*out = target
	default:
		return errors.Errorf("expected output of *database.SlowQueryLogger, got %T", out)
	}
	return nil
}
