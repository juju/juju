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
	"github.com/juju/juju/worker/changestream/eventmultiplexer"
	"github.com/juju/juju/worker/changestream/stream"
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

// ChangeStream represents an interface for getting an event queue for
// a particular namespace.
type ChangeStream interface {
	NamespacedEventMux(string) (changestream.EventSource, error)
}

// EventMultiplexerWorker represents a worker for subscribing to events that
// will be multiplexer to subscribers from the database change log.
type EventMultiplexerWorker interface {
	worker.Worker
	EventSource() changestream.EventSource
}

// WorkerConfig encapsulates the configuration options for the
// changestream worker.
type WorkerConfig struct {
	DBGetter                  DBGetter
	FileNotifyWatcher         FileNotifyWatcher
	Clock                     clock.Clock
	Logger                    Logger
	NewEventMultiplexerWorker EventMultiplexerWorkerFn
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
	if c.NewEventMultiplexerWorker == nil {
		return errors.NotValidf("missing NewEventMultiplexerWorker")
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

// NamespacedEventMux returns a new EventMultiplexer for the given namespace.
// The EventMultiplexer will be subscribed to the given options.
func (w *changeStreamWorker) NamespacedEventMux(namespace string) (changestream.EventSource, error) {
	if err := w.runner.StartWorker(namespace, func() (worker.Worker, error) {
		db, err := w.cfg.DBGetter.GetDB(namespace)
		if err != nil {
			return nil, errors.Trace(err)
		}

		mux, err := w.cfg.NewEventMultiplexerWorker(db, fileNotifyWatcher{
			fileNotifier: w.cfg.FileNotifyWatcher,
			fileName:     namespace,
		}, w.cfg.Clock, w.cfg.Logger)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return mux, nil
	}); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return nil, errors.Trace(err)
	}

	mux, err := w.runner.Worker(namespace, w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return mux.(EventMultiplexerWorker).EventSource(), nil
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

// NewEventMultiplexerWorker creates a new EventMultiplexerWorker.
func NewEventMultiplexerWorker(
	db coredatabase.TxnRunner, fileNotifier FileNotifier, clock clock.Clock, logger Logger,
) (EventMultiplexerWorker, error) {
	stream := stream.New(db, fileNotifier, clock, logger)

	mux, err := eventmultiplexer.New(stream, clock, logger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &eventMultiplexerWorker{
		mux: mux,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			stream,
			mux,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// eventMultiplexerWorker is a worker that is responsible for managing the lifecycle
// of both the DBStream and the EventQueue.
type eventMultiplexerWorker struct {
	catacomb catacomb.Catacomb

	mux *eventmultiplexer.EventMultiplexer
}

// Kill is part of the worker.Worker interface.
func (w *eventMultiplexerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *eventMultiplexerWorker) Wait() error {
	return w.catacomb.Wait()
}

// EventSource returns the event source for this worker.
func (w *eventMultiplexerWorker) EventSource() changestream.EventSource {
	return w.mux
}

func (w *eventMultiplexerWorker) loop() error {
	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}
