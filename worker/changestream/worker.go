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
	"github.com/juju/juju/worker/changestream/eventqueue"
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
	EventQueue(string) (EventQueue, error)
}

// EventQueue represents an interface for managing subscriptions to listen to
// changes from the database change log.
type EventQueue interface {
	// Subscribe returns a new subscription to listen to changes from the
	// database change log.
	Subscribe(...changestream.SubscriptionOption) (changestream.Subscription, error)
}

// EventQueueWorker represents a worker for subscribing to events from the
// database change log.
type EventQueueWorker interface {
	worker.Worker
	EventQueue() EventQueue
}

// WorkerConfig encapsulates the configuration options for the
// changestream worker.
type WorkerConfig struct {
	DBGetter            DBGetter
	FileNotifyWatcher   FileNotifyWatcher
	Clock               clock.Clock
	Logger              Logger
	NewEventQueueWorker EventQueueWorkerFn
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
	if c.NewEventQueueWorker == nil {
		return errors.NotValidf("missing NewEventQueueWorker")
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

// EventQueue returns a new EventQueue for the given namespace. The EventQueue
// will be subscribed to the given options.
func (w *changeStreamWorker) EventQueue(namespace string) (EventQueue, error) {
	if e, err := w.runner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return e.(EventQueueWorker).EventQueue(), nil
	}

	db, err := w.cfg.DBGetter.GetDB(namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

	eqWorker, err := w.cfg.NewEventQueueWorker(db, fileNotifyWatcher{
		fileNotifier: w.cfg.FileNotifyWatcher,
		fileName:     namespace,
	}, w.cfg.Clock, w.cfg.Logger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := w.runner.StartWorker(namespace, func() (worker.Worker, error) {
		return eqWorker, nil
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return eqWorker.EventQueue(), nil
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

// NewEventQueueWorker creates a new EventQueueWorker.
func NewEventQueueWorker(db coredatabase.TrackedDB, fileNotifier FileNotifier, clock clock.Clock, logger Logger) (EventQueueWorker, error) {
	stream := stream.New(db, fileNotifier, clock, logger)

	eventQueue, err := eventqueue.New(stream, logger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &eventQueueWorker{
		eventQueue: eventQueue,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			stream,
			eventQueue,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// eventQueueWorker is a worker that is responsible for managing the lifecycle
// of both the DBStream and the EventQueue.
type eventQueueWorker struct {
	catacomb catacomb.Catacomb

	eventQueue *eventqueue.EventQueue
}

// Kill is part of the worker.Worker interface.
func (w *eventQueueWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *eventQueueWorker) Wait() error {
	return w.catacomb.Wait()
}

// EventQueue returns the event queue for this worker.
func (w *eventQueueWorker) EventQueue() EventQueue {
	return w.eventQueue
}

func (w *eventQueueWorker) loop() error {
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	}
}
