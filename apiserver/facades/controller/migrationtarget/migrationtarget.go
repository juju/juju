// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/names/v6"
	"github.com/juju/version/v2"
	"github.com/vallerion/rscanner"

	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	coremigration "github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ModelImporter defines an interface for importing models.
type ModelImporter interface {
	// ImportModel takes a serialized description model (yaml bytes) and returns
	// a state model and state state.
	ImportModel(ctx context.Context, bytes []byte) (*state.Model, *state.State, error)
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

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// GetApplicationLife returns the life value of the application with the
	// given name.
	GetApplicationLife(ctx context.Context, name string) (life.Value, error)
}

type StatusService interface {
	// GetUnitWorkloadStatus returns the workload status of the specified unit.
	// Returns [applicationerrors.UnitNotFound] if the unit does not exist.
	GetUnitWorkloadStatus(context.Context, unit.Name) (*status.StatusInfo, error)
	// GetUnitAgentStatus returns the agent status of the specified unit.
	// Returns [applicationerrors.UnitNotFound] if the unit does not exist.
	GetUnitAgentStatus(context.Context, unit.Name) (*status.StatusInfo, error)
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
	AdoptResources(context.Context, version.Number) error

	// CheckMachines is responsible for checking a model after it has been
	// migrated into this target controller. We check the machines that exist in
	// the model against the machines reported by the models cloud and report
	// any discrepancies.
	CheckMachines(context.Context) ([]modelmigration.MigrationMachineDiscrepancy, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
	// not exist.
	GetModelTargetAgentVersion(context.Context) (version.Number, error)
}

// ModelMigrationServiceGetter describes a function that is able to return the
// [ModelMigrationService] for a given model id.
type ModelMigrationServiceGetter func(context.Context, coremodel.UUID) (ModelMigrationService, error)

// ModelAgentServiceGetter describes a function that is able to return the
// [ModelAgentService] for a given model id.
type ModelAgentServiceGetter func(context.Context, coremodel.UUID) (ModelAgentService, error)

// UpgradeService provides a subset of the upgrade domain service methods.
type UpgradeService interface {
	// IsUpgrading returns whether the controller is currently upgrading.
	IsUpgrading(context.Context) (bool, error)
}

// API implements the API required for the model migration
// master worker when communicating with the target controller.
type API struct {
	state          *state.State
	modelImporter  ModelImporter
	upgradeService UpgradeService

	applicationService          ApplicationService
	statusService               StatusService
	controllerConfigService     ControllerConfigService
	externalControllerService   ExternalControllerService
	modelAgentServiceGetter     ModelAgentServiceGetter
	modelMigrationServiceGetter ModelMigrationServiceGetter

	pool       *state.StatePool
	authorizer facade.Authorizer

	requiredMigrationFacadeVersions facades.FacadeVersions

	logDir string
}

// NewAPI returns a new migration target api. Accepts a NewEnvironFunc and
// envcontext.ProviderCallContext for testing purposes.
func NewAPI(
	ctx facade.ModelContext,
	authorizer facade.Authorizer,
	controllerConfigService ControllerConfigService,
	externalControllerService ExternalControllerService,
	applicationService ApplicationService,
	statusService StatusService,
	upgradeService UpgradeService,
	modelAgentServiceGetter ModelAgentServiceGetter,
	modelMigrationServiceGetter ModelMigrationServiceGetter,
	requiredMigrationFacadeVersions facades.FacadeVersions,
	logDir string,
) (*API, error) {
	return &API{
		state:                           ctx.State(),
		modelImporter:                   ctx.ModelImporter(),
		pool:                            ctx.StatePool(),
		controllerConfigService:         controllerConfigService,
		externalControllerService:       externalControllerService,
		applicationService:              applicationService,
		statusService:                   statusService,
		upgradeService:                  upgradeService,
		modelAgentServiceGetter:         modelAgentServiceGetter,
		modelMigrationServiceGetter:     modelMigrationServiceGetter,
		authorizer:                      authorizer,
		requiredMigrationFacadeVersions: requiredMigrationFacadeVersions,
		logDir:                          logDir,
	}, nil
}

func checkAuth(ctx context.Context, authorizer facade.Authorizer, st *state.State) error {
	if !authorizer.AuthClient() {
		return errors.New(
			"client does not have permission for migration target facade",
		).Add(apiservererrors.ErrPerm)
	}

	return authorizer.HasPermission(ctx, permission.SuperuserAccess, st.ControllerTag())
}

// Prechecks ensure that the target controller is ready to accept a
// model migration.
func (api *API) Prechecks(ctx context.Context, model params.MigrationModelInfo) error {
	var modelDescription description.Model
	if serialized := model.ModelDescription; len(serialized) > 0 {
		var err error
		modelDescription, err = description.Deserialize(model.ModelDescription)
		if err != nil {
			return errors.Errorf(
				"cannot deserialize model %q description during prechecks: %w",
				model.UUID,
				err,
			)
		}
	}

	// If there are no required migration facade versions, then we
	// don't need to check anything.
	if len(api.requiredMigrationFacadeVersions) > 0 {
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
	}

	err := migration.ImportPrecheck(ctx, modelDescription)
	if err != nil {
		return fmt.Errorf("migration import prechecks: %w", err)
	}

	ownerTag, err := names.ParseUserTag(model.OwnerTag)
	if err != nil {
		return errors.Errorf("cannot parse model %q owner during prechecks: %w", model.UUID, err)
	}
	controllerState, err := api.pool.SystemState()
	if err != nil {
		return errors.Errorf(
			"getting system state during prechecks for model %q: %w",
			model.UUID,
			err,
		)
	}
	// NOTE (thumper): it isn't clear to me why api.state would be different
	// from the controllerState as I had thought that the Precheck call was
	// on the controller model, in which case it should be the same as the
	// controllerState.
	modelAgentService, err := api.modelAgentServiceGetter(ctx, coremodel.UUID(controllerState.ModelUUID()))
	if err != nil {
		return errors.Errorf("cannot get model agent service: %w", err)
	}
	backend, err := migration.PrecheckShim(api.state, controllerState)
	if err != nil {
		return errors.Errorf("cannot create prechecks backend: %w", err)
	}
	if err := migration.TargetPrecheck(
		ctx,
		backend,
		migration.PoolShim(api.pool),
		coremigration.ModelInfo{
			UUID:                   model.UUID,
			Name:                   model.Name,
			Owner:                  ownerTag,
			AgentVersion:           model.AgentVersion,
			ControllerAgentVersion: model.ControllerAgentVersion,
			ModelDescription:       modelDescription,
		},
		api.upgradeService,
		api.applicationService,
		api.statusService,
		modelAgentService,
	); err != nil {
		return errors.Errorf("migration target prechecks failed: %w", err)
	}
	return nil
}

// Import takes a serialized Juju model, deserializes it, and
// recreates it in the receiving controller.
func (api *API) Import(ctx context.Context, serialized params.SerializedModel) error {
	_, st, err := api.modelImporter.ImportModel(ctx, serialized.Bytes)
	if err != nil {
		return err
	}
	defer st.Close()
	// TODO(mjs) - post import checks
	// NOTE(fwereade) - checks here would be sensible, but we will
	// also need to check after the binaries are imported too.
	return err
}

func (api *API) getModel(modelTag string) (*state.Model, func(), error) {
	tag, err := names.ParseModelTag(modelTag)
	if err != nil {
		return nil, nil, errors.Errorf("cannot parse model tag: %w", err)
	}
	model, ph, err := api.pool.GetModel(tag.Id())
	if err != nil {
		return nil, nil, errors.Errorf("cannot get model %q: %w", tag.Id(), err)
	}
	return model, func() { ph.Release() }, nil
}

func (api *API) getImportingModel(tag string) (*state.Model, func(), error) {
	model, release, err := api.getModel(tag)
	if err != nil {
		return nil, nil, errors.Errorf("cannot get importing model: %w", err)
	}
	if model.MigrationMode() != state.MigrationModeImporting {
		release()
		return nil, nil, errors.New("migration mode for the model is not importing")
	}
	return model, release, nil
}

// Abort removes the specified model from the database. It is an error to
// attempt to Abort a model that has a migration mode other than importing.
func (api *API) Abort(ctx context.Context, args params.ModelArgs) error {
	model, releaseModel, err := api.getImportingModel(args.ModelTag)
	if err != nil {
		return errors.Errorf("cannot get model to abort: %w", err)
	}
	defer releaseModel()

	st, err := api.pool.Get(model.UUID())
	if err != nil {
		return errors.Errorf("cannot get model %q state to abort: %w", model.UUID(), err)
	}
	defer st.Release()
	return st.RemoveImportingModelDocs()
}

// Activate sets the migration mode of the model to "none", meaning it
// is ready for use. It also adds any required
// external controller records for those controllers hosting offers used
// by the model.
func (api *API) Activate(ctx context.Context, args params.ActivateModelArgs) error {
	model, release, err := api.getImportingModel(args.ModelTag)
	if err != nil {
		return errors.Errorf("cannot get model to activate: %w", err)
	}
	defer release()

	// Add any required external controller records if there are cross
	// model relations to the source controller that were local but
	// now need to be external after migration.
	if len(args.CrossModelUUIDs) > 0 {
		cTag, err := names.ParseControllerTag(args.ControllerTag)
		if err != nil {
			return errors.Errorf(
				"cannot parse controller tag when activating model %q: %w",
				model.UUID(),
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
				model.UUID(),
				err,
			)
		}
	}

	// Update the source controller attribute on remote applications
	// to allow external controller ref counts to function properly.
	remoteApps, err := commoncrossmodel.GetBackend(model.State()).AllRemoteApplications()
	if err != nil {
		return errors.Errorf("cannot get remote applications for model %q: %w", model.UUID(), err)
	}
	for _, app := range remoteApps {
		var sourceControllerUUID string
		extInfo, err := api.externalControllerService.ControllerForModel(ctx, app.SourceModel().Id())
		if err != nil && !errors.Is(err, coreerrors.NotFound) {
			return errors.Errorf(
				"cannot get controller information for remote application %q: %w",
				app.Name(),
				err,
			)
		}
		if err == nil {
			sourceControllerUUID = extInfo.ControllerUUID
		}
		if err := app.SetSourceController(sourceControllerUUID); err != nil {
			return errors.Errorf(
				"cannot update application %q source controller to %q: %w",
				app.Name(),
				sourceControllerUUID,
				err,
			)
		}
	}

	// TODO(fwereade) - need to validate binaries here.
	return model.SetMigrationMode(state.MigrationModeNone)
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
	model, release, err := api.getModel(args.ModelTag)
	if err != nil {
		return time.Time{}, errors.Errorf("cannot get model: %w", err)
	}
	defer release()

	// Look up the last line in the model log file and get the timestamp.
	modelLogFile := corelogger.ModelLogFile(api.logDir, corelogger.LoggerKey{
		ModelUUID:  model.UUID(),
		ModelName:  model.Name(),
		ModelOwner: model.Owner().Id(),
	})

	f, err := os.Open(modelLogFile)
	if err != nil && !os.IsNotExist(err) {
		return time.Time{}, errors.Errorf(
			"cannot open model %q log file %q: %w",
			model.UUID(),
			modelLogFile,
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
			"cannot interrogate model %q log file %q: %w",
			model.UUID(),
			modelLogFile,
			err,
		)
	}
	scanner := rscanner.NewScanner(f, fs.Size())

	var lastTimestamp time.Time
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		var err error
		lastTimestamp, err = logLineTimestamp(line)
		if err == nil {
			break
		}

	}
	return lastTimestamp, nil
}

func logLineTimestamp(line string) (time.Time, error) {
	parts := strings.SplitN(line, " ", 7)
	if len(parts) < 7 {
		return time.Time{}, errors.Errorf("invalid log line %q", line)
	}
	timeStr := parts[1] + " " + parts[2]
	timeStamp, err := time.Parse("2006-01-02 15:04:05", timeStr)
	if err != nil {
		return time.Time{}, errors.Errorf("invalid log timestamp %q: %w", timeStr, err)
	}
	return timeStamp, nil
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
