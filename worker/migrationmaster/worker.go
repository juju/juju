// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/api"
	masterapi "github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/api/migrationtarget"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.migrationmaster")

var apiOpen = api.Open

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
	// TODO(mjs) - run the migration phase changes and abort the
	// migration when things go wrong.

	// TODO(mjs) - more logging when things go wrong.

	targetInfo, err := w.waitForMigration()
	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof("exporting model")
	bytes, err := w.client.Export()
	if err != nil {
		return errors.Annotate(err, "model export")
	}

	logger.Infof("opening API connection to target controller")
	conn, err := openAPIConn(targetInfo)
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	logger.Infof("importing model into target controller")
	targetClient := migrationtarget.NewClient(conn)
	err = targetClient.Import(bytes)
	if err != nil {
		return errors.Annotate(err, "model import")
	}

	// For now just abort the migration (this is a work in progress)
	err = w.client.SetPhase(migration.ABORT)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.New("migration seen and aborted")
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

func openAPIConn(targetInfo *migration.TargetInfo) (api.Connection, error) {
	apiInfo := &api.Info{
		Addrs:    targetInfo.Addrs,
		CACert:   targetInfo.CACert,
		Tag:      targetInfo.AuthTag,
		Password: targetInfo.Password,
	}
	return apiOpen(apiInfo, api.DefaultDialOpts())
}
