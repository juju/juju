// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/worker/common"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
	IsTraceEnabled() bool
}

// EventQueueWorkerFn is an alias function that allows the creation of
// EventQueueWorker.
type EventQueueWorkerFn = func(coredatabase.TrackedDB, FileNotifier, clock.Clock, Logger) (EventQueueWorker, error)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	DBAccessor        string
	FileNotifyWatcher string

	Clock               clock.Clock
	Logger              Logger
	NewEventQueueWorker EventQueueWorkerFn
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.DBAccessor == "" {
		return errors.NotValidf("empty DBAccessorName")
	}
	if cfg.FileNotifyWatcher == "" {
		return errors.NotValidf("empty FileNotifyWatcherName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewEventQueueWorker == nil {
		return errors.NotValidf("nil NewEventQueueWorker")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the changestream
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DBAccessor,
			config.FileNotifyWatcher,
		},
		Output: changeStreamOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var dbGetter DBGetter
			if err := context.Get(config.DBAccessor, &dbGetter); err != nil {
				return nil, errors.Trace(err)
			}

			var fileNotifyWatcher FileNotifyWatcher
			if err := context.Get(config.FileNotifyWatcher, &fileNotifyWatcher); err != nil {
				return nil, errors.Trace(err)
			}

			cfg := WorkerConfig{
				DBGetter:            dbGetter,
				FileNotifyWatcher:   fileNotifyWatcher,
				Clock:               config.Clock,
				Logger:              config.Logger,
				NewEventQueueWorker: config.NewEventQueueWorker,
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
