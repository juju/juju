// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
)

// FileNotifyWatcher represents a way to watch for changes in a namespace folder
// directory.
type FileNotifyWatcher interface {
	// Changes returns a channel for the given file name that will contain
	// if a file was created or deleted.
	// TODO (stickupkid): We could further advance this to return a channel
	// of ints that represents the number of changes we want to advance per
	// step.
	Changes(fileName string) (<-chan bool, error)
}

// WorkerConfig encapsulates the configuration options for the
// changestream worker.
type WorkerConfig struct {
	Clock             clock.Clock
	Logger            logger.Logger
	NewWatcher        WatcherFn
	NewINotifyWatcher func() (INotifyWatcher, error)
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.NewWatcher == nil {
		return errors.NotValidf("missing NewWatcher")
	}

	return nil
}

type fileNotifyWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb
	runner   *worker.Runner
}

func newWorker(cfg WorkerConfig) (*fileNotifyWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "file-notify-watcher",
		// Prevent the runner from restarting the worker, if one of the
		// workers dies, we want to stop the whole thing.
		IsFatal: func(err error) bool { return false },
		Clock:   cfg.Clock,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &fileNotifyWorker{
		cfg:    cfg,
		runner: runner,
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.runner,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *fileNotifyWorker) loop() (err error) {
	defer w.runner.Kill()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *fileNotifyWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *fileNotifyWorker) Wait() error {
	return w.catacomb.Wait()
}

// Changes returns a channel containing all the change events for the given
// fileName.
func (w *fileNotifyWorker) Changes(fileName string) (<-chan bool, error) {
	if fw, err := w.runner.Worker(fileName, w.catacomb.Dying()); err == nil {
		return fw.(FileWatcher).Changes(), nil
	}

	watcher, err := w.cfg.NewWatcher(fileName, WithLogger(w.cfg.Logger), WithINotifyWatcherFn(w.cfg.NewINotifyWatcher))
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := w.runner.StartWorker(context.TODO(), fileName, func(ctx context.Context) (worker.Worker, error) {
		return watcher, nil
	}); err != nil {
		return nil, errors.Annotatef(err, "starting worker for fileName %q", fileName)
	}

	return watcher.Changes(), nil
}
