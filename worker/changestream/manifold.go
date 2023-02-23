// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/worker/common"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
}

// StreamFn is an alias function that allows the creation of a DBStream.
type StreamFn = func(*sql.DB, clock.Clock, Logger) DBStream

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	DBAccessor string
	Clock      clock.Clock
	Logger     Logger
	NewStream  StreamFn
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
	if cfg.NewStream == nil {
		return errors.NotValidf("nil NewStream")
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
		Output: changeStreamOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var dbGetter DBGetter
			if err := context.Get(config.DBAccessor, &dbGetter); err != nil {
				return nil, err
			}

			cfg := WorkerConfig{
				DBGetter:  dbGetter,
				Clock:     config.Clock,
				Logger:    config.Logger,
				NewStream: config.NewStream,
			}

			w, err := newWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

func changeStreamOutput(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*changeStreamWorker)
	if !ok {
		return errors.Errorf("in should be a *changeStreamWorker; got %T", in)
	}

	switch out := out.(type) {
	case *ChangeStream:
		var target ChangeStream = w
		*out = target
	default:
		return errors.Errorf("out should be a *changestream.ChangeStream; got %T", out)
	}
	return nil
}
