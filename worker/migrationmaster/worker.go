// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/migrationtarget"
	"github.com/juju/juju/apiserver/params"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/fortress"
)

var (
	// ErrInactive is returned when the migration is no longer active
	// (probably aborted). In this case the migrationmaster should be
	// restarted so that it can wait for the next migration attempt.
	ErrInactive = errors.New("migration is no longer active")

	// ErrMigrated is returned when the model has migrated to another
	// server. The migrationmaster should not be restarted again in
	// this case.
	ErrMigrated = errors.New("model has migrated")
)

const (
	// maxMinionWait is the maximum time that the migrationmaster will
	// wait for minions to report back regarding a given migration
	// phase.
	maxMinionWait = 15 * time.Minute

	// minionWaitLogInterval is the time between progress update
	// messages, while the migrationmaster is waiting for reports from
	// minions.
	minionWaitLogInterval = 30 * time.Second
)

// Facade exposes controller functionality to a Worker.
type Facade interface {
	// Watch returns a watcher which reports when a migration is
	// active for the model associated with the API connection.
	Watch() (watcher.NotifyWatcher, error)

	// MigrationStatus returns the details and progress of the latest
	// model migration.
	MigrationStatus() (coremigration.MigrationStatus, error)

	// SetPhase updates the phase of the currently active model
	// migration.
	SetPhase(coremigration.Phase) error

	// SetStatusMessage sets a human readable message regarding the
	// progress of a migration.
	SetStatusMessage(string) error

	// Prechecks performs pre-migration checks on the model and
	// (source) controller.
	Prechecks() error

	// ModelInfo return basic information about the model to migrated.
	ModelInfo() (coremigration.ModelInfo, error)

	// Export returns a serialized representation of the model
	// associated with the API connection.
	Export() (coremigration.SerializedModel, error)

	// Reap removes all documents of the model associated with the API
	// connection.
	Reap() error

	// WatchMinionReports returns a watcher which reports when a migration
	// minion has made a report for the current migration phase.
	WatchMinionReports() (watcher.NotifyWatcher, error)

	// MinionReports returns details of the reports made by migration
	// minions to the controller for the current migration phase.
	MinionReports() (coremigration.MinionReports, error)
}

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID       string
	Facade          Facade
	Guard           fortress.Guard
	APIOpen         func(*api.Info, api.DialOpts) (api.Connection, error)
	UploadBinaries  func(migration.UploadBinariesConfig) error
	CharmDownloader migration.CharmDownloader
	ToolsDownloader migration.ToolsDownloader
	Clock           clock.Clock
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Guard == nil {
		return errors.NotValidf("nil Guard")
	}
	if config.APIOpen == nil {
		return errors.NotValidf("nil APIOpen")
	}
	if config.UploadBinaries == nil {
		return errors.NotValidf("nil UploadBinaries")
	}
	if config.CharmDownloader == nil {
		return errors.NotValidf("nil CharmDownloader")
	}
	if config.ToolsDownloader == nil {
		return errors.NotValidf("nil ToolsDownloader")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// New returns a Worker backed by config, or an error.
func New(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Soon we will get model specific logs generated in the
	// controller logged against the model. Until then, distinguish
	// the logs from different migrationmaster insteads using the
	// model UUID suffix.
	loggerName := "juju.worker.migrationmaster:" + config.ModelUUID[len(config.ModelUUID)-6:]
	logger := loggo.GetLogger(loggerName)

	w := &Worker{
		config: config,
		logger: logger,
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
	logger   loggo.Logger
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

	phase := status.Phase

	for {
		var err error
		switch phase {
		case coremigration.QUIESCE:
			phase, err = w.doQUIESCE(status)
		case coremigration.IMPORT:
			phase, err = w.doIMPORT(status.TargetInfo, status.ModelUUID)
		case coremigration.VALIDATION:
			phase, err = w.doVALIDATION(status)
		case coremigration.SUCCESS:
			phase, err = w.doSUCCESS(status)
		case coremigration.LOGTRANSFER:
			phase, err = w.doLOGTRANSFER()
		case coremigration.REAP:
			phase, err = w.doREAP()
		case coremigration.ABORT:
			phase, err = w.doABORT(status.TargetInfo, status.ModelUUID)
		default:
			return errors.Errorf("unknown phase: %v [%d]", phase.String(), phase)
		}

		if err != nil {
			// A phase handler should only return an error if the
			// migration master should exit. In the face of other
			// errors the handler should log the problem and then
			// return the appropriate error phase to transition to -
			// i.e. ABORT or REAPFAILED)
			return errors.Trace(err)
		}

		if w.killed() {
			return w.catacomb.ErrDying()
		}

		w.logger.Infof("setting migration phase to %s", phase)
		if err := w.config.Facade.SetPhase(phase); err != nil {
			return errors.Annotate(err, "failed to set phase")
		}
		status.Phase = phase

		if modelHasMigrated(phase) {
			return ErrMigrated
		} else if phase.IsTerminal() {
			// Some other terminal phase (aborted), exit and try
			// again.
			return ErrInactive
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

func (w *Worker) setInfoStatus(s string, a ...interface{}) {
	w.setStatusAndLog(w.logger.Infof, s, a...)
}

func (w *Worker) setWarningStatus(s string, a ...interface{}) {
	w.setStatusAndLog(w.logger.Warningf, s, a...)
}

func (w *Worker) setErrorStatus(s string, a ...interface{}) {
	w.setStatusAndLog(w.logger.Errorf, s, a...)
}

func (w *Worker) setStatusAndLog(log func(string, ...interface{}), s string, a ...interface{}) {
	message := fmt.Sprintf(s, a...)
	log(message)
	if err := w.setStatus(message); err != nil {
		// Setting status isn't critical. If it fails, just logging
		// the problem here and not passing it upstream makes things a
		// lot clearer in the caller.
		w.logger.Errorf("%s", err)
	}
}

func (w *Worker) setStatus(message string) error {
	err := w.config.Facade.SetStatusMessage(message)
	return errors.Annotate(err, "failed to set status message")
}

func (w *Worker) doQUIESCE(status coremigration.MigrationStatus) (coremigration.Phase, error) {
	err := w.prechecks(status)
	if err != nil {
		w.setErrorStatus(err.Error())
		return coremigration.ABORT, nil
	}

	ok, err := w.waitForMinions(status, failFast, "quiescing")
	if err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}
	if !ok {
		return coremigration.ABORT, nil
	}

	return coremigration.IMPORT, nil
}

func (w *Worker) prechecks(status coremigration.MigrationStatus) error {
	w.setInfoStatus("performing source prechecks")
	err := w.config.Facade.Prechecks()
	if err != nil {
		return errors.Annotate(err, "source prechecks failed")
	}

	w.setInfoStatus("performing target prechecks")
	model, err := w.config.Facade.ModelInfo()
	if err != nil {
		return errors.Annotate(err, "failed to obtain model info during prechecks")
	}
	conn, err := w.openAPIConn(status.TargetInfo)
	if err != nil {
		return errors.Annotate(err, "failed to connect to target controller during prechecks")
	}
	defer conn.Close()
	targetClient := migrationtarget.NewClient(conn)
	err = targetClient.Prechecks(model)
	return errors.Annotate(err, "target prechecks failed")
}

func (w *Worker) doIMPORT(targetInfo coremigration.TargetInfo, modelUUID string) (coremigration.Phase, error) {
	err := w.transferModel(targetInfo, modelUUID)
	if err != nil {
		w.setErrorStatus("model data transfer failed, %v", err)
		return coremigration.ABORT, nil
	}
	return coremigration.VALIDATION, nil
}

func (w *Worker) transferModel(targetInfo coremigration.TargetInfo, modelUUID string) error {
	w.setInfoStatus("exporting model")
	serialized, err := w.config.Facade.Export()
	if err != nil {
		return errors.Annotate(err, "model export failed")
	}

	w.setInfoStatus("importing model into target controller")
	conn, err := w.openAPIConn(targetInfo)
	if err != nil {
		return errors.Annotate(err, "failed to connect to target controller")
	}
	defer conn.Close()
	targetClient := migrationtarget.NewClient(conn)
	err = targetClient.Import(serialized.Bytes)
	if err != nil {
		return errors.Annotate(err, "failed to import model into target controller")
	}

	w.setInfoStatus("uploading model binaries into target controller")
	targetModelConn, err := w.openAPIConnForModel(targetInfo, modelUUID)
	if err != nil {
		return errors.Annotate(err, "failed to open connection to target model")
	}
	defer targetModelConn.Close()
	targetModelClient := targetModelConn.Client()
	err = w.config.UploadBinaries(migration.UploadBinariesConfig{
		Charms:          serialized.Charms,
		CharmDownloader: w.config.CharmDownloader,
		CharmUploader:   targetModelClient,
		Tools:           serialized.Tools,
		ToolsDownloader: w.config.ToolsDownloader,
		ToolsUploader:   targetModelClient,
	})
	return errors.Annotate(err, "failed migration binaries")
}

func (w *Worker) doVALIDATION(status coremigration.MigrationStatus) (coremigration.Phase, error) {
	// Wait for agents to complete their validation checks.
	ok, err := w.waitForMinions(status, failFast, "validating")
	if err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}
	if !ok {
		return coremigration.ABORT, nil
	}

	// Once all agents have validated, activate the model in the
	// target controller.
	err = w.activateModel(status.TargetInfo, status.ModelUUID)
	if err != nil {
		w.setErrorStatus("model activation failed, %v", err)
		return coremigration.ABORT, nil
	}
	return coremigration.SUCCESS, nil
}

func (w *Worker) activateModel(targetInfo coremigration.TargetInfo, modelUUID string) error {
	w.setInfoStatus("activating model in target controller")
	conn, err := w.openAPIConn(targetInfo)
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	targetClient := migrationtarget.NewClient(conn)
	err = targetClient.Activate(modelUUID)
	return errors.Trace(err)
}

func (w *Worker) doSUCCESS(status coremigration.MigrationStatus) (coremigration.Phase, error) {
	_, err := w.waitForMinions(status, waitForAll, "successful")
	if err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}
	// There's no turning back from SUCCESS - any problems should have
	// been picked up in VALIDATION. After the minion wait in the
	// SUCCESS phase, the migration can only proceed to LOGTRANSFER.
	return coremigration.LOGTRANSFER, nil
}

func (w *Worker) doLOGTRANSFER() (coremigration.Phase, error) {
	// TODO(mjs) - To be implemented.
	// w.setInfoStatus("successful: transferring logs to target controller")
	return coremigration.REAP, nil
}

func (w *Worker) doREAP() (coremigration.Phase, error) {
	w.setInfoStatus("successful, removing model from source controller")
	err := w.config.Facade.Reap()
	if err != nil {
		return coremigration.REAPFAILED, errors.Trace(err)
	}
	return coremigration.DONE, nil
}

func (w *Worker) doABORT(targetInfo coremigration.TargetInfo, modelUUID string) (coremigration.Phase, error) {
	w.setInfoStatus("aborted, removing model from target controller")
	if err := w.removeImportedModel(targetInfo, modelUUID); err != nil {
		// This isn't fatal. Removing the imported model is a best
		// efforts attempt so just report the error and proceed.
		w.setWarningStatus("failed to remove model from target controller, %v", err)
	}
	return coremigration.ABORTDONE, nil
}

func (w *Worker) removeImportedModel(targetInfo coremigration.TargetInfo, modelUUID string) error {
	conn, err := w.openAPIConn(targetInfo)
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	targetClient := migrationtarget.NewClient(conn)
	err = targetClient.Abort(modelUUID)
	return errors.Trace(err)
}

func (w *Worker) waitForActiveMigration() (coremigration.MigrationStatus, error) {
	var empty coremigration.MigrationStatus

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

		status, err := w.config.Facade.MigrationStatus()
		switch {
		case params.IsCodeNotFound(err):
			// There's never been a migration.
		case err == nil && status.Phase.IsTerminal():
			// No migration in progress.
			if modelHasMigrated(status.Phase) {
				return empty, ErrMigrated
			}
		case err != nil:
			return empty, errors.Annotate(err, "retrieving migration status")
		default:
			// Migration is in progress.
			return status, nil
		}

		// While waiting for a migration, ensure the fortress is open.
		if err := w.config.Guard.Unlock(); err != nil {
			return empty, errors.Trace(err)
		}
	}
}

// Possible values for waitForMinion's waitPolicy argument.
const failFast = false  // Stop waiting at first minion failure report
const waitForAll = true // Wait for all minion reports to arrive (or timeout)

func (w *Worker) waitForMinions(
	status coremigration.MigrationStatus,
	waitPolicy bool,
	infoPrefix string,
) (success bool, err error) {
	clk := w.config.Clock
	maxWait := maxMinionWait - clk.Now().Sub(status.PhaseChangedTime)
	timeout := clk.After(maxWait)

	w.setInfoStatus("%s, waiting for agents to report back", infoPrefix)
	w.logger.Infof("waiting for agents to report back for migration phase %s (will wait up to %s)",
		status.Phase, truncDuration(maxWait))

	watch, err := w.config.Facade.WatchMinionReports()
	if err != nil {
		return false, errors.Trace(err)
	}
	if err := w.catacomb.Add(watch); err != nil {
		return false, errors.Trace(err)
	}

	logProgress := clk.After(minionWaitLogInterval)

	var reports coremigration.MinionReports
	for {
		select {
		case <-w.catacomb.Dying():
			return false, w.catacomb.ErrDying()

		case <-timeout:
			w.logger.Errorf(formatMinionTimeout(reports, status, infoPrefix))
			w.setErrorStatus("%s, timed out waiting for agents to report", infoPrefix)
			return false, nil

		case <-watch.Changes():
			var err error
			reports, err = w.config.Facade.MinionReports()
			if err != nil {
				return false, errors.Trace(err)
			}
			if err := validateMinionReports(reports, status); err != nil {
				return false, errors.Trace(err)
			}
			failures := len(reports.FailedMachines) + len(reports.FailedUnits)
			if failures > 0 {
				w.logger.Errorf(formatMinionFailure(reports, infoPrefix))
				w.setErrorStatus("%s, some agents reported failure", infoPrefix)
				if waitPolicy == failFast {
					return false, nil
				}
			}
			if reports.UnknownCount == 0 {
				msg := formatMinionWaitDone(reports, infoPrefix)
				if failures > 0 {
					w.logger.Errorf(msg)
					w.setErrorStatus("%s, some agents reported failure", infoPrefix)
					return false, nil
				}
				w.logger.Infof(msg)
				w.setInfoStatus("%s, all agents reported success", infoPrefix)
				return true, nil
			}

		case <-logProgress:
			w.setInfoStatus("%s, %s", infoPrefix, formatMinionWaitUpdate(reports))
			logProgress = clk.After(minionWaitLogInterval)
		}
	}
}

func truncDuration(d time.Duration) time.Duration {
	return (d / time.Second) * time.Second
}

func validateMinionReports(reports coremigration.MinionReports, status coremigration.MigrationStatus) error {
	if reports.MigrationId != status.MigrationId {
		return errors.Errorf("unexpected migration id in minion reports, got %v, expected %v",
			reports.MigrationId, status.MigrationId)
	}
	if reports.Phase != status.Phase {
		return errors.Errorf("minion reports phase (%s) does not match migration phase (%s)",
			reports.Phase, status.Phase)
	}
	return nil
}

func formatMinionTimeout(
	reports coremigration.MinionReports,
	status coremigration.MigrationStatus,
	infoPrefix string,
) string {
	if reports.IsZero() {
		return fmt.Sprintf("no agents reported in time")
	}

	var fails []string
	if len(reports.SomeUnknownMachines) > 0 {
		fails = append(fails, fmt.Sprintf("machines: %s", strings.Join(reports.SomeUnknownMachines, ",")))
	}
	if len(reports.SomeUnknownUnits) > 0 {
		fails = append(fails, fmt.Sprintf("units: %s", strings.Join(reports.SomeUnknownUnits, ",")))
	}
	return fmt.Sprintf("%d agents failed to report in time for %q phase (including %s)",
		reports.UnknownCount, infoPrefix, strings.Join(fails, "; "))
}

func formatMinionFailure(reports coremigration.MinionReports, infoPrefix string) string {
	var fails []string
	if len(reports.FailedMachines) > 0 {
		fails = append(fails, fmt.Sprintf("machines: %s", strings.Join(reports.FailedMachines, ",")))
	}
	if len(reports.FailedUnits) > 0 {
		fails = append(fails, fmt.Sprintf("units: %s", strings.Join(reports.FailedUnits, ",")))
	}
	return fmt.Sprintf("agents failed phase %q (%s)", infoPrefix, strings.Join(fails, "; "))
}

func formatMinionWaitDone(reports coremigration.MinionReports, infoPrefix string) string {
	return fmt.Sprintf("completed waiting for agents to report for %q, %d succeeded, %d failed",
		infoPrefix, reports.SuccessCount, len(reports.FailedMachines)+len(reports.FailedUnits))
}

func formatMinionWaitUpdate(reports coremigration.MinionReports) string {
	if reports.IsZero() {
		return fmt.Sprintf("no reports from agents yet")
	}

	msg := fmt.Sprintf("waiting for agents to report back: %d succeeded, %d still to report",
		reports.SuccessCount, reports.UnknownCount)
	failed := len(reports.FailedMachines) + len(reports.FailedUnits)
	if failed > 0 {
		msg += fmt.Sprintf(", %d failed", failed)
	}
	return msg
}

func (w *Worker) openAPIConn(targetInfo coremigration.TargetInfo) (api.Connection, error) {
	return w.openAPIConnForModel(targetInfo, "")
}

func (w *Worker) openAPIConnForModel(targetInfo coremigration.TargetInfo, modelUUID string) (api.Connection, error) {
	apiInfo := &api.Info{
		Addrs:    targetInfo.Addrs,
		CACert:   targetInfo.CACert,
		Tag:      targetInfo.AuthTag,
		Password: targetInfo.Password,
		ModelTag: names.NewModelTag(modelUUID),
	}
	if targetInfo.Macaroon != nil {
		apiInfo.Macaroons = []macaroon.Slice{{targetInfo.Macaroon}}
	}

	// Use zero DialOpts (no retries) because the worker must stay
	// responsive to Kill requests. We don't want it to be blocked by
	// a long set of retry attempts.
	return w.config.APIOpen(apiInfo, api.DialOpts{})
}

func modelHasMigrated(phase coremigration.Phase) bool {
	return phase == coremigration.DONE || phase == coremigration.REAPFAILED
}
