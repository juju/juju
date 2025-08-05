// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/filenotifywatcher"
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
	Logger            logger.Logger
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

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "change-stream",
		// Prevent the runner from restarting the worker, if one of the
		// workers dies, we want to stop the whole thing.
		IsFatal: func(err error) bool {
			return false
		},
		// ShouldRestart is used to determine if the worker should be
		// restarted. We only want to restart the worker if the error is not
		// ErrDBDead, ErrDBNotFound or NotValid.
		// The ErrDBNotFound error can be returned if the namespace doesn't
		// exist and so can not be retrieved. When this happens, we do not
		// want to restart the worker and instead return the error to the
		// caller.
		// The caller can retry if they want, but internally to the
		// changestream, the worker is dead.
		ShouldRestart: func(err error) bool {
			// This can occur if the database namespace is not valid.
			if errors.Is(err, errors.NotValid) {
				return false
			}

			// The database is not found, we do not want to restart the
			// worker in this case.
			if errors.Is(err, coredatabase.ErrDBNotFound) {
				return false
			}

			// If the database is dead, then we should collapse the whole change
			// stream worker, but this got here first, so we just want to
			// prevent additional noise.
			if errors.Is(err, coredatabase.ErrDBDead) {
				return false
			}

			return true
		},
		Clock:  cfg.Clock,
		Logger: internalworker.WrapLogger(cfg.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &changeStreamWorker{
		cfg:    cfg,
		runner: runner,
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Name: "change-stream",
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
func (w *changeStreamWorker) GetWatchableDB(ctx context.Context, namespace string) (changestream.WatchableDB, error) {
	if mux, err := w.workerFromCache(namespace); err != nil {
		return nil, errors.Trace(err)
	} else if mux != nil {
		return mux, nil
	}

	// If the worker doesn't exist yet, create it.
	if err := w.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		db, err := w.cfg.DBGetter.GetDB(ctx, namespace)
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
			return nil, coredatabase.ErrChangeStreamDying
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
