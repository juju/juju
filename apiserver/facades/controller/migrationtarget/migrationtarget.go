// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/collections/set"
	"github.com/juju/description/v12"
	"github.com/juju/names/v6"
	"github.com/vallerion/rscanner"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	corecredential "github.com/juju/juju/core/credential"
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
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	"github.com/juju/juju/domain/export"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/rpc/params"
)

// ModelImporter defines an interface for importing models.
type ModelImporter interface {
	// ImportModel takes a serialized description model (yaml bytes) and returns
	// a state model and state state.
	ImportModel(ctx context.Context, bytes []byte) error
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
	// Controller returns the external controller record for the given
	// controller UUID, or an error satisfying [coreerrors.NotFound] when no
	// such record exists.
	Controller(ctx context.Context, controllerUUID string) (*crossmodel.ControllerInfo, error)

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

// UserService provides user lookups for v8 prechecks.
type UserService interface {
	// GetUserByName returns the user with the given name, excluding removed
	// users. It returns an error satisfying [accesserrors.UserNotFound] when
	// no such user exists.
	GetUserByName(ctx context.Context, name user.Name) (user.User, error)
}

// CredentialService provides cloud credential lookups for v8 prechecks.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given key, or an
	// error satisfying [credentialerrors.NotFound] when no such credential
	// exists.
	CloudCredential(ctx context.Context, key corecredential.Key) (cloud.Credential, error)
}

// SecretBackendService provides secret backend lookups for v8 prechecks.
type SecretBackendService interface {
	// GetSecretBackendByName returns the secret backend with the given name,
	// or an error satisfying [secretbackenderrors.NotFound] when no such
	// backend exists.
	GetSecretBackendByName(ctx context.Context, name string) (*secretbackend.SecretBackend, error)
}

// MigrationImportService provides the controller-scoped import guard reads
// for v8 prechecks and import.
type MigrationImportService interface {
	// CheckTargetImportSchema verifies that the controller database schema
	// can host a migrationtarget v8 model import. On failure it returns an
	// error satisfying [coreerrors.NotSupported].
	CheckTargetImportSchema(ctx context.Context) error

	// GetImportClaim returns the target-side import claim held for the given
	// model UUID, or [modelmigrationerrors.ErrImportNotFound] when no claim
	// exists.
	GetImportClaim(ctx context.Context, modelUUID coremodel.UUID) (modelmigration.ImportClaim, error)

	// ModelNamespaceExists reports whether the given model's dqlite namespace
	// mapping already exists on this controller.
	ModelNamespaceExists(ctx context.Context, modelUUID coremodel.UUID) (bool, error)
}

// APIV8 implements the MigrationTarget v8 facade: typed-envelope
// (params.SerializedModelV2) prechecks and import. It embeds the v7 API for
// the methods that are unchanged (Abort, Activate, AdoptResources, etc.) and
// shadows Prechecks and Import with the v8 envelope signatures — the inverse
// of the legacy.go adapter pattern, because Go cannot overload method names
// by parameter type.
type APIV8 struct {
	*API

	localMacaroonMinter facade.LocalMacaroonMinter

	controllerUUID         string
	userService            UserService
	credentialService      CredentialService
	secretBackendService   SecretBackendService
	migrationImportService MigrationImportService
}

// NewAPIV8 returns a new MigrationTarget v8 facade wrapping the given v7 API.
func NewAPIV8(
	api *API,
	minter facade.LocalMacaroonMinter,
	controllerUUID string,
	userService UserService,
	credentialService CredentialService,
	secretBackendService SecretBackendService,
	migrationImportService MigrationImportService,
) (*APIV8, error) {
	return &APIV8{
		API:                    api,
		localMacaroonMinter:    minter,
		controllerUUID:         controllerUUID,
		userService:            userService,
		credentialService:      credentialService,
		secretBackendService:   secretBackendService,
		migrationImportService: migrationImportService,
	}, nil
}

// Prechecks ensures that the target controller is ready to accept the model
// described by the v8 envelope. It performs schema, payload-version, static
// payload and environmental checks without writing any target-side rows and
// without deserializing a description.Model.
func (api *APIV8) Prechecks(ctx context.Context, envelope params.SerializedModelV2) error {
	return api.prechecks(ctx, envelope)
}

// Import accepts a v8 model migration envelope. The full import path (durable
// claim, controller-fact application and model-DB import) is not implemented
// yet; the method runs only the mandatory pre-write guards and then succeeds
// as a no-op shell WITHOUT importing anything, so the source-side migration
// worker can be exercised against this facade during development.
//
// Unlike Prechecks, Import deliberately does NOT re-run the static payload or
// environmental prechecks. Per the spec (WS4a / Task 6) the only work that
// must precede the first target-side write is schema validation, payload
// version/decode preparation and the non-empty SourceMigrationUUID check —
// exactly what guards covers. The equivalent collision checks become
// structural guarded writes inside the real import path (UNIQUE(model_uuid)
// claim insert, compare-or-insert controller facts), so Import does not
// duplicate the Prechecks routine. This mirrors v7, where Import does not
// re-run Prechecks either.
func (api *APIV8) Import(ctx context.Context, envelope params.SerializedModelV2) error {
	if _, err := api.guards(ctx, envelope); err != nil {
		return errors.Capture(err)
	}

	// TODO(modelmigration): implement the v8 import path: durable
	// model_migration_import claim, target-local model bootstrap,
	// controller-fact application and the V2 model-DB import. This must land
	// before a 4.1 release ships.
	api.logger.Warningf(ctx,
		"MigrationTarget v8 Import for model %q is a no-op shell; the model was NOT imported",
		envelope.ModelInfo.UUID)
	return nil
}

// prechecks is the v8 precheck routine used by Prechecks. It runs the
// mandatory pre-write guards followed by the static payload and environmental
// checks. It must not write any target-side rows.
func (api *APIV8) prechecks(ctx context.Context, envelope params.SerializedModelV2) error {
	view, err := api.guards(ctx, envelope)
	if err != nil {
		return errors.Capture(err)
	}

	// Static payload checks (charm manifests, fan config).
	if err := migration.ImportPayloadPrecheck(ctx, view); err != nil {
		return errors.Errorf("migration import prechecks: %w", err)
	}

	// Environmental checks against the target controller.
	if err := api.targetPrechecks(ctx, envelope, view); err != nil {
		return errors.Errorf("migration target prechecks failed: %w", err)
	}

	return nil
}

// guards runs the mandatory pre-write guards that must pass before any
// target-side row is written on the v8 import path: envelope identity
// validation (including a non-empty source migration UUID), the runtime
// controller-schema guard and the payload version/decode checks. It returns
// the version-neutral static-check view of the decoded payload for callers
// that go on to run the static/environmental prechecks. It must not write any
// target-side rows.
func (api *APIV8) guards(ctx context.Context, envelope params.SerializedModelV2) (export.StaticCheckView, error) {
	// Envelope sanity first: nothing else is meaningful without a valid
	// model identity.
	if err := validateEnvelopeModelInfo(envelope.ModelInfo); err != nil {
		return export.StaticCheckView{}, errors.Capture(err)
	}

	// Runtime schema guard: the controller DB must carry the v8 import
	// objects before any further work.
	if err := api.migrationImportService.CheckTargetImportSchema(ctx); err != nil {
		return export.StaticCheckView{}, errors.Capture(err)
	}

	// Payload-version checks. The newer-than-target check runs before the
	// decoder registry lookup so a payload from a newer Juju gets the
	// actionable upgrade message rather than an unknown-version error.
	targetVersion := export.LatestSupportedPayloadVersion()
	if envelope.PayloadVersion.Compare(targetVersion) > 0 {
		return export.StaticCheckView{}, errors.Errorf(
			"source payload version %q is newer than target %q; upgrade the target controller first %w",
			envelope.PayloadVersion, targetVersion, coreerrors.NotSupported)
	}

	payload, err := export.DecodePayload(envelope.PayloadVersion, envelope.Payload)
	if err != nil {
		return export.StaticCheckView{}, errors.Capture(err)
	}

	view, err := export.StaticCheckViewFor(payload)
	if err != nil {
		return export.StaticCheckView{}, errors.Capture(err)
	}

	return view, nil
}

// validateEnvelopeModelInfo validates the bootstrap identity fields of the v8
// envelope.
func validateEnvelopeModelInfo(info params.SerializedModelInfo) error {
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
// controller, using only the typed envelope fields and the decoded payload
// view.
func (api *APIV8) targetPrechecks(
	ctx context.Context, envelope params.SerializedModelV2, view export.StaticCheckView,
) error {
	// Target controller readiness: upgrade in progress and controller
	// machine health, mirroring the v7 TargetPrecheck coverage.
	modelAgentService, err := api.modelAgentServiceGetter(ctx, api.controllerModelUUID)
	if err != nil {
		return errors.Errorf("cannot get model agent service: %w", err)
	}
	if err := migration.TargetControllerPrecheck(
		ctx, api.upgradeService, api.statusService, modelAgentService, api.machineService,
	); err != nil {
		return errors.Capture(err)
	}

	// The migrating model's agent version must not be ahead of the target
	// controller. The version comes from the payload's agent_version row; the
	// v8 envelope deliberately carries no separate agent version field.
	controllerVersion, err := modelAgentService.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return errors.Errorf("retrieving target controller version: %w", err)
	}
	if view.AgentTargetVersion != (semversion.Number{}) &&
		controllerVersion.Compare(view.AgentTargetVersion) < 0 {
		return errors.Errorf("model has higher version than target controller (%s > %s)",
			view.AgentTargetVersion, controllerVersion)
	}

	if err := api.checkCloudAndRegion(ctx, envelope.ModelInfo); err != nil {
		return errors.Capture(err)
	}
	if err := api.checkUsers(ctx, envelope.Users); err != nil {
		return errors.Capture(err)
	}
	if err := api.checkCredential(ctx, envelope.ModelCredential); err != nil {
		return errors.Capture(err)
	}
	if err := api.checkSecretBackend(ctx, envelope.SecretBackend); err != nil {
		return errors.Capture(err)
	}
	if err := api.checkExternalControllers(ctx, envelope.ExternalControllers); err != nil {
		return errors.Capture(err)
	}
	if err := api.checkModelCollisions(ctx, envelope.ModelInfo); err != nil {
		return errors.Capture(err)
	}
	return nil
}

// checkCloudAndRegion verifies the model's cloud exists on the target and, if
// set, that the cloud region is known to that cloud.
func (api *APIV8) checkCloudAndRegion(ctx context.Context, info params.SerializedModelInfo) error {
	cl, err := api.cloudService.Cloud(ctx, info.Cloud)
	if errors.Is(err, clouderrors.NotFound) {
		return errors.Errorf("model's cloud %q not found on target controller", info.Cloud)
	} else if err != nil {
		return errors.Errorf("retrieving cloud %q: %w", info.Cloud, err)
	}
	if info.CloudRegion == "" {
		return nil
	}
	if _, err := cloud.RegionByName(cl.Regions, info.CloudRegion); err != nil {
		return errors.Errorf(
			"model's cloud region %q not valid for cloud %q on target controller: %w",
			info.CloudRegion, info.Cloud, err)
	}
	return nil
}

// checkUsers verifies that every model user in the envelope can be applied on
// the target: a missing user is fine (it is recreated on import), but an
// existing user must be usable.
func (api *APIV8) checkUsers(ctx context.Context, users []params.ModelUser) error {
	for _, u := range users {
		name, err := user.NewName(u.Name)
		if err != nil {
			return errors.Errorf("model user name %q %w", u.Name, coreerrors.NotValid)
		}
		existing, err := api.userService.GetUserByName(ctx, name)
		if errors.Is(err, accesserrors.UserNotFound) {
			// The user is recreated from the envelope on import.
			continue
		} else if err != nil {
			return errors.Errorf("retrieving model user %q: %w", u.Name, err)
		}
		if existing.Disabled {
			return errors.Errorf("model user %q is disabled on the target controller", u.Name)
		}
	}
	return nil
}

// checkCredential verifies that the model's cloud credential either does not
// exist on the target (it is created on import) or matches the incoming
// credential exactly.
func (api *APIV8) checkCredential(ctx context.Context, cred *params.ModelCloudCredential) error {
	if cred == nil {
		return nil
	}
	owner, err := user.NewName(cred.Owner)
	if err != nil {
		return errors.Errorf("model credential owner %q %w", cred.Owner, coreerrors.NotValid)
	}
	key := corecredential.Key{
		Cloud: cred.Cloud,
		Owner: owner,
		Name:  cred.Name,
	}
	existing, err := api.credentialService.CloudCredential(ctx, key)
	if errors.Is(err, credentialerrors.NotFound) {
		// The credential is created from the envelope on import.
		return nil
	} else if err != nil {
		return errors.Errorf("retrieving model credential %q: %w", key, err)
	}
	if string(existing.AuthType()) != cred.AuthType {
		return errors.Errorf(
			"model credential %q already exists on the target controller with auth-type %q, not %q",
			key, existing.AuthType(), cred.AuthType)
	}
	if !maps.Equal(existing.Attributes(), cred.Attributes) {
		return errors.Errorf(
			"model credential %q already exists on the target controller with different attributes", key)
	}
	if existing.Revoked && !cred.Revoked {
		return errors.Errorf(
			"model credential %q is revoked on the target controller", key)
	}
	return nil
}

// checkSecretBackend verifies the model's secret backend exists on the target.
func (api *APIV8) checkSecretBackend(ctx context.Context, backend *params.ModelSecretBackend) error {
	if backend == nil {
		return nil
	}
	_, err := api.secretBackendService.GetSecretBackendByName(ctx, backend.Name)
	if errors.Is(err, secretbackenderrors.NotFound) {
		return errors.Errorf("model's secret backend %q not found on target controller", backend.Name)
	} else if err != nil {
		return errors.Errorf("retrieving secret backend %q: %w", backend.Name, err)
	}
	return nil
}

// checkExternalControllers verifies that none of the third-party external
// controllers referenced by the model collide with a controller already
// registered on the target under the same UUID but with different
// addresses or CA certificate.
func (api *APIV8) checkExternalControllers(ctx context.Context, refs []params.ExternalControllerRef) error {
	for _, ref := range refs {
		if ref.UUID == api.controllerUUID {
			// Offers consumed from this (target) controller become local
			// again after migration; there is no external record to compare.
			continue
		}
		existing, err := api.externalControllerService.Controller(ctx, ref.UUID)
		if errors.Is(err, coreerrors.NotFound) {
			// The controller record is created from the envelope on import.
			continue
		} else if err != nil {
			return errors.Errorf("retrieving external controller %q: %w", ref.UUID, err)
		}
		if existing.CACert != ref.CACert ||
			!set.NewStrings(existing.Addrs...).Difference(set.NewStrings(ref.Addresses...)).IsEmpty() ||
			!set.NewStrings(ref.Addresses...).Difference(set.NewStrings(existing.Addrs...)).IsEmpty() {
			return errors.Errorf(
				"external controller %q: %w",
				ref.UUID, modelmigrationerrors.ErrExternalControllerConflict)
		}
	}
	return nil
}

// checkModelCollisions rejects imports that would collide with live rows on
// the target's shared-namespace tables (model, model_namespace) or with an
// existing import claim, and rejects model name/qualifier conflicts.
func (api *APIV8) checkModelCollisions(ctx context.Context, info params.SerializedModelInfo) error {
	modelUUID := coremodel.UUID(info.UUID)

	claim, err := api.migrationImportService.GetImportClaim(ctx, modelUUID)
	if err == nil {
		// TODO(modelmigration): the import claim/retry semantics (coded
		// AlreadyExists with phase-specific wording on Import) are owned by
		// the import-path task; prechecks only report the occupied slot.
		return errors.Errorf(
			"model %q already has an import claim on this controller (phase %q, source migration %q, updated %s)",
			modelUUID, claim.Phase, claim.SourceMigrationUUID, claim.UpdatedAt.Format("2006-01-02 15:04:05"))
	} else if !errors.Is(err, modelmigrationerrors.ErrImportNotFound) {
		return errors.Errorf("retrieving import claim for model %q: %w", modelUUID, err)
	}

	// No import claim: any live row for this UUID is a hard collision.
	_, err = api.modelService.Model(ctx, modelUUID)
	if err == nil {
		return errors.Errorf("model with same UUID already exists (%s)", modelUUID)
	} else if !errors.Is(err, modelerrors.NotFound) {
		return errors.Errorf("retrieving model %q: %w", modelUUID, err)
	}

	exists, err := api.migrationImportService.ModelNamespaceExists(ctx, modelUUID)
	if err != nil {
		return errors.Errorf("checking model namespace for model %q: %w", modelUUID, err)
	}
	if exists {
		return errors.Errorf(
			"model database namespace for %q already exists on target controller", modelUUID)
	}

	models, err := api.modelService.GetAllModels(ctx)
	if err != nil {
		return errors.Errorf("retrieving models: %w", err)
	}
	for _, model := range models {
		if model.Name == info.Name && model.Qualifier.String() == info.Qualifier {
			return errors.Errorf("model named %q already exists", info.Name)
		}
	}
	return nil
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
