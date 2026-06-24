// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/description/v12"
	"github.com/juju/names/v6"
	"github.com/vallerion/rscanner"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	coremigration "github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/export"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/rpc/params"
)

// ModelImporter defines an interface for importing models.
type ModelImporter interface {
	// ImportModel takes a serialized description model (yaml bytes) and returns
	// a state model and state state.
	ImportModel(ctx context.Context, bytes []byte) error

	// ImportModelV2 applies a v8 migration envelope's controller-scoped
	// semantic data to the target controller: the durable
	// model_migration_import claim, the target-local model bootstrap, and
	// the users, credential, permissions, authorized keys, secret backend,
	// leadership and cloud image metadata carried by the envelope.
	ImportModelV2(ctx context.Context, envelope params.SerializedModelV2, view export.ProjectionView) error
}

// CloudService provides a subset of the cloud domain service methods.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
	// ListAll returns all the clouds.
	ListAll(ctx context.Context) ([]cloud.Cloud, error)
}

// ExternalControllerService provides a subset of the external controller
// domain service methods.
type ExternalControllerService interface {
	// ControllerForModel returns the controller record that's associated
	// with the modelUUID.
	ControllerForModel(ctx context.Context, modelUUID string) (*crossmodel.ControllerInfo, error)

	// UpdateExternalController persists the input controller
	// record.
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo) error
}

// ControllerConfigService provides a subset of the controller config domain
// service methods.
type ControllerConfigService interface {
	// ControllerConfig returns the controller config.
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelManagerService describes the method needed to update model metadata.
type ModelManagerService interface {
	Create(context.Context, coremodel.UUID) error
}

// ModelMigrationService provides the means for supporting model migration
// actions between controllers and answering questions about the underlying
// model(s) that are being migrated.
type ModelMigrationService interface {
	// AdoptResources is responsible for taking ownership of the cloud resources
	// of a model when it has been migrated into this controller.
	AdoptResources(context.Context, semversion.Number) error

	// CheckMachines is responsible for checking a model after it has been
	// migrated into this target controller. We check the machines that exist in
	// the model against the machines reported by the models cloud and report
	// any discrepancies.
	CheckMachines(context.Context) ([]modelmigration.MigrationMachineDiscrepancy, error)

	// ActivateImport finalises the import of the model.
	ActivateImport(ctx context.Context) error

	// ModelMigrationMode returns the current migration mode for the model.
	ModelMigrationMode(ctx context.Context) (modelmigration.MigrationMode, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetMachinesNotAtTargetAgentVersion reports all of the machines in the model that
	// are currently not at the desired target version. This also returns machines
	// that have no reported agent version set. If all units are up to the
	// target version or no units exist in the model a zero length slice is
	// returned.
	GetMachinesNotAtTargetAgentVersion(context.Context) ([]machine.Name, error)

	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/modelagent/errors.NotFound] when the model
	// does not exist.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// GetUnitsNotAtTargetAgentVersion reports all of the units in the model that
	// are currently not at the desired target agent version. This also returns
	// units that have no reported agent version set. If all units are up to the
	// target version or no units exist in the model a zero length slice is
	// returned.
	GetUnitsNotAtTargetAgentVersion(context.Context) ([]unit.Name, error)
}

// ModelMigrationServiceGetter describes a function that is able to return the
// [ModelMigrationService] for a given model id.
type ModelMigrationServiceGetter func(context.Context, coremodel.UUID) (ModelMigrationService, error)

// RemovalServiceGetter describes a function that is able to return the
// [RemovalService] for a given model id.
type RemoveServiceGetter func(context.Context, coremodel.UUID) (RemovalService, error)

// ModelAgentServiceGetter describes a function that is able to return the
// [ModelAgentService] for a given model id.
type ModelAgentServiceGetter func(context.Context, coremodel.UUID) (ModelAgentService, error)

// UpgradeService provides a subset of the upgrade domain service methods.
type UpgradeService interface {
	// IsUpgrading returns whether the controller is currently upgrading.
	IsUpgrading(context.Context) (bool, error)
}

// StatusService defines the methods that the facade assumes from the Status
// service.
type StatusService interface {
	// CheckUnitStatusesReadyForMigration returns true is the statuses of all units
	// in the model indicate they can be migrated.
	CheckUnitStatusesReadyForMigration(context.Context) error

	// CheckMachineStatusesReadyForMigration returns an error if the statuses of any
	// machines in the model indicate they cannot be migrated.
	CheckMachineStatusesReadyForMigration(context.Context) error
}

// ModelService defines the methods to get models hosted on this controller.
type ModelService interface {
	// GetAllModels  lists all models in the controller. If no models exist then
	// an empty slice is returned.
	GetAllModels(ctx context.Context) ([]coremodel.Model, error)
	// Model returns the model associated with the provided uuid.
	Model(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error)
}

// MachineService is used to get the life of all machines before migrating.
type MachineService interface {
	// AllMachineNames returns the names of all machines in the model.
	AllMachineNames(ctx context.Context) ([]machine.Name, error)
	// GetMachineLife returns the GetMachineLife status of the specified machine.
	// It returns a NotFound if the given machine doesn't exist.
	GetMachineLife(ctx context.Context, machineName machine.Name) (life.Value, error)
	// GetMachineBase returns the base for the given machine.
	//
	// The following errors may be returned:
	// - [machineerrors.MachineNotFound] if the machine does not exist.
	GetMachineBase(ctx context.Context, mName machine.Name) (base.Base, error)
}

// RemovalService defines the methods required to remove an importing model
// that has failed to import completely.
type RemovalService interface {
	// RemoveMigratingModel removes the model that is in the importing state.
	RemoveMigratingModel(ctx context.Context, modelUUID coremodel.UUID) error
}

// APIV4 implements the APIV4.
type APIV4 struct {
	*APIV5
}

// APIV5 implements the APIV5.
type APIV5 struct {
	*APIV6
}

// APIV6 implements the APIV6.
type APIV6 struct {
	*API
}

// API implements the API required for the model migration
// master worker when communicating with the target controller.
type API struct {
	controllerModelUUID coremodel.UUID

	modelImporter  ModelImporter
	modelService   ModelService
	upgradeService UpgradeService
	statusService  StatusService
	machineService MachineService

	cloudService                CloudService
	controllerConfigService     ControllerConfigService
	externalControllerService   ExternalControllerService
	modelAgentServiceGetter     ModelAgentServiceGetter
	modelMigrationServiceGetter ModelMigrationServiceGetter
	removalServiceGetter        RemoveServiceGetter
	authorizer                  facade.Authorizer

	requiredMigrationFacadeVersions facades.FacadeVersions

	logDir string
	logger corelogger.Logger
}

// NewAPI returns a new migration target api. Accepts a NewEnvironFunc and
// envcontext.ProviderCallContext for testing purposes.
func NewAPI(
	ctx facade.ModelContext,
	authorizer facade.Authorizer,
	cloudService CloudService,
	controllerConfigService ControllerConfigService,
	externalControllerService ExternalControllerService,
	modelService ModelService,
	upgradeService UpgradeService,
	statusService StatusService,
	machineService MachineService,
	modelAgentServiceGetter ModelAgentServiceGetter,
	modelMigrationServiceGetter ModelMigrationServiceGetter,
	removalServiceGetter RemoveServiceGetter,
	requiredMigrationFacadeVersions facades.FacadeVersions,
	logDir string,
	logger corelogger.Logger,
) (*API, error) {
	return &API{
		controllerModelUUID:             ctx.ControllerModelUUID(),
		modelImporter:                   ctx.ModelImporter(),
		cloudService:                    cloudService,
		controllerConfigService:         controllerConfigService,
		externalControllerService:       externalControllerService,
		modelService:                    modelService,
		upgradeService:                  upgradeService,
		statusService:                   statusService,
		machineService:                  machineService,
		modelAgentServiceGetter:         modelAgentServiceGetter,
		modelMigrationServiceGetter:     modelMigrationServiceGetter,
		removalServiceGetter:            removalServiceGetter,
		authorizer:                      authorizer,
		requiredMigrationFacadeVersions: requiredMigrationFacadeVersions,
		logDir:                          logDir,
		logger:                          logger,
	}, nil
}

func checkAuth(ctx context.Context, authorizer facade.Authorizer, controllerTag names.Tag) error {
	if !authorizer.AuthClient() {
		return errors.New(
			"client does not have permission for migration target facade",
		).Add(apiservererrors.ErrPerm)
	}

	return authorizer.HasPermission(ctx, permission.SuperuserAccess, controllerTag)
}

// Prechecks ensure that the target controller is ready to accept a
// model migration.
func (api *API) Prechecks(ctx context.Context, model params.MigrationModelInfo) error {
	modelDescription, err := description.Deserialize(model.ModelDescription)
	if err != nil {
		return errors.Errorf(
			"cannot deserialize model %q description during prechecks: %w",
			model.UUID,
			err,
		)
	}

	// Ensure that when attempting to migrate a model, the source
	// controller has the required facades for the migration.
	sourceFacadeVersions := facades.FacadeVersions{}
	for name, versions := range model.FacadeVersions {
		sourceFacadeVersions[name] = versions
	}
	if !facades.CompleteIntersection(api.requiredMigrationFacadeVersions, sourceFacadeVersions) {
		majorMinor := fmt.Sprintf("%d.%d",
			model.ControllerAgentVersion.Major,
			model.ControllerAgentVersion.Minor,
		)

		// If the patch is zero, then we don't need to mention it.
		var patchMessage string
		if model.ControllerAgentVersion.Patch > 0 {
			patchMessage = fmt.Sprintf(", that is greater than %s.%d", majorMinor, model.ControllerAgentVersion.Patch)
		}

		return errors.Errorf(`
Source controller does not support required facades for performing migration.
Upgrade the controller to a newer version of %s%s or migrate to a controller
with an earlier version of the target controller and try again.

`[1:], majorMinor, patchMessage)
	}

	err = migration.ImportDescriptionPrecheck(ctx, modelDescription)
	if err != nil {
		return fmt.Errorf("migration import prechecks: %w", err)
	}

	// NOTE (thumper): it isn't clear to me why api.state would be different
	// from the controllerState as I had thought that the Precheck call was
	// on the controller model, in which case it should be the same as the
	// controllerState.
	modelAgentService, err := api.modelAgentServiceGetter(ctx, api.controllerModelUUID)
	if err != nil {
		return errors.Errorf("cannot get model agent service: %w", err)
	}

	if err := migration.TargetPrecheck(
		ctx,
		coremigration.ModelInfo{
			UUID:                   model.UUID,
			Name:                   model.Name,
			Qualifier:              coremodel.Qualifier(model.Qualifier),
			AgentVersion:           model.AgentVersion,
			ControllerAgentVersion: model.ControllerAgentVersion,
			ModelDescription:       modelDescription,
		},
		api.modelService,
		api.upgradeService,
		api.statusService,
		modelAgentService,
		api.machineService,
		api.cloudService,
		func(ctx context.Context, modelUUID coremodel.UUID) (migration.ModelMigrationService, error) {
			return api.modelMigrationServiceGetter(ctx, modelUUID)
		},
	); err != nil {
		return errors.Errorf("migration target prechecks failed: %w", err)
	}
	return nil
}

// Import takes a serialized Juju model, deserializes it, and
// recreates it in the receiving controller.
func (api *API) Import(ctx context.Context, serialized params.SerializedModel) error {
	err := api.modelImporter.ImportModel(ctx, serialized.Bytes)
	if err != nil {
		return err
	}
	return nil
}

// Abort removes the specified model from the database. It is an error to
// attempt to Abort a model that has a migration mode other than importing.
func (api *API) Abort(ctx context.Context, args params.ModelArgs) error {
	modelTag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return errors.Capture(err)
	}

	api.logger.Debugf(ctx, "Abort migrating model %q", args.ModelTag)

	modelUUID := coremodel.UUID(modelTag.Id())
	removalServiceGetter, err := api.removalServiceGetter(ctx, modelUUID)
	if err != nil {
		return errors.Capture(err)
	}

	err = removalServiceGetter.RemoveMigratingModel(ctx, modelUUID)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// Activate sets the migration mode of the model to "none", meaning it
// is ready for use. It also adds any required
// external controller records for those controllers hosting offers used
// by the model.
func (api *API) Activate(ctx context.Context, args params.ActivateModelArgs) error {
	modelTag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return errors.Capture(err)
	}

	api.logger.Debugf(ctx, "Activate migrating model %q", args.ModelTag)

	modelUUID := coremodel.UUID(modelTag.Id())
	modelMigrationService, err := api.modelMigrationServiceGetter(ctx, modelUUID)
	if err != nil {
		return errors.Capture(err)
	}

	// Add any required external controller records if there are cross
	// model relations to the source controller that were local but
	// now need to be external after migration.
	if len(args.CrossModelUUIDs) > 0 {
		cTag, err := names.ParseControllerTag(args.ControllerTag)
		if err != nil {
			return errors.Errorf(
				"cannot parse controller tag when activating model %q: %w",
				modelUUID,
				err,
			)
		}
		err = api.externalControllerService.UpdateExternalController(ctx, crossmodel.ControllerInfo{
			ControllerUUID: cTag.Id(),
			Alias:          args.ControllerAlias,
			Addrs:          args.SourceAPIAddrs,
			CACert:         args.SourceCACert,
			ModelUUIDs:     args.CrossModelUUIDs,
		})
		if err != nil {
			return errors.Errorf(
				"cannot save source controller %q info when activating model %q: %w",
				cTag.Id(),
				modelUUID,
				err,
			)
		}
	}

	// Activate the import, this will clear any migration flags and allow the
	// model to be used normally.
	if err := modelMigrationService.ActivateImport(ctx); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// LatestLogTime returns the time of the most recent log record
// received by the logtransfer endpoint. This can be used as the start
// point for streaming logs from the source if the transfer was
// interrupted.
//
// Log messages are assumed to be sent in time order (which is how
// debug-log emits them). If that isn't the case then this mechanism
// can't be used to avoid duplicates when logtransfer is restarted.
//
// Returns the zero time if no logs have been transferred.
func (api *API) LatestLogTime(ctx context.Context, args params.ModelArgs) (time.Time, error) {
	tag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return time.Time{}, errors.Errorf("cannot parse model tag: %w", err)
	}
	modelUUID := tag.Id()

	// Look up the last line in the log file and get the timestamp.
	// TODO (stickupkid): This should come from the logsink directly, to
	// prevent unfettered access.
	logFile := filepath.Join(api.logDir, "logsink.log")

	f, err := os.Open(logFile)
	if err != nil && !os.IsNotExist(err) {
		return time.Time{}, errors.Errorf(
			"cannot open %q log file %q: %w",
			modelUUID,
			logFile,
			err,
		)
	} else if err != nil {
		return time.Time{}, nil
	}
	defer func() {
		_ = f.Close()
	}()

	fs, err := f.Stat()
	if err != nil {
		return time.Time{}, errors.Errorf(
			"cannot interrogate %q log file %q: %w",
			modelUUID,
			logFile,
			err,
		)
	}
	scanner := rscanner.NewScanner(f, fs.Size())

	var lastTimestamp time.Time
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		logRecord, err := unmarshalLine(line)
		if err != nil {
			return time.Time{}, errors.Errorf(
				"cannot unmarshal log line %q: %w", line, err,
			)
		} else if logRecord.ModelUUID != modelUUID {
			continue
		}
		lastTimestamp = logRecord.Time
		break
	}
	return lastTimestamp, nil
}

func unmarshalLine(line []byte) (corelogger.LogRecord, error) {
	var logRecord corelogger.LogRecord
	if err := json.Unmarshal(line, &logRecord); err != nil {
		return logRecord, errors.Errorf("cannot unmarshal log line %q: %w", line, err)
	}
	return logRecord, nil
}

// AdoptResources asks the cloud provider to update the controller
// tags for a model's resources. This prevents the resources from
// being destroyed if the source controller is destroyed after the
// model is migrated away.
func (api *API) AdoptResources(ctx context.Context, args params.AdoptResourcesArgs) error {
	tag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return errors.Errorf("cannot parse model tag: %w", err)
	}

	modelId := coremodel.UUID(tag.Id())
	svc, err := api.modelMigrationServiceGetter(ctx, modelId)
	if err != nil {
		return errors.Errorf("cannot get model migration service for model %q: %w", modelId, err)
	}

	return svc.AdoptResources(ctx, args.SourceControllerVersion)
}

// CheckMachines compares the machines in state with the ones reported
// by the provider and reports any discrepancies.
func (api *API) CheckMachines(ctx context.Context, args params.ModelArgs) (params.ErrorResults, error) {
	tag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return params.ErrorResults{}, errors.Errorf(
			"cannot parse model tag: %w", err,
		)
	}

	modelId := coremodel.UUID(tag.Id())
	migrationService, err := api.modelMigrationServiceGetter(ctx, modelId)
	if err != nil {
		return params.ErrorResults{}, errors.Errorf(
			"cannot get model migration service for model %q: %w",
			modelId,
			err,
		)
	}
	discrepancies, err := migrationService.CheckMachines(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Errorf(
			"cannot check machine discrepancies in imported model %q: %w",
			modelId,
			err,
		)
	}

	result := params.ErrorResults{
		Results: make([]params.ErrorResult, 0, len(discrepancies)),
	}

	for _, discrepancy := range discrepancies {
		var errorMsg string

		// If we have an empty MachineName it means that an instance was found
		// in the models cloud that does not have a corresponding machine in the
		// Juju controller.
		if discrepancy.MachineName == "" {
			errorMsg = fmt.Sprintf(
				"no machine in model %q with instance %q",
				modelId,
				discrepancy.CloudInstanceId,
			)
		} else {
			errorMsg = fmt.Sprintf(
				"could not find cloud instance %q for machine %q",
				discrepancy.CloudInstanceId,
				discrepancy.MachineName,
			)
		}

		result.Results = append(result.Results, params.ErrorResult{
			Error: &params.Error{Message: errorMsg},
		})
	}

	return result, nil
}

// CACert returns the certificate used to validate the state connection.
func (api *API) CACert(ctx context.Context) (params.BytesResult, error) {
	cfg, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return params.BytesResult{}, errors.Errorf(
			"cannot get controller ca certificates for model migration: %w",
			err,
		)
	}
	caCert, _ := cfg.CACert()
	return params.BytesResult{Result: []byte(caCert)}, nil
}

// MigrationImportService runs the environmental v8 import prechecks against
// the target controller database. The migrationtarget facade stays thin: it
// assembles the typed precheck arguments from the v8 args and delegates the
// cross-domain reads to the modelmigration domain.
type MigrationImportService interface {
	// PrecheckImport runs the environmental prechecks for a v8 model migration
	// import against the target controller (cloud/region existence, user
	// usability, credential revoked status, secret backend existence, and
	// model UUID/name collisions). It performs no writes.
	PrecheckImport(ctx context.Context, args modelmigration.ImportPrecheckArgs) error
}

// APIV8 implements the MigrationTarget v8 facade for typed
// params.SerializedModelV2 prechecks and import. It embeds the v7 API for the
// unchanged methods (Abort, Activate, AdoptResources, etc.) and shadows
// Prechecks and Import with the v8 args signatures — the inverse
// of the legacy.go adapter pattern, because Go cannot overload method names
// by parameter type.
type APIV8 struct {
	*API

	localMacaroonMinter facade.LocalMacaroonMinter

	migrationImportService MigrationImportService
}

// NewAPIV8 returns a new MigrationTarget v8 facade wrapping the given v7 API.
func NewAPIV8(
	api *API,
	minter facade.LocalMacaroonMinter,
	migrationImportService MigrationImportService,
) (*APIV8, error) {
	return &APIV8{
		API:                    api,
		localMacaroonMinter:    minter,
		migrationImportService: migrationImportService,
	}, nil
}

// Prechecks ensures that the target controller is ready to accept the model
// described by the v8 args. It performs schema, payload-version and
// environmental checks without writing any target-side rows and without
// deserializing a description.Model.
func (api *APIV8) Prechecks(ctx context.Context, args params.SerializedModelV2) error {
	view, err := api.importGuard(ctx, args)
	if err != nil {
		return errors.Capture(err)
	}

	if err := api.targetPrechecks(ctx, args, view); err != nil {
		return errors.Errorf("migration target prechecks failed: %w", err)
	}

	return nil
}

// Import accepts a v8 model migration envelope, claims the model, bootstraps
// it and applies the envelope's controller-scoped semantic data. Model-DB
// content import and activation are not yet part of this path (Tasks 7-10).
//
// Unlike Prechecks, Import deliberately does NOT re-run the environmental
// prechecks. Per the spec (WS4a / Task 6) the only work that must precede the
// first target-side write is schema validation, payload version/decode
// preparation and the non-empty SourceMigrationUUID check — exactly what
// importGuard covers. The equivalent collision checks become
// structural guarded writes inside the real import path (UNIQUE(model_uuid)
// claim insert, compare-or-insert controller data), so Import does not
// duplicate the Prechecks routine. This mirrors v7, where Import does not
// re-run Prechecks either.
func (api *APIV8) Import(ctx context.Context, envelope params.SerializedModelV2) error {
	view, err := api.importGuard(ctx, envelope)
	if err != nil {
		return errors.Capture(err)
	}

	if err := api.modelImporter.ImportModelV2(ctx, envelope, view); err != nil {
		return errors.Capture(err)
	}
	return nil
}

// importGuard runs the mandatory pre-write checks that must pass before any
// target-side row is written on the v8 import path: model identity
// validation (including a non-empty source migration UUID) and the payload
// version/decode checks. It returns the version-neutral projection view of
// the decoded payload for callers that go on to run environmental
// prechecks. It must not write any target-side rows.
//
// There is no runtime controller-schema guard: the v8 import objects are
// guaranteed present by the time the facade serves requests, because the
// controllerupgrader manifold gates the apiserver behind completion of the
// controller-DB schema upgrade (see the migrationtarget v8 spec, §4.6).
func (api *APIV8) importGuard(ctx context.Context, args params.SerializedModelV2) (export.ProjectionView, error) {
	// Model sanity first: nothing else is meaningful without a valid
	// model identity.
	if err := validateModelInfo(args.ModelInfo); err != nil {
		return export.ProjectionView{}, errors.Capture(err)
	}

	// Payload-version checks. The newer-than-target check runs before the
	// decoder registry lookup so a payload from a newer Juju gets the
	// actionable upgrade message rather than an unknown-version error.
	targetVersion := export.LatestSupportedPayloadVersion()
	if args.PayloadVersion.Compare(targetVersion) > 0 {
		return export.ProjectionView{}, errors.Errorf(
			"source payload version %q is newer than target %q; upgrade the target controller first %w",
			args.PayloadVersion, targetVersion, coreerrors.NotSupported)
	}

	payload, err := export.DecodePayload(args.PayloadVersion, args.Payload)
	if err != nil {
		return export.ProjectionView{}, errors.Capture(err)
	}

	view, err := export.ProjectionViewForPayload(payload)
	if err != nil {
		return export.ProjectionView{}, errors.Capture(err)
	}

	return view, nil
}

// validateModelInfo validates the bootstrap identity fields of the v8 args.
func validateModelInfo(info params.SerializedModelInfo) error {
	if err := coremodel.UUID(info.UUID).Validate(); err != nil {
		return errors.Errorf("model UUID %q %w", info.UUID, coreerrors.NotValid)
	}
	if info.Name == "" {
		return errors.Errorf("empty model name %w", coreerrors.NotValid)
	}
	if info.Qualifier == "" {
		return errors.Errorf("empty model qualifier %w", coreerrors.NotValid)
	}
	if info.SourceMigrationUUID == "" {
		return errors.Errorf("empty source migration UUID %w", coreerrors.NotValid)
	}
	return nil
}

// targetPrechecks runs the environmental v8 prechecks against the target
// controller. The controller-readiness and agent-version checks reuse the
// existing v7 precheck helpers; the cloud/region, user, credential, secret
// backend and model-collision checks are delegated to the modelmigration
// domain (which reads the controller database directly), keeping this facade
// thin.
func (api *APIV8) targetPrechecks(
	ctx context.Context, args params.SerializedModelV2, view export.ProjectionView,
) error {
	// Target controller readiness: upgrade in progress and controller
	// machine health, mirroring the v7 TargetPrecheck coverage.
	modelAgentService, err := api.modelAgentServiceGetter(ctx, api.controllerModelUUID)
	if err != nil {
		return errors.Errorf("cannot get model agent service: %w", err)
	}
	if err := migration.TargetControllerPrecheck(
		ctx, api.upgradeService, api.statusService, modelAgentService, api.machineService,
		view.AgentTargetVersion,
	); err != nil {
		return errors.Capture(err)
	}

	// Environmental checks against the target controller database, owned by
	// the modelmigration domain.
	if err := api.migrationImportService.PrecheckImport(ctx, importPrecheckArgs(args)); err != nil {
		return errors.Capture(err)
	}
	return nil
}

// importPrecheckArgs builds the typed precheck arguments consumed by the
// modelmigration domain from the v8 args' semantic fields.
func importPrecheckArgs(in params.SerializedModelV2) modelmigration.ImportPrecheckArgs {
	args := modelmigration.ImportPrecheckArgs{
		ModelUUID:      in.ModelInfo.UUID,
		ModelName:      in.ModelInfo.Name,
		ModelQualifier: in.ModelInfo.Qualifier,
		Cloud:          in.ModelInfo.Cloud,
		CloudRegion:    in.ModelInfo.CloudRegion,
	}
	for _, u := range in.Users {
		args.Users = append(args.Users, u.Name)
	}
	if cred := in.ModelCredential; cred != nil {
		args.Credential = &modelmigration.ImportPrecheckCredential{
			Cloud:   cred.Cloud,
			Owner:   cred.Owner,
			Name:    cred.Name,
			Revoked: cred.Revoked,
		}
	}
	if in.SecretBackend != nil {
		args.SecretBackend = in.SecretBackend.Name
	}
	for _, m := range in.CloudImageMetadata {
		args.CloudImageMetadata = append(args.CloudImageMetadata, modelmigration.ImportPrecheckImageMetadata{
			Stream:          m.Stream,
			Region:          m.Region,
			Version:         m.Version,
			Arch:            m.Arch,
			VirtType:        m.VirtType,
			RootStorageType: m.RootStorageType,
			Source:          m.Source,
			ImageID:         m.ImageId,
		})
	}
	return args
}

// CreateMigrationMacaroon mints a directly-presentable 24h login macaroon for
// the authenticated admin so the migrationmaster worker can reconnect to this
// controller without a discharge ceremony or a stored cleartext password.
func (api *APIV8) CreateMigrationMacaroon(ctx context.Context) (params.CreateMigrationMacaroonResult, error) {
	tag, ok := api.authorizer.GetAuthTag().(names.UserTag)
	if !ok || !tag.IsLocal() {
		return params.CreateMigrationMacaroonResult{}, apiservererrors.ErrPerm
	}
	mac, err := api.localMacaroonMinter.CreateMigrationMacaroon(ctx, tag, bakery.LatestVersion)
	if err != nil {
		return params.CreateMigrationMacaroonResult{}, errors.Capture(err)
	}
	return params.CreateMigrationMacaroonResult{Macaroon: mac}, nil
}
