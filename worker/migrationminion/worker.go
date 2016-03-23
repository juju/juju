// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/errors"

	minionapi "github.com/juju/juju/api/migrationminion"
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
		}
	}
}
