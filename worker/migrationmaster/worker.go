// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"
	"launchpad.net/tomb"

	masterapi "github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/worker"
)

// New starts a migration master worker using the supplied migration
// master API facade.
func New(client masterapi.Client) worker.Worker {
	w := &migrationMaster{
		client: client,
	}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.run())
	}()
	return w
}

type migrationMaster struct {
	tomb   tomb.Tomb
	client masterapi.Client
}

// Kill implements worker.Worker.
func (w *migrationMaster) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *migrationMaster) Wait() error {
	return w.tomb.Wait()
}

func (w *migrationMaster) run() error {
	targetInfo, err := w.waitForMigration()
	if err != nil {
		return errors.Trace(err)
	}
	var _ = targetInfo
	return errors.New("migration seen")
}

func (w *migrationMaster) waitForMigration() (*migration.TargetInfo, error) {
	watcher, err := w.client.Watch()
	if err != nil {
		return nil, errors.Annotate(err, "watching for migration")
	}
	defer worker.Stop(watcher)

	select {
	case <-w.tomb.Dying():
		return nil, tomb.ErrDying
	case info := <-watcher.Changes():
		return &info, nil
	}
}
