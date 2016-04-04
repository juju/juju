// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/api"
	masterapi "github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/api/migrationtarget"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/worker"
)

var (
	logger           = loggo.GetLogger("juju.worker.migrationmaster")
	apiOpen          = api.Open
	tempSuccessSleep = 10 * time.Second

	ErrDone = errors.New("done processing migration")
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
	// TODO(mjs) - log messages should indicate the model name and
	// UUID. Independent logger per migration master instance?

	if err := w.waitForActiveMigration(); err != nil {
		return errors.Trace(err)
	}

	status, err := w.client.GetMigrationStatus()
	if err != nil {
		return errors.Annotate(err, "retrieving migration status")
	}

	phase := status.Phase
	for {
		var err error
		switch phase {
		case migration.QUIESCE:
			phase, err = w.doQUIESCE()
		case migration.READONLY:
			phase, err = w.doREADONLY()
		case migration.IMPORT:
			phase, err = w.doIMPORT(status.TargetInfo)
		case migration.VALIDATION:
			phase, err = w.doVALIDATION()
		case migration.SUCCESS:
			phase, err = w.doSUCCESS()
		case migration.LOGTRANSFER:
			phase, err = w.doLOGTRANSFER()
		case migration.REAP:
			phase, err = w.doREAP()
		case migration.DONE:
			phase, err = w.doDONE()
		case migration.REAPFAILED:
			phase, err = w.doREAPFAILED()
		case migration.ABORT:
			phase, err = w.doABORT(status.TargetInfo, status.ModelUUID)
		default:
			return errors.Errorf("unknown phase: %v [%d]", phase.String(), phase)
		}

		if err != nil {
			// A phase handler should only return an error if the
			// migration master should exit. In the face of other
			// errors the handler should log the problem and then
			// return the appropriate error phases to transition to -
			// i.e. ABORT or REAPFAILED)
			return errors.Trace(err)
		}

		if phase == migration.NONE {
			return ErrDone
		}

		if w.killed() {
			return tomb.ErrDying
		}

		logger.Infof("setting migration phase to %s", phase)
		if err := w.client.SetPhase(phase); err != nil {
			return errors.Annotate(err, "failed to set phase")
		}
	}
}

func (w *migrationMaster) killed() bool {
	select {
	case <-w.tomb.Dying():
		return true
	default:
		return false
	}
}

func (w *migrationMaster) doQUIESCE() (migration.Phase, error) {
	// TODO(mjs) - Wait for all agents to report back.
	return migration.READONLY, nil
}

func (w *migrationMaster) doREADONLY() (migration.Phase, error) {
	// TODO(mjs) - To be implemented.
	return migration.IMPORT, nil
}

func (w *migrationMaster) doIMPORT(targetInfo migration.TargetInfo) (migration.Phase, error) {
	logger.Infof("exporting model")
	bytes, err := w.client.Export()
	if err != nil {
		logger.Errorf("model export failed: %v", err)
		return migration.ABORT, nil
	}

	logger.Infof("opening API connection to target controller")
	conn, err := openAPIConn(targetInfo)
	if err != nil {
		logger.Errorf("failed to connect to target controller: %v", err)
		return migration.ABORT, nil
	}
	defer conn.Close()

	logger.Infof("importing model into target controller")
	targetClient := migrationtarget.NewClient(conn)
	err = targetClient.Import(bytes)
	if err != nil {
		logger.Errorf("failed to import model into target controller: %v", err)
		return migration.ABORT, nil
	}

	return migration.VALIDATION, nil
}

func (w *migrationMaster) doVALIDATION() (migration.Phase, error) {
	// TODO(mjs) - Wait for all agents to report back.
	return migration.SUCCESS, nil
}

func (w *migrationMaster) doSUCCESS() (migration.Phase, error) {
	// XXX(mjs) - this is a horrible hack, which helps to ensure that
	// minions will see the SUCCESS state (due to watcher event
	// coalescing). It will go away soon.
	time.Sleep(tempSuccessSleep)
	return migration.LOGTRANSFER, nil
}

func (w *migrationMaster) doLOGTRANSFER() (migration.Phase, error) {
	// TODO(mjs) - To be implemented.
	return migration.REAP, nil
}

func (w *migrationMaster) doREAP() (migration.Phase, error) {
	// TODO(mjs) - To be implemented.
	return migration.DONE, nil
}

func (w *migrationMaster) doDONE() (migration.Phase, error) {
	return migration.NONE, nil
}

func (w *migrationMaster) doREAPFAILED() (migration.Phase, error) {
	return migration.NONE, nil
}

func (w *migrationMaster) doABORT(targetInfo migration.TargetInfo, modelUUID string) (migration.Phase, error) {
	if err := removeImportedModel(targetInfo, modelUUID); err != nil {
		// This isn't fatal. Removing the imported model is a best
		// efforts attempt.
		logger.Errorf("failed to reverse model import: %v", err)
	}
	return migration.NONE, nil
}

func removeImportedModel(targetInfo migration.TargetInfo, modelUUID string) error {
	conn, err := openAPIConn(targetInfo)
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	targetClient := migrationtarget.NewClient(conn)
	err = targetClient.Abort(names.NewModelTag(modelUUID))
	return errors.Trace(err)
}

func (w *migrationMaster) waitForActiveMigration() error {
	watcher, err := w.client.Watch()
	if err != nil {
		return errors.Annotate(err, "watching for migration")
	}
	defer worker.Stop(watcher)

	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case <-watcher.Changes():
		return nil
	}
}

func openAPIConn(targetInfo migration.TargetInfo) (api.Connection, error) {
	apiInfo := &api.Info{
		Addrs:    targetInfo.Addrs,
		CACert:   targetInfo.CACert,
		Tag:      targetInfo.AuthTag,
		Password: targetInfo.Password,
	}
	// Use zero DialOpts (no retries) because the worker must stay
	// responsive to Kill requests. We don't want it to be blocked by
	// a long set of retry attempts.
	return apiOpen(apiInfo, api.DialOpts{})
}
