// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/worker/dbaccessor"
	"github.com/juju/juju/worker/filenotifywatcher"
)

// DBGetter describes the ability to supply a sql.DB
// reference for a particular database.
type DBGetter = dbaccessor.DBGetter

// FileNotifyWatcher is the interface that the worker uses to interact with the
// file notify watcher.
type FileNotifyWatcher = filenotifywatcher.FileNotifyWatcher

// FileNotifyWatcher represents a way to watch for changes in a namespace folder
// directory.
type FileNotifier interface {
	// Changes returns a channel if a file was created or deleted.
	Changes() (<-chan bool, error)
}

// ChangeStream represents a stream of changes that flows from the underlying
// change log table in the database.
type ChangeStream interface {
	// Changes returns a channel for a given namespace (database).
	// The channel will return events represented by change log rows
	// from the database.
	// The change event IDs will be monotonically increasing
	// (though not necessarily sequential).
	// Events will be coalesced into a single change if they are
	// for the same entity and edit type.
	Changes(namespace string) (<-chan changestream.ChangeEvent, error)
}

// DBStream is the interface that the worker uses to interact with the raw
// database stream. This is not namespaced and works exactly on the raw
// database.
type DBStream interface {
	worker.Worker
	Changes() <-chan changestream.ChangeEvent
}

// WorkerConfig encapsulates the configuration options for the
// changestream worker.
type WorkerConfig struct {
	DBGetter          DBGetter
	FileNotifyWatcher FileNotifyWatcher
	Clock             clock.Clock
	Logger            Logger
	NewStream         StreamFn
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
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

func (w *changeStreamWorker) loop() (err error) {
	defer w.runner.Kill()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *changeStreamWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *changeStreamWorker) Wait() error {
	return w.catacomb.Wait()
}

// Changes returns a channel containing all the change events for the given
// namespace.
func (w *changeStreamWorker) Changes(namespace string) (<-chan changestream.ChangeEvent, error) {
	trackedDB, err := w.cfg.DBGetter.GetDB(namespace)
	if err != nil {
		return nil, errors.Annotatef(err, "getting db for namespace %q", namespace)
	}

	stream, err := w.cfg.NewStream(trackedDB.DB(), fileNotifyWatcher{
		fileNotifier: w.cfg.FileNotifyWatcher,
		fileName:     fmt.Sprintf("change-stream-%s", namespace),
	}, w.cfg.Clock, w.cfg.Logger)
	if err != nil {
		return nil, errors.Annotatef(err, "creating stream for namespace %q", namespace)
	}

	if err := w.runner.StartWorker(namespace, func() (worker.Worker, error) {
		return stream, nil
	}); err != nil {
		return nil, errors.Annotatef(err, "starting worker for namespace %q", namespace)
	}
	return stream.Changes(), nil
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
