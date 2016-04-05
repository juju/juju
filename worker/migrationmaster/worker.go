// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/api/migrationtarget"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/fortress"
)

var (
	logger           = loggo.GetLogger("juju.worker.migrationmaster")
	apiOpen          = api.Open
	tempSuccessSleep = 10 * time.Second

	ErrDone = errors.New("done processing migration")
)

type Facade interface {

	// Watch returns a watcher which reports when a migration is
	// active for the model associated with the API connection.
	Watch() (watcher.NotifyWatcher, error)

	// GetMigrationStatus returns the details and progress of the
	// latest model migration.
	GetMigrationStatus() (migrationmaster.MigrationStatus, error)

	// SetPhase updates the phase of the currently active model
	// migration.
	SetPhase(migration.Phase) error

	// Export returns a serialized representation of the model
	// associated with the API connection.
	Export() ([]byte, error)
}

type Config struct {
	Facade Facade
	Guard  fortress.Guard
}

func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Guard != nil {
		return errors.NotValidf("nil Guard")
	}
	return nil
}

// New starts a migration master worker using the supplied migration
// master API facade.
func New(config Config) (*Master, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &Master{
		config: config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.run,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type Master struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill implements worker.Worker.
func (w *Master) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *Master) Wait() error {
	return w.catacomb.Wait()
}

func (w *Master) run() error {
	// TODO(mjs) - log messages should indicate the model name and
	// UUID. Independent logger per migration master instance?
	// XXX(fwereade): +1 to context-aware loggers.

	status, err := w.waitForActiveMigration()
	if err != nil {
		return errors.Trace(err)
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
			return w.catacomb.ErrDying()
		}

		logger.Infof("setting migration phase to %s", phase)
		if err := w.config.Facade.SetPhase(phase); err != nil {
			return errors.Annotate(err, "failed to set phase")
		}
	}
}

func (w *Master) killed() bool {
	select {
	case <-w.catacomb.Dying():
		return true
	default:
		return false
	}
}

func (w *Master) doQUIESCE() (migration.Phase, error) {
	// TODO(mjs) - Wait for all agents to report back.
	return migration.READONLY, nil
}

func (w *Master) doREADONLY() (migration.Phase, error) {
	// TODO(mjs) - To be implemented.
	return migration.IMPORT, nil
}

func (w *Master) doIMPORT(targetInfo migration.TargetInfo) (migration.Phase, error) {
	logger.Infof("exporting model")
	bytes, err := w.config.Facade.Export()
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

func (w *Master) doVALIDATION() (migration.Phase, error) {
	// TODO(mjs) - Wait for all agents to report back.
	return migration.SUCCESS, nil
}

func (w *Master) doSUCCESS() (migration.Phase, error) {
	// XXX(mjs) - this is a horrible hack, which helps to ensure that
	// minions will see the SUCCESS state (due to watcher event
	// coalescing). It will go away soon.
	time.Sleep(tempSuccessSleep)
	return migration.LOGTRANSFER, nil
}

func (w *Master) doLOGTRANSFER() (migration.Phase, error) {
	// TODO(mjs) - To be implemented.
	return migration.REAP, nil
}

func (w *Master) doREAP() (migration.Phase, error) {
	// TODO(mjs) - To be implemented.
	return migration.DONE, nil
}

func (w *Master) doDONE() (migration.Phase, error) {
	return migration.NONE, nil
}

func (w *Master) doREAPFAILED() (migration.Phase, error) {
	return migration.NONE, nil
}

func (w *Master) doABORT(targetInfo migration.TargetInfo, modelUUID string) (migration.Phase, error) {
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
	err = targetClient.Abort(modelUUID)
	return errors.Trace(err)
}

func (w *Master) waitForActiveMigration() (*migrationmaster.MigrationStatus, error) {
	watcher, err := w.config.Facade.Watch()
	if err != nil {
		return nil, errors.Annotate(err, "watching for migration")
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return nil, errors.Trace(err)
	}
	defer watcher.Kill()

	for {
		select {
		case <-w.catacomb.Dying():
			return nil, w.catacomb.ErrDying()
		case <-watcher.Changes():
		}
		status, err := w.config.Facade.GetMigrationStatus()
		if err != nil {
			return nil, errors.Annotate(err, "retrieving migration status")
		}
		if status.Phase == migration.NONE {
			if err := w.config.Guard.Unlock(); err != nil {
				return nil, errors.Trace(err)
			}
			continue
		}

		// XXX(fwereade): as in migrationminion, this could
		// plausibly send an out-of-date value. Similarly,
		// probably doesn't actually matter, but still bugs me.
		err = w.config.Guard.Lockdown(w.catacomb.Dying())
		if errors.Cause(err) == fortress.ErrAborted {
			return nil, w.catacomb.ErrDying()
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return &status, nil
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
