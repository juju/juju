// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/errors"

	minionapi "github.com/juju/juju/api/migrationminion"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

// New starts a migration minion worker using the supplied migration
// minion API facade.
func New(client minionapi.Client) (worker.Worker, error) {
	w := &migrationMinion{
		client: client,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type migrationMinion struct {
	catacomb catacomb.Catacomb
	client   minionapi.Client
}

// Kill implements worker.Worker.
func (w *migrationMinion) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *migrationMinion) Wait() error {
	return w.catacomb.Wait()
}

func (w *migrationMinion) loop() error {
	watcher, err := w.client.Watch()
	if err != nil {
		return errors.Annotate(err, "setting up watcher")
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case status, ok := <-watcher.Changes():
			if !ok {
				return errors.New("watcher channel closed")
			}
			switch status.Phase {
			case migration.QUIESCE:
				// TODO(mjs) - once Will's stable mode work comes
				// together this worker will only start up when a
				// migration is active. Here the minion should report
				// to the controller that it is running so that the
				// migration can progress to READONLY.
			case migration.SUCCESS:
				// TODO(mjs) - API cutover goes here.
			case migration.LOGTRANSFER, migration.ABORT, migration.REAPFAILED:
				// TODO(mjs) - exit here once Will's stable mode work
				// comes together. The minion is done if these phases
				// are reached.
			default:
				// The minion doesn't need to do anything for other
				// migration phases.
			}
		}
	}
}
