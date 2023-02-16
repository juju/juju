// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/worker/dbaccessor"
)

// DBGetter describes the ability to supply a sql.DB
// reference for a particular database.
type DBGetter = dbaccessor.DBGetter

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
	DBGetter  DBGetter
	Clock     clock.Clock
	Logger    Logger
	NewStream StreamFn
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
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
}

func newWorker(cfg WorkerConfig) (*changeStreamWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &changeStreamWorker{
		cfg: cfg,
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *changeStreamWorker) loop() (err error) {
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
	db, err := w.cfg.DBGetter.GetDB(namespace)
	if err != nil {
		return nil, errors.Annotatef(err, "getting db for namespace %q", namespace)
	}

	// TODO (stickupkid): We could potentially cache the streams here and hand
	// out the same stream for the same namespace.
	stream := w.cfg.NewStream(db, w.cfg.Clock, w.cfg.Logger)
	if err := w.catacomb.Add(stream); err != nil {
		return nil, errors.Trace(err)
	}
	return stream.Changes(), nil
}
