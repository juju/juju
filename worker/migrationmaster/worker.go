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
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/fortress"
)

var (
	logger           = loggo.GetLogger("juju.worker.migrationmaster")
	apiOpen          = api.Open
	tempSuccessSleep = 10 * time.Second

	// ErrDoneForNow indicates a temporary issue was encountered and
	// that the worker should restart and retry.
	ErrDoneForNow = errors.New("done for now")
)

// Facade exposes controller functionality to a Worker.
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

	// Reap removes all documents of the model associated with the API
	// connection.
	Reap() error
}

// Config defines the operation of a Worker.
type Config struct {
	Facade Facade
	Guard  fortress.Guard
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Guard == nil {
		return errors.NotValidf("nil Guard")
	}
	return nil
}

// New returns a Worker backed by config, or an error.
func New(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &Worker{
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

// Worker waits until a migration is active and its configured
// Fortress is locked down, and then orchestrates a model migration.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill implements worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) run() error {
	status, err := w.waitForActiveMigration()
	if err != nil {
		return errors.Trace(err)
	}

	err = w.config.Guard.Lockdown(w.catacomb.Dying())
	if errors.Cause(err) == fortress.ErrAborted {
		return w.catacomb.ErrDying()
	} else if err != nil {
		return errors.Trace(err)
	}

	// TODO(mjs) - log messages should indicate the model name and
	// UUID. Independent logger per migration master instance?

	phase := status.Phase
	for {
		var err error
		switch phase {
		case migration.QUIESCE:
			phase, err = w.doQUIESCE()
		case migration.READONLY:
			phase, err = w.doREADONLY()
		case migration.PRECHECK:
			phase, err = w.doPRECHECK()
		case migration.IMPORT:
			phase, err = w.doIMPORT(status.TargetInfo)
		case migration.VALIDATION:
			phase, err = w.doVALIDATION(status.TargetInfo, status.ModelUUID)
		case migration.SUCCESS:
			phase, err = w.doSUCCESS()
		case migration.LOGTRANSFER:
			phase, err = w.doLOGTRANSFER()
		case migration.REAP:
			phase, err = w.doREAP()
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

		if w.killed() {
			return w.catacomb.ErrDying()
		}

		logger.Infof("setting migration phase to %s", phase)
		if err := w.config.Facade.SetPhase(phase); err != nil {
			return errors.Annotate(err, "failed to set phase")
		}

		if modelHasMigrated(phase) {
			// TODO(mjs) - use manifold Filter so that the dep engine
			// error types aren't required here.
			return dependency.ErrUninstall
		} else if phase.IsTerminal() {
			// Some other terminal phase, exit and try again.
			return ErrDoneForNow
		}
	}
}

func (w *Worker) killed() bool {
	select {
	case <-w.catacomb.Dying():
		return true
	default:
		return false
	}
}

func (w *Worker) doQUIESCE() (migration.Phase, error) {
	// TODO(mjs) - Wait for all agents to report back.
	return migration.READONLY, nil
}

func (w *Worker) doREADONLY() (migration.Phase, error) {
	// TODO(mjs) - To be implemented.
	return migration.PRECHECK, nil
}

func (w *Worker) doPRECHECK() (migration.Phase, error) {
	// TODO(mjs) - To be implemented.
	return migration.IMPORT, nil
}

func (w *Worker) doIMPORT(targetInfo migration.TargetInfo) (migration.Phase, error) {
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

func (w *Worker) doVALIDATION(targetInfo migration.TargetInfo, modelUUID string) (migration.Phase, error) {
	// TODO(mjs) - Wait for all agents to report back.

	// Once all agents have validated, activate the model.
	err := activateModel(targetInfo, modelUUID)
	if err != nil {
		return migration.ABORT, nil
	}
	return migration.SUCCESS, nil
}

func activateModel(targetInfo migration.TargetInfo, modelUUID string) error {
	conn, err := openAPIConn(targetInfo)
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	targetClient := migrationtarget.NewClient(conn)
	err = targetClient.Activate(modelUUID)
	return errors.Trace(err)
}

func (w *Worker) doSUCCESS() (migration.Phase, error) {
	// XXX(mjs) - this is a horrible hack, which helps to ensure that
	// minions will see the SUCCESS state (due to watcher event
	// coalescing). It will go away soon.
	time.Sleep(tempSuccessSleep)
	return migration.LOGTRANSFER, nil
}

func (w *Worker) doLOGTRANSFER() (migration.Phase, error) {
	// TODO(mjs) - To be implemented.
	return migration.REAP, nil
}

func (w *Worker) doREAP() (migration.Phase, error) {
	err := w.config.Facade.Reap()
	if err != nil {
		return migration.REAPFAILED, errors.Trace(err)
	}
	return migration.DONE, nil
}

func (w *Worker) doABORT(targetInfo migration.TargetInfo, modelUUID string) (migration.Phase, error) {
	if err := removeImportedModel(targetInfo, modelUUID); err != nil {
		// This isn't fatal. Removing the imported model is a best
		// efforts attempt.
		logger.Errorf("failed to reverse model import: %v", err)
	}
	return migration.ABORTDONE, nil
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

func (w *Worker) waitForActiveMigration() (migrationmaster.MigrationStatus, error) {
	var empty migrationmaster.MigrationStatus

	watcher, err := w.config.Facade.Watch()
	if err != nil {
		return empty, errors.Annotate(err, "watching for migration")
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return empty, errors.Trace(err)
	}
	defer watcher.Kill()

	for {
		select {
		case <-w.catacomb.Dying():
			return empty, w.catacomb.ErrDying()
		case <-watcher.Changes():
		}
		status, err := w.config.Facade.GetMigrationStatus()
		switch {
		case params.IsCodeNotFound(err):
			if err := w.config.Guard.Unlock(); err != nil {
				return empty, errors.Trace(err)
			}
			continue
		case err != nil:
			return empty, errors.Annotate(err, "retrieving migration status")
		}
		if modelHasMigrated(status.Phase) {
			return empty, dependency.ErrUninstall
		}
		if !status.Phase.IsTerminal() {
			return status, nil
		}
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

func modelHasMigrated(phase migration.Phase) bool {
	return phase == migration.DONE || phase == migration.REAPFAILED
}
