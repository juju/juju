// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
)

// FileNotifyWatcher represents a way to watch for changes in a namespace folder
// directory.
type FileNotifyWatcher interface {
	// Changes returns a channel for the given namespace that will contain
	// if a file was created or deleted.
	// TODO (stickupkid): We could further advance this to return a channel
	// of ints that represents the number of changes we want to advance per
	// step.
	Changes(namespace string) (<-chan bool, error)
}

// WorkerConfig encapsulates the configuration options for the
// changestream worker.
type WorkerConfig struct {
	Clock      clock.Clock
	Logger     Logger
	NewWatcher WatcherFn
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

	w := &fileNotifyWorker{
		cfg: cfg,
		runner: worker.NewRunner(worker.RunnerParams{
			// Prevent the runner from restarting the worker, if one of the
			// workers dies, we want to stop the whole thing.
			IsFatal: func(err error) bool { return false },
			Clock:   cfg.Clock,
		}),
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
// namespace.
func (w *fileNotifyWorker) Changes(namespace string) (<-chan bool, error) {
	if w, err := w.runner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return w.(FileWatcher).Changes(), nil
	}

	watcher, err := w.cfg.NewWatcher(namespace, WithLogger(w.cfg.Logger))
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := w.runner.StartWorker(namespace, func() (worker.Worker, error) {
		return watcher, nil
	}); err != nil {
		return nil, errors.Annotatef(err, "starting worker for namespace %q", namespace)
	}

	return watcher.Changes(), nil
}
