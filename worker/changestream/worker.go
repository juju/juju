// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/worker/filenotifywatcher"
)

// DBGetter describes the ability to supply a sql.DB
// reference for a particular database.
type DBGetter = coredatabase.DBGetter

// FileNotifyWatcher is the interface that the worker uses to interact with the
// file notify watcher.
type FileNotifyWatcher = filenotifywatcher.FileNotifyWatcher

// FileNotifier represents a way to watch for changes in a namespace folder
// directory.
type FileNotifier interface {
	// Changes returns a channel if a file was created or deleted.
	Changes() (<-chan bool, error)
}

// WorkerConfig encapsulates the configuration options for the
// changestream worker.
type WorkerConfig struct {
	AgentTag          string
	DBGetter          DBGetter
	FileNotifyWatcher FileNotifyWatcher
	Clock             clock.Clock
	Logger            Logger
	Metrics           Metrics
	NewWatchableDB    WatchableDBFn
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.AgentTag == "" {
		return errors.NotValidf("missing AgentTag")
	}
	if c.DBGetter == nil {
		return errors.NotValidf("missing DBGetter")
	}
	if c.FileNotifyWatcher == nil {
		return errors.NotValidf("missing FileNotifyWatcher")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	if c.Metrics == nil {
		return errors.NotValidf("missing metrics Collector")
	}
	if c.NewWatchableDB == nil {
		return errors.NotValidf("missing NewWatchableDB")
	}
	return nil
}

type changeStreamWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb
	runner   *worker.Runner
}

func newWorker(cfg WorkerConfig) (*changeStreamWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &changeStreamWorker{
		cfg: cfg,
		runner: worker.NewRunner(worker.RunnerParams{
			// Prevent the runner from restarting the worker, if one of the
			// workers dies, we want to stop the whole thing.
			IsFatal: func(err error) bool {
				return false
			},
			Clock: cfg.Clock,
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

func (w *changeStreamWorker) loop() error {
	defer w.runner.Kill()

	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}

// Kill is part of the worker.Worker interface.
func (w *changeStreamWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *changeStreamWorker) Wait() error {
	return w.catacomb.Wait()
}

// Report returns a map of the worker's status.
func (w *changeStreamWorker) Report() map[string]any {
	return w.runner.Report()
}

// GetWatchableDB returns a new WatchableDB for the given namespace.
func (w *changeStreamWorker) GetWatchableDB(namespace string) (changestream.WatchableDB, error) {
	if mux, err := w.workerFromCache(namespace); err != nil {
		return nil, errors.Trace(err)
	} else if mux != nil {
		return mux, nil
	}

	// If the worker doesn't exist yet, create it.
	if err := w.runner.StartWorker(namespace, func() (worker.Worker, error) {
		db, err := w.cfg.DBGetter.GetDB(namespace)
		if err != nil {
			return nil, errors.Trace(err)
		}

		mux, err := w.cfg.NewWatchableDB(w.cfg.AgentTag, db, fileNotifyWatcher{
			fileNotifier: w.cfg.FileNotifyWatcher,
			fileName:     namespace,
		}, w.cfg.Clock, w.cfg.Metrics.ForNamespace(namespace), w.cfg.Logger)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return mux, nil
	}); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return nil, errors.Trace(err)
	}

	// Block until the worker is started and ready to go.
	mux, err := w.runner.Worker(namespace, w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return mux.(WatchableDBWorker), nil
}

func (w *changeStreamWorker) workerFromCache(namespace string) (WatchableDBWorker, error) {
	// If the worker already exists, return the existing worker early.
	if mux, err := w.runner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return mux.(WatchableDBWorker), nil
	} else if errors.Is(errors.Cause(err), worker.ErrDead) {
		// Handle the case where the change stream runner is dead due to this
		// worker dying.
		select {
		case <-w.catacomb.Dying():
			return nil, w.catacomb.ErrDying()
		default:
			return nil, errors.Trace(err)
		}
	} else if !errors.Is(errors.Cause(err), errors.NotFound) {
		// If it's not a NotFound error, return the underlying error. We should
		// only start a worker if it doesn't exist yet.
		return nil, errors.Trace(err)
	}
	// We didn't find the worker, so return nil, we'll create it in the next
	// step.
	return nil, nil
}

// fileNotifyWatcher is a wrapper around the FileNotifyWatcher that is used to
// filter the events to only those that are for the given namespace.
type fileNotifyWatcher struct {
	fileNotifier FileNotifyWatcher
	fileName     string
}

func (f fileNotifyWatcher) Changes() (<-chan bool, error) {
	return f.fileNotifier.Changes(f.fileName)
}
