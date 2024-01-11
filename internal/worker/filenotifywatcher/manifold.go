// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/internal/worker/common"
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

// WatcherFn is a function that returns a new Watcher.
type WatcherFn = func(string, ...Option) (FileWatcher, error)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	Clock             clock.Clock
	Logger            Logger
	NewWatcher        WatcherFn
	NewINotifyWatcher func() (INotifyWatcher, error)
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWatcher == nil {
		return errors.NotValidf("nil NewWatcher")
	}
	if cfg.NewINotifyWatcher == nil {
		return errors.NotValidf("nil NewINotifyWatcher")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the filenotifywatcher
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Output: fileNotifyWatcherOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			cfg := WorkerConfig{
				Clock:             config.Clock,
				Logger:            config.Logger,
				NewWatcher:        config.NewWatcher,
				NewINotifyWatcher: config.NewINotifyWatcher,
			}

			w, err := newWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

func fileNotifyWatcherOutput(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*fileNotifyWorker)
	if !ok {
		return errors.Errorf("in should be a *fileNotifyWorker; got %T", in)
	}

	switch out := out.(type) {
	case *FileNotifyWatcher:
		var target FileNotifyWatcher = w
		*out = target
	default:
		return errors.Errorf("out should be a *filenotifywatcher.FileNotifyWatcher; got %T", out)
	}
	return nil
}
