// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5/catacomb"
	"github.com/kr/pretty"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/controller/migrationtarget"
	"github.com/juju/juju/controller"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	coremigration "github.com/juju/juju/core/migration"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application/charm"
	domainexport "github.com/juju/juju/domain/export"
	"github.com/juju/juju/domain/modelmigration"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/wrench"
	"github.com/juju/juju/rpc/params"
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

	// utcZero matches the deserialised zero times coming back from
	// MigrationTarget.LatestLogTime, because they have a non-nil
	// location.
	utcZero = time.Time{}.In(time.UTC)
)

// progressUpdateInterval is the time between progress update
// messages. It's used while the migrationmaster is waiting for
// reports from minions and while it's transferring log messages
// to the newly-migrated model.
const progressUpdateInterval = 30 * time.Second

type CharmService interface {
	// GetCharmArchive returns a ReadCloser stream for the charm archive for a given
	// charm id, along with the hash of the charm archive. Clients can use the hash
	// to verify the integrity of the charm archive.
	GetCharmArchive(context.Context, charm.CharmLocator) (io.ReadCloser, string, error)

	// ListCharmLocators returns a list of charm locators. The locator allows
	// the charm URL to be reconstructed. If no names are provided, all the
	// model's charms are listed.
	ListCharmLocators(context.Context, ...string) ([]charm.CharmLocator, error)
}

// ModelMigrationService exposes the model migration domain operations the
// worker uses to drive the source side of a migration: the export lifecycle,
// phase and status reporting, and minion reports.
type ModelMigrationService interface {
	// WatchForMigration returns a notification watcher that fires when this
	// model starts or stops undergoing migration.
	WatchForMigration(context.Context) (watcher.NotifyWatcher, error)

	// Migration returns status about migration of this model. If the model is
	// not currently being migrated, a migration with phase
	// [coremigration.NONE] is returned.
	Migration(context.Context) (modelmigration.Migration, error)

	// SetMigrationPhase progresses the active migration to the given phase.
	SetMigrationPhase(context.Context, coremigration.Phase) error

	// SetMigrationStatusMessage records a human readable message regarding
	// the progress of the active migration.
	SetMigrationStatusMessage(context.Context, string) error

	// MarkModelAsGone removes the migrated model from this controller once
	// the target controller owns it, completing the export migration.
	MarkModelAsGone(context.Context) error

	// WatchMinionReports returns a notification watcher that fires when any
	// minion reports an update to their phase.
	WatchMinionReports(context.Context) (watcher.NotifyWatcher, error)

	// MinionReports returns phase information about minions in this model.
	MinionReports(context.Context) (coremigration.MinionReports, error)

	// SourceControllerInfo returns this (source) controller's identity and
	// client connection details used by the target controller to dial back
	// during model activation.
	SourceControllerInfo(context.Context) (coremigration.SourceControllerInfo, error)
}

// ExportService exposes the model-database export used as the envelope
// payload.
type ExportService interface {
	// Export returns the model-database contents at the latest supported
	// payload schema version.
	Export(context.Context) (*domainexport.ModelExport, error)

	// GetControllerModelInfo reads the controller-database information scoped
	// to this model in target-portable semantic form.
	GetControllerModelInfo(context.Context) (coremodelmigration.ControllerModelInfo, error)
}

// ControllerConfigService provides access to the controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelAgentService reports the agent binaries in use by the model's agents.
type ModelAgentService interface {
	// GetModelAgentBinaryMetadata reports the agent binary metadata that each
	// machine and unit in the model is running.
	GetModelAgentBinaryMetadata(
		context.Context,
	) (map[machine.Name]coreagentbinary.Metadata, map[unit.Name]coreagentbinary.Metadata, error)
}

// ResourceService lists the model resources that need binary transfer
// and opens resource content for download.
type ResourceService interface {
	// ListAllModelResources returns the application and unit resources to
	// export for all applications in the model.
	ListAllModelResources(context.Context) ([]resource.Resource, error)

	// GetResourceUUIDByApplicationAndResourceName returns the UUID of the
	// resource identified by application and resource name.
	GetResourceUUIDByApplicationAndResourceName(ctx context.Context, appName, resName string) (resource.UUID, error)

	// OpenResource returns the details of and a reader for the resource.
	OpenResource(ctx context.Context, resourceUUID resource.UUID) (resource.Resource, io.ReadCloser, error)
}

// AgentBinaryStore provides an interface for interacting with the stored agent
// binaries within a controller and model.
type AgentBinaryStore interface {
	// GetAgentBinaryForSHA256 returns the agent binary associated with the
	// given SHA256 sum. The following errors can be expected:
	// - [github.com/juju/juju/domain/agentbinary/errors.NotFound] when no agent
	// binaries exist for the provided sha.
	GetAgentBinaryUsingSHA256(context.Context, string) (io.ReadCloser, int64, error)
}

// LoggingService provides access to the controller-wide logging configuration
// used to determine whether Loki forwarding is enabled.
type LoggingService interface {
	// IsLokiEnabled returns true if a Loki config exists with a non-empty
	// endpoint.
	IsLokiEnabled(context.Context) (bool, error)
}

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID               string
	CharmService            CharmService
	ModelMigrationService   ModelMigrationService
	ExportService           ExportService
	ControllerConfigService ControllerConfigService
	ModelAgentService       ModelAgentService
	ResourceService         ResourceService
	Guard                   fortress.Guard
	APIOpen                 func(context.Context, *api.Info, api.DialOpts) (api.Connection, error)
	UploadBinaries          func(context.Context, migration.UploadBinariesConfig, logger.Logger) error
	AgentBinaryStore        AgentBinaryStore
	LoggingService          LoggingService
	Clock                   clock.Clock

	// SourcePrecheck runs source-side migration prechecks using local
	// domain services instead of an API facade.
	SourcePrecheck func(context.Context) error

	// StreamModelLog returns a channel that yields log messages on or
	// after the given time from the local logsink.log file.
	StreamModelLog func(context.Context, time.Time) (<-chan common.LogMessage, error)
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if !names.IsValidModel(config.ModelUUID) {
		return errors.NotValidf("model UUID %q", config.ModelUUID)
	}
	if config.CharmService == nil {
		return errors.NotValidf("nil CharmService")
	}
	if config.ModelMigrationService == nil {
		return errors.NotValidf("nil ModelMigrationService")
	}
	if config.ExportService == nil {
		return errors.NotValidf("nil ExportService")
	}
	if config.ControllerConfigService == nil {
		return errors.NotValidf("nil ControllerConfigService")
	}
	if config.ModelAgentService == nil {
		return errors.NotValidf("nil ModelAgentService")
	}
	if config.ResourceService == nil {
		return errors.NotValidf("nil ResourceService")
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
	if config.AgentBinaryStore == nil {
		return errors.NotValidf("nil AgentBinaryStore")
	}
	if config.LoggingService == nil {
		return errors.NotValidf("nil LoggingService")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.SourcePrecheck == nil {
		return errors.NotValidf("nil SourcePrecheck")
	}
	if config.StreamModelLog == nil {
		return errors.NotValidf("nil StreamModelLog")
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
	// the logs from different migrationmaster insteads using the short
	// model UUID suffix.
	loggerName := "juju.worker.migrationmaster." + names.NewModelTag(config.ModelUUID).ShortId()
	logger := internallogger.GetLogger(loggerName, logger.MIGRATION)

	w := &Worker{
		config: config,
		logger: logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "migration-master",
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
	catacomb            catacomb.Catacomb
	config              Config
	logger              logger.Logger
	lastFailure         string
	minionReportTimeout time.Duration
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
	ctx, cancel := w.scopedContext()
	defer cancel()

	status, err := w.waitForActiveMigration(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	err = w.config.Guard.Lockdown(ctx)
	if errors.Cause(err) == fortress.ErrAborted {
		return w.catacomb.ErrDying()
	} else if err != nil {
		return errors.Trace(err)
	}

	controllerConfig, err := w.config.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Annotate(err, "retrieving controller config")
	}
	w.minionReportTimeout = controllerConfig.MigrationMinionWaitMax()

	phase := status.Phase

	for {
		var err error
		switch phase {
		case coremigration.QUIESCE:
			phase, err = w.doQUIESCE(ctx, status)
		case coremigration.IMPORT:
			phase, err = w.doIMPORT(ctx, status)
		case coremigration.VALIDATION:
			phase, err = w.doVALIDATION(ctx, status)
		case coremigration.SUCCESS:
			phase, err = w.doSUCCESS(ctx, status)
		case coremigration.LOGTRANSFER:
			phase, err = w.doLOGTRANSFER(ctx, status.TargetInfo, status.ModelUUID, status.MigrationId)
		case coremigration.REAP:
			phase, err = w.doREAP(ctx)
		case coremigration.ABORT:
			phase, err = w.doABORT(ctx, status.TargetInfo, status.ModelUUID)
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

		if phase == coremigration.DONE {
			// MarkModelAsGone has already moved the export to DONE;
			// setting the phase again would fail as the export is no
			// longer active.
			return ErrMigrated
		}

		w.logger.Infof(ctx, "setting migration phase to %s", phase)
		if err := w.config.ModelMigrationService.SetMigrationPhase(ctx, phase); err != nil {
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

func (w *Worker) setInfoStatus(ctx context.Context, s string, a ...any) {
	w.setStatusAndLog(ctx, w.logger.Infof, s, a...)
}

func (w *Worker) setErrorStatus(ctx context.Context, s string, a ...any) {
	w.lastFailure = fmt.Sprintf(s, a...)
	w.setStatusAndLog(ctx, w.logger.Errorf, s, a...)
}

func (w *Worker) setStatusAndLog(ctx context.Context, log func(context.Context, string, ...any), s string, a ...any) {
	message := fmt.Sprintf(s, a...)
	log(ctx, message)
	if err := w.setStatus(ctx, message); err != nil {
		// Setting status isn't critical. If it fails, just logging
		// the problem here and not passing it upstream makes things a
		// lot clearer in the caller.
		w.logger.Errorf(ctx, "%s", err)
	}
}

func (w *Worker) setStatus(ctx context.Context, message string) error {
	err := w.config.ModelMigrationService.SetMigrationStatusMessage(ctx, message)
	return errors.Annotate(err, "failed to set status message")
}

func (w *Worker) doQUIESCE(ctx context.Context, status coremigration.MigrationStatus) (coremigration.Phase, error) {
	// Run prechecks before waiting for minions to report back. This
	// short-circuits the long timeout in the case of an agent being
	// down.
	if err := w.prechecks(ctx, status); err != nil {
		w.setErrorStatus(ctx, "%s", err.Error())
		return coremigration.ABORT, nil
	}

	ok, err := w.waitForMinions(ctx, status, failFast, "quiescing")
	if err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}
	if !ok {
		return coremigration.ABORT, nil
	}

	// Now that the model is stable, run the prechecks again.
	if err := w.prechecks(ctx, status); err != nil {
		w.setErrorStatus(ctx, "%s", err.Error())
		return coremigration.ABORT, nil
	}

	return coremigration.IMPORT, nil
}

// checkTargetSupportsEnvelope hard-errors when the target controller does not
// advertise the migrationtarget facade version able to accept the
// SerializedModelV2 envelope. There is no fallback to a legacy path. The guard
// is shared with the synchronous InitiateMigration precheck so both fail with
// the same message.
func checkTargetSupportsEnvelope(targetClient *migrationtarget.Client) error {
	return migration.CheckTargetSupportsEnvelope(targetClient.BestFacadeVersion())
}

func (w *Worker) prechecks(ctx context.Context, status coremigration.MigrationStatus) error {
	w.setInfoStatus(ctx, "performing source prechecks")
	if err := w.config.SourcePrecheck(ctx); err != nil {
		return errors.Annotate(err, "source prechecks failed")
	}

	w.setInfoStatus(ctx, "performing target prechecks")
	model, err := w.assembleEnvelope(ctx, status.MigrationId)
	if err != nil {
		return errors.Annotate(err, "failed to assemble model envelope during prechecks")
	}
	targetConn, err := w.openAPIConn(ctx, status.TargetInfo)
	if err != nil {
		return errors.Annotate(err, "failed to connect to target controller during prechecks")
	}
	defer targetConn.Close()
	if targetConn.ControllerTag() != names.NewControllerTag(status.TargetInfo.ControllerUUID) {
		return errors.Errorf("unexpected target controller UUID (got %s, expected %s)",
			targetConn.ControllerTag().Id(), status.TargetInfo.ControllerUUID)
	}
	targetClient := migrationtarget.NewClient(targetConn)
	if err := checkTargetSupportsEnvelope(targetClient); err != nil {
		return errors.Trace(err)
	}
	err = targetClient.PrechecksV2(ctx, model.envelope)
	return errors.Annotate(err, "target prechecks failed")
}

func (w *Worker) doIMPORT(ctx context.Context, status coremigration.MigrationStatus) (coremigration.Phase, error) {
	phase, err := w.transferModel(ctx, status)
	if err != nil {
		w.setErrorStatus(ctx, "model data transfer failed, %v", err)
		return coremigration.ABORT, nil
	}
	return phase, nil
}

type uploadWrapper struct {
	client    *migrationtarget.Client
	modelUUID string
}

// UploadTools prepends the model UUID to the args passed to the migration client.
func (w *uploadWrapper) UploadTools(ctx context.Context, r io.Reader, vers semversion.Binary) (tools.List, error) {
	return w.client.UploadTools(ctx, w.modelUUID, r, vers)
}

// UploadCharm prepends the model UUID to the args passed to the migration client.
func (w *uploadWrapper) UploadCharm(ctx context.Context, curl string, charmRef string, content io.Reader) (string, error) {
	return w.client.UploadCharm(ctx, w.modelUUID, curl, charmRef, content)
}

// UploadResource prepends the model UUID to the args passed to the migration client.
func (w *uploadWrapper) UploadResource(ctx context.Context, res resource.Resource, content io.Reader) error {
	return w.client.UploadResource(ctx, w.modelUUID, res, content)
}

func importErrIsActivating(err error) bool {
	// TODO(modelmigration): remove this string match when target-side Import v8
	// is implemented. Import v8 needs to return structured duplicate-import
	// state so the source can tell whether the target is still safely
	// importing, or has crossed into activation where aborting is no longer
	// correct. Until then, keep the existing resume path for the placeholder
	// activation wording and do not add a public RPC error code.
	return err != nil && strings.Contains(err.Error(), "activation in progress")
}

// transferModel assembles the authoritative import envelope, imports it into
// the target controller, and uploads the model binaries. It returns the phase
// the migration should move to next; any error moves the migration to ABORT.
func (w *Worker) transferModel(ctx context.Context, status coremigration.MigrationStatus) (coremigration.Phase, error) {
	w.setInfoStatus(ctx, "exporting model")
	model, err := w.assembleEnvelope(ctx, status.MigrationId)
	if err != nil {
		return coremigration.UNKNOWN, errors.Annotate(err, "model export failed")
	}

	w.setInfoStatus(ctx, "importing model into target controller")
	conn, err := w.openAPIConn(ctx, status.TargetInfo)
	if err != nil {
		return coremigration.UNKNOWN, errors.Annotate(err, "failed to connect to target controller")
	}
	defer conn.Close()
	targetClient := migrationtarget.NewClient(conn)
	if err := checkTargetSupportsEnvelope(targetClient); err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}

	err = targetClient.ImportV2(ctx, model.envelope)
	switch {
	case params.IsCodeAlreadyExists(err):
		if importErrIsActivating(err) {
			// A previous import of this model has crossed the activation
			// point of no return on the target. Skip the binary uploads
			// (they completed before activation started) and continue to
			// VALIDATION so activation is retried and completed.
			w.setInfoStatus(ctx, "target reports model activation in progress, continuing to validation")
			return coremigration.VALIDATION, nil
		}
		return coremigration.UNKNOWN, errors.New("target controller already has an import for this model")
	case err != nil:
		return coremigration.UNKNOWN, errors.Annotate(err, "failed to import model into target controller")
	}

	if wrench.IsActive("migrationmaster", "die-in-export") {
		// Simulate a abort causing failure to test last status not over written.
		return coremigration.UNKNOWN, errors.New("wrench in the transferModel works")
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	w.setInfoStatus(ctx, "uploading model binaries into target controller")
	wrapper := &uploadWrapper{targetClient, status.ModelUUID}
	err = w.config.UploadBinaries(ctx, migration.UploadBinariesConfig{
		Charms:        model.charms,
		CharmService:  w.config.CharmService,
		CharmUploader: wrapper,

		Tools:            model.tools,
		AgentBinaryStore: w.config.AgentBinaryStore,
		ToolsUploader:    wrapper,

		Resources:          model.resources,
		ResourceDownloader: &resourceDownloaderShim{svc: w.config.ResourceService},
		ResourceUploader:   wrapper,
	}, w.logger)
	if err != nil {
		return coremigration.UNKNOWN, errors.Annotate(err, "failed to migrate binaries")
	}
	return coremigration.VALIDATION, nil
}

func (w *Worker) doVALIDATION(ctx context.Context, status coremigration.MigrationStatus) (coremigration.Phase, error) {
	// Wait for agents to complete their validation checks.
	ok, err := w.waitForMinions(ctx, status, failFast, "validating")
	if err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}
	if !ok {
		return coremigration.ABORT, nil
	}

	client, closer, err := w.openTargetAPI(ctx, status.TargetInfo)
	if err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}
	defer func() { _ = closer() }()

	// Check that the provider and target controller agree about what
	// machines belong to the migrated model.
	ok, err = w.checkTargetMachines(ctx, client, status.ModelUUID)
	if err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}
	if !ok {
		return coremigration.ABORT, nil
	}

	// Once all agents have validated, activate the model in the
	// target controller.
	err = w.activateModel(ctx, client, status.ModelUUID)
	if err != nil {
		w.setErrorStatus(ctx, "model activation failed, %v", err)
		return coremigration.ABORT, nil
	}
	return coremigration.SUCCESS, nil
}

func (w *Worker) checkTargetMachines(ctx context.Context, targetClient *migrationtarget.Client, modelUUID string) (bool, error) {
	w.setInfoStatus(ctx, "checking machines in migrated model")
	results, err := targetClient.CheckMachines(ctx, modelUUID)
	if err != nil {
		return false, errors.Trace(err)
	}
	if len(results) > 0 {
		for _, resultErr := range results {
			w.logger.Errorf(ctx, resultErr.Error())
		}
		plural := "s"
		if len(results) == 1 {
			plural = ""
		}
		w.setErrorStatus(ctx, "machine sanity check failed, %d error%s found", len(results), plural)
		return false, nil
	}
	return true, nil
}

func (w *Worker) activateModel(ctx context.Context, targetClient *migrationtarget.Client, modelUUID string) error {
	w.setInfoStatus(ctx, "activating model in target controller")
	info, err := w.config.ModelMigrationService.SourceControllerInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(targetClient.Activate(ctx, modelUUID, info, nil))
}

func (w *Worker) doSUCCESS(ctx context.Context, status coremigration.MigrationStatus) (coremigration.Phase, error) {
	_, err := w.waitForMinions(ctx, status, waitForAll, "successful")
	if err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}
	err = w.transferResources(ctx, status.TargetInfo, status.ModelUUID)
	if err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}
	// There's no turning back from SUCCESS - any problems should have
	// been picked up in VALIDATION. After the minion wait in the
	// SUCCESS phase, the migration can only proceed to LOGTRANSFER.
	return coremigration.LOGTRANSFER, nil
}

func (w *Worker) transferResources(ctx context.Context, targetInfo coremigration.TargetInfo, modelUUID string) error {
	w.setInfoStatus(ctx, "transferring ownership of cloud resources to target controller")
	conn, err := w.openAPIConn(ctx, targetInfo)
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	targetClient := migrationtarget.NewClient(conn)
	err = targetClient.AdoptResources(ctx, modelUUID)
	return errors.Trace(err)
}

func (w *Worker) doLOGTRANSFER(ctx context.Context, targetInfo coremigration.TargetInfo, modelUUID, migrationID string) (coremigration.Phase, error) {
	lokiEnabled, err := w.config.LoggingService.IsLokiEnabled(ctx)
	if err != nil {
		return coremigration.UNKNOWN, errors.Annotate(err, "checking Loki config for log transfer")
	}
	if lokiEnabled {
		w.logger.Infof(ctx, "Loki forwarding enabled on source controller - skipping log transfer for model %s (migration %s)", modelUUID, migrationID)
		w.emitCutoverMarker(ctx, modelUUID, migrationID)
		return coremigration.REAP, nil
	}
	err = w.transferLogs(ctx, targetInfo, modelUUID)
	if err != nil {
		return coremigration.UNKNOWN, errors.Trace(err)
	}
	return coremigration.REAP, nil
}

// emitCutoverMarker logs a cutover marker record at the point where log
// transfer is skipped because Loki forwarding is enabled. The marker
// includes the model UUID, migration identifier, and cutover timestamp
// so operators can correlate logs across the source and target Loki
// stores.
func (w *Worker) emitCutoverMarker(ctx context.Context, modelUUID, migrationID string) {
	w.logger.Infof(ctx,
		"MIGRATION CUTOVER: model=%s migration=%s t_cutover=%s",
		modelUUID, migrationID, w.config.Clock.Now().UTC().Format(time.RFC3339Nano),
	)
}

func (w *Worker) transferLogs(ctx context.Context, targetInfo coremigration.TargetInfo, modelUUID string) error {
	sent := 0
	reportProgress := func(finished bool, sent int) {
		verb := "transferring"
		if finished {
			verb = "transferred"
		}
		w.setInfoStatus(ctx, "successful, %s logs to target controller (%d sent)", verb, sent)
	}
	reportProgress(false, sent)

	conn, err := w.openAPIConn(ctx, targetInfo)
	if err != nil {
		return errors.Annotate(err, "connecting to target API")
	}
	defer conn.Close()

	targetClient := migrationtarget.NewClient(conn)
	latestLogTime, err := targetClient.LatestLogTime(ctx, modelUUID)
	if err != nil {
		return errors.Annotate(err, "getting log start time")
	}

	if latestLogTime != utcZero {
		w.logger.Debugf(ctx, "log transfer was interrupted - restarting from %s", latestLogTime)
	}

	throwWrench := latestLogTime == utcZero && wrench.IsActive("migrationmaster", "die-after-500-log-messages")

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	logSource, err := w.config.StreamModelLog(ctx, latestLogTime)
	if err != nil {
		return errors.Annotate(err, "opening source log stream")
	}

	// TODO(debug-log) - delete old model logs off the source controller after migration
	logTarget, err := targetClient.OpenLogTransferStream(ctx, modelUUID)
	if err != nil {
		return errors.Annotate(err, "opening target log stream")
	}
	defer logTarget.Close()

	clk := w.config.Clock
	logProgress := clk.After(progressUpdateInterval)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case msg, ok := <-logSource:
			if !ok {
				// The channel's been closed, we're finished!
				reportProgress(true, sent)
				return nil
			}
			err := logTarget.WriteJSON(params.LogRecord{
				Entity:   msg.Entity,
				Time:     msg.Timestamp,
				Module:   msg.Module,
				Location: msg.Location,
				Level:    msg.Severity,
				Message:  msg.Message,
				Labels:   msg.Labels,
			})
			if err != nil {
				return errors.Trace(err)
			}
			sent++

			if throwWrench && sent == 500 {
				// Simulate a connection drop to test restartability.
				return errors.New("wrench in the works")
			}
		case <-logProgress:
			reportProgress(false, sent)
			logProgress = clk.After(progressUpdateInterval)
		}
	}
}

func (w *Worker) doREAP(ctx context.Context) (coremigration.Phase, error) {
	w.setInfoStatus(ctx, "successful, removing model from source controller")
	// NOTE(babbageclunk): MarkModelAsGone sets the migration phase to
	// DONE if successful - this avoids a race where this worker is
	// killed by the model going away before it can update the phase
	// itself.
	err := w.config.ModelMigrationService.MarkModelAsGone(ctx)
	if err != nil {
		w.setErrorStatus(ctx, "removing exported model failed: %s", err.Error())
		return coremigration.REAPFAILED, nil
	}
	return coremigration.DONE, nil
}

func (w *Worker) doABORT(ctx context.Context, targetInfo coremigration.TargetInfo, modelUUID string) (coremigration.Phase, error) {
	w.setInfoStatus(ctx, "aborted, removing model from target controller: %s", w.lastFailure)
	if err := w.removeImportedModel(ctx, targetInfo, modelUUID); err != nil {
		// This isn't fatal. Removing the imported model is a best
		// efforts attempt so just report the error and proceed.
		w.logger.Warningf(ctx, "failed to remove model from target controller, %v", err)
	}
	return coremigration.ABORTDONE, nil
}

func (w *Worker) removeImportedModel(ctx context.Context, targetInfo coremigration.TargetInfo, modelUUID string) error {
	conn, err := w.openAPIConn(ctx, targetInfo)
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	targetClient := migrationtarget.NewClient(conn)
	err = targetClient.Abort(ctx, modelUUID)
	return errors.Trace(err)
}

func (w *Worker) waitForActiveMigration(ctx context.Context) (coremigration.MigrationStatus, error) {
	var empty coremigration.MigrationStatus

	watcher, err := w.config.ModelMigrationService.WatchForMigration(ctx)
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

		mig, err := w.config.ModelMigrationService.Migration(ctx)
		switch {
		case err != nil:
			return empty, errors.Annotate(err, "retrieving migration status")
		case mig.Phase == coremigration.NONE:
			// There's never been a migration.
		case mig.Phase.IsTerminal():
			// No migration in progress.
			if modelHasMigrated(mig.Phase) {
				return empty, ErrMigrated
			}
		default:
			// Migration is in progress.
			return coremigration.MigrationStatus{
				MigrationId:      mig.UUID,
				ModelUUID:        w.config.ModelUUID,
				Phase:            mig.Phase,
				PhaseChangedTime: mig.PhaseChangedTime,
				TargetInfo:       mig.Target,
			}, nil
		}

		// While waiting for a migration, ensure the fortress is open.
		if err := w.config.Guard.Unlock(ctx); err != nil {
			return empty, errors.Trace(err)
		}
	}
}

// Possible values for waitForMinion's waitPolicy argument.
const failFast = false  // Stop waiting at first minion failure report
const waitForAll = true // Wait for all minion reports to arrive (or timeout)

func (w *Worker) waitForMinions(
	ctx context.Context,
	status coremigration.MigrationStatus,
	waitPolicy bool,
	infoPrefix string,
) (success bool, err error) {
	clk := w.config.Clock
	timeout := clk.After(w.minionReportTimeout)

	w.setInfoStatus(ctx, "%s, waiting for agents to report back", infoPrefix)
	w.logger.Infof(ctx, "waiting for agents to report back for migration phase %s (will wait up to %s)",
		status.Phase, truncDuration(w.minionReportTimeout))

	watch, err := w.config.ModelMigrationService.WatchMinionReports(ctx)
	if err != nil {
		return false, errors.Trace(err)
	}
	if err := w.catacomb.Add(watch); err != nil {
		return false, errors.Trace(err)
	}

	logProgress := clk.After(progressUpdateInterval)

	var reports coremigration.MinionReports
	for {
		select {
		case <-w.catacomb.Dying():
			return false, w.catacomb.ErrDying()

		case <-timeout:
			w.logger.Errorf(ctx, formatMinionTimeout(reports, status, infoPrefix))
			w.setErrorStatus(ctx, "%s, timed out waiting for agents to report", infoPrefix)
			return false, nil

		case <-watch.Changes():
			var err error
			reports, err = w.config.ModelMigrationService.MinionReports(ctx)
			if err != nil {
				return false, errors.Trace(err)
			}
			if err := validateMinionReports(reports, status); err != nil {
				return false, errors.Trace(err)
			}
			w.logger.Debugf(ctx, "migration minion reports:\n%s", pretty.Sprint(reports))
			failures := len(reports.FailedMachines) + len(reports.FailedUnits) + len(reports.FailedApplications)
			if failures > 0 {
				w.logger.Errorf(ctx, formatMinionFailure(reports, infoPrefix))
				w.setErrorStatus(ctx, "%s, some agents reported failure", infoPrefix)
				if waitPolicy == failFast {
					return false, nil
				}
			}
			if reports.UnknownCount == 0 {
				msg := formatMinionWaitDone(reports, infoPrefix)
				if failures > 0 {
					w.logger.Errorf(ctx, msg)
					w.setErrorStatus(ctx, "%s, some agents reported failure", infoPrefix)
					return false, nil
				}
				w.logger.Infof(ctx, msg)
				w.setInfoStatus(ctx, "%s, all agents reported success", infoPrefix)
				return true, nil
			}

		case <-logProgress:
			w.setInfoStatus(ctx, "%s, %s", infoPrefix, formatMinionWaitUpdate(reports))
			logProgress = clk.After(progressUpdateInterval)
		}
	}
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
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
	failedCount := len(reports.FailedMachines) + len(reports.FailedUnits) + len(reports.FailedApplications)
	if reports.SuccessCount+failedCount > reports.TotalCount {
		return errors.Annotatef(
			coremigration.ErrMinionReportsInvalid,
			"minion report counts exceed total agents: %d succeeded, %d failed, %d total",
			reports.SuccessCount, failedCount, reports.TotalCount,
		)
	}
	return nil
}

func formatMinionTimeout(
	reports coremigration.MinionReports,
	status coremigration.MigrationStatus,
	infoPrefix string,
) string {
	if reports.IsZero() {
		return "no agents reported in time"
	}

	var fails []string
	if len(reports.SomeUnknownMachines) > 0 {
		fails = append(fails, fmt.Sprintf("machines: %s", strings.Join(reports.SomeUnknownMachines, ",")))
	}
	if len(reports.SomeUnknownUnits) > 0 {
		fails = append(fails, fmt.Sprintf("units: %s", strings.Join(reports.SomeUnknownUnits, ",")))
	}
	if len(reports.SomeUnknownApplications) > 0 {
		fails = append(fails, fmt.Sprintf("applications: %s", strings.Join(reports.SomeUnknownApplications, ",")))
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
	if len(reports.FailedApplications) > 0 {
		fails = append(fails, fmt.Sprintf("applications: %s", strings.Join(reports.FailedApplications, ",")))
	}
	return fmt.Sprintf("agents failed phase %q (%s)", infoPrefix, strings.Join(fails, "; "))
}

func formatMinionWaitDone(reports coremigration.MinionReports, infoPrefix string) string {
	return fmt.Sprintf("completed waiting for agents to report for %q, %d succeeded, %d failed",
		infoPrefix, reports.SuccessCount, len(reports.FailedMachines)+len(reports.FailedUnits)+len(reports.FailedApplications))
}

func formatMinionWaitUpdate(reports coremigration.MinionReports) string {
	if reports.IsZero() {
		return "no reports from agents yet"
	}

	msg := fmt.Sprintf("waiting for agents to report back: %d succeeded, %d still to report",
		reports.SuccessCount, reports.UnknownCount)
	failed := len(reports.FailedMachines) + len(reports.FailedUnits) + len(reports.FailedApplications)
	if failed > 0 {
		msg += fmt.Sprintf(", %d failed", failed)
	}
	return msg
}

func (w *Worker) openTargetAPI(ctx context.Context, targetInfo coremigration.TargetInfo) (*migrationtarget.Client, func() error, error) {
	conn, err := w.openAPIConn(ctx, targetInfo)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return migrationtarget.NewClient(conn), conn.Close, nil
}

func (w *Worker) openAPIConn(ctx context.Context, targetInfo coremigration.TargetInfo) (api.Connection, error) {
	return w.openAPIConnForModel(ctx, targetInfo, "")
}

func (w *Worker) openAPIConnForModel(ctx context.Context, targetInfo coremigration.TargetInfo, modelUUID string) (api.Connection, error) {
	apiInfo := &api.Info{
		Addrs:     targetInfo.Addrs,
		CACert:    targetInfo.CACert,
		Password:  targetInfo.Password,
		ModelTag:  names.NewModelTag(modelUUID),
		Macaroons: targetInfo.Macaroons,
	}
	if targetInfo.User != "" {
		if !names.IsValidUser(targetInfo.User) {
			return nil, errors.Errorf("user %q not valid", targetInfo.User)
		}
		authTag := names.NewUserTag(targetInfo.User)
		// Only local users must be added to the api info.
		// For external users, the tag needs to be left empty.
		if authTag.IsLocal() {
			apiInfo.Tag = authTag
		}
	}
	loginProvider := migration.NewLoginProvider(targetInfo)
	return w.config.APIOpen(ctx, apiInfo, migration.ControllerDialOpts(loginProvider))
}

func modelHasMigrated(phase coremigration.Phase) bool {
	return phase == coremigration.DONE || phase == coremigration.REAPFAILED
}

// resourceDownloaderShim implements migration.ResourceDownloader by
// resolving the resource UUID from the application/name pair and opening
// the resource content through the domain service directly.
type resourceDownloaderShim struct {
	svc ResourceService
}

func (a *resourceDownloaderShim) OpenResource(ctx context.Context, appName, resName string) (io.ReadCloser, error) {
	uuid, err := a.svc.GetResourceUUIDByApplicationAndResourceName(ctx, appName, resName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	_, reader, err := a.svc.OpenResource(ctx, uuid)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return reader, nil
}
