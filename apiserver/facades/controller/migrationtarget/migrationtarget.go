// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/vallerion/rscanner"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/facades"
	corelogger "github.com/juju/juju/core/logger"
	coremigration "github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	credentialservice "github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/environs"
	environscontext "github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
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

// ModelManagerService describes the method needed to update model metadata.
type ModelManagerService interface {
	Create(context.Context, coremodel.UUID) error
}

// UpgradeService provides a subset of the upgrade domain service methods.
type UpgradeService interface {
	// IsUpgrading returns whether the controller is currently upgrading.
	IsUpgrading(context.Context) (bool, error)
}

// API implements the API required for the model migration
// master worker when communicating with the target controller.
type API struct {
	state                           *state.State
	modelImporter                   ModelImporter
	externalControllerService       ExternalControllerService
	cloudService                    common.CloudService
	upgradeService                  UpgradeService
	credentialService               credentialcommon.CredentialService
	credentialValidator             credentialservice.CredentialValidator
	credentialCallContextGetter     credentialservice.ValidationContextGetter
	credentialInvalidatorGetter     environscontext.ModelCredentialInvalidatorGetter
	pool                            *state.StatePool
	authorizer                      facade.Authorizer
	presence                        facade.Presence
	getEnviron                      stateenvirons.NewEnvironFunc
	getCAASBroker                   stateenvirons.NewCAASBrokerFunc
	requiredMigrationFacadeVersions facades.FacadeVersions

	logDir string
}

// APIV1 implements the V1 version of the API facade.
type APIV1 struct {
	*API
}

// APIV2 implements the V2 version of the API facade.
type APIV2 struct {
	*APIV1
}

// NewAPI returns a new APIV1. Accepts a NewEnvironFunc and envcontext.ProviderCallContext
// for testing purposes.
func NewAPI(
	ctx facade.ModelContext,
	authorizer facade.Authorizer,
	controllerConfigService ControllerConfigService,
	externalControllerService ExternalControllerService,
	upgradeService UpgradeService,
	cloudService common.CloudService,
	credentialService credentialcommon.CredentialService,
	validator credentialservice.CredentialValidator,
	credentialCallContextGetter credentialservice.ValidationContextGetter,
	credentialInvalidatorGetter environscontext.ModelCredentialInvalidatorGetter,
	getEnviron stateenvirons.NewEnvironFunc,
	getCAASBroker stateenvirons.NewCAASBrokerFunc,
	requiredMigrationFacadeVersions facades.FacadeVersions,
	logDir string,
) (*API, error) {
	return &API{
		state:                           ctx.State(),
		modelImporter:                   ctx.ModelImporter(),
		pool:                            ctx.StatePool(),
		externalControllerService:       externalControllerService,
		cloudService:                    cloudService,
		upgradeService:                  upgradeService,
		credentialService:               credentialService,
		credentialValidator:             validator,
		credentialCallContextGetter:     credentialCallContextGetter,
		credentialInvalidatorGetter:     credentialInvalidatorGetter,
		authorizer:                      authorizer,
		presence:                        ctx.Presence(),
		getEnviron:                      getEnviron,
		getCAASBroker:                   getCAASBroker,
		requiredMigrationFacadeVersions: requiredMigrationFacadeVersions,
		logDir:                          logDir,
	}, nil
}

func checkAuth(authorizer facade.Authorizer, st *state.State) error {
	if !authorizer.AuthClient() {
		return errors.Trace(apiservererrors.ErrPerm)
	}

	return authorizer.HasPermission(permission.SuperuserAccess, st.ControllerTag())
}

// Prechecks ensure that the target controller is ready to accept a
// model migration.
func (api *API) Prechecks(ctx context.Context, model params.MigrationModelInfo) error {
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

	ownerTag, err := names.ParseUserTag(model.OwnerTag)
	if err != nil {
		return errors.Trace(err)
	}
	controllerState, err := api.pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	// NOTE (thumper): it isn't clear to me why api.state would be different
	// from the controllerState as I had thought that the Precheck call was
	// on the controller model, in which case it should be the same as the
	// controllerState.
	backend, err := migration.PrecheckShim(api.state, controllerState)
	if err != nil {
		return errors.Annotate(err, "creating backend")
	}
	return migration.TargetPrecheck(
		ctx,
		backend,
		migration.PoolShim(api.pool),
		coremigration.ModelInfo{
			UUID:                   model.UUID,
			Name:                   model.Name,
			Owner:                  ownerTag,
			AgentVersion:           model.AgentVersion,
			ControllerAgentVersion: model.ControllerAgentVersion,
		},
		api.presence.ModelPresence(controllerState.ModelUUID()),
		api.upgradeService,
	)
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
		return nil, nil, errors.Trace(err)
	}
	model, ph, err := api.pool.GetModel(tag.Id())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return model, func() { ph.Release() }, nil
}

func (api *API) getImportingModel(tag string) (*state.Model, func(), error) {
	model, release, err := api.getModel(tag)
	if err != nil {
		return nil, nil, errors.Trace(err)
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
		return errors.Trace(err)
	}
	defer releaseModel()

	st, err := api.pool.Get(model.UUID())
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()
	return st.RemoveImportingModelDocs()
}

// Activate sets the migration mode of the model to "none", meaning it
// is ready for use. It is an error to attempt to Abort a model that
// has a migration mode other than importing.
func (api *APIV1) Activate(ctx context.Context, args params.ModelArgs) error {
	model, release, err := api.getImportingModel(args.ModelTag)
	if err != nil {
		return errors.Trace(err)
	}
	defer release()

	if err := model.SetStatus(status.StatusInfo{Status: status.Available}, nil); err != nil {
		return errors.Trace(err)
	}

	// TODO(fwereade) - need to validate binaries here.
	return model.SetMigrationMode(state.MigrationModeNone)
}

// Activate sets the migration mode of the model to "none", meaning it
// is ready for use. It is an error to attempt to Abort a model that
// has a migration mode other than importing. It also adds any required
// external controller records for those controllers hosting offers used
// by the model.
func (api *API) Activate(ctx context.Context, args params.ActivateModelArgs) error {
	model, release, err := api.getImportingModel(args.ModelTag)
	if err != nil {
		return errors.Trace(err)
	}
	defer release()

	// Add any required external controller records if there are cross
	// model relations to the source controller that were local but
	// now need to be external after migration.
	if len(args.CrossModelUUIDs) > 0 {
		cTag, err := names.ParseControllerTag(args.ControllerTag)
		if err != nil {
			return errors.Trace(err)
		}
		err = api.externalControllerService.UpdateExternalController(ctx, crossmodel.ControllerInfo{
			ControllerTag: cTag,
			Alias:         args.ControllerAlias,
			Addrs:         args.SourceAPIAddrs,
			CACert:        args.SourceCACert,
			ModelUUIDs:    args.CrossModelUUIDs,
		})
		if err != nil {
			return errors.Annotate(err, "saving source controller info")
		}
	}

	// Update the source controller attribute on remote applications
	// to allow external controller ref counts to function properly.
	remoteApps, err := model.State().AllRemoteApplications()
	if err != nil {
		return errors.Trace(err)
	}
	for _, app := range remoteApps {
		var sourceControllerUUID string
		extInfo, err := api.externalControllerService.ControllerForModel(ctx, app.SourceModel().Id())
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
		if err == nil {
			sourceControllerUUID = extInfo.ControllerTag.Id()
		}
		if err := app.SetSourceController(sourceControllerUUID); err != nil {
			return errors.Annotatef(err, "updating source controller uuid for %q", app.Name())
		}
	}

	if err := model.SetStatus(status.StatusInfo{Status: status.Available}, nil); err != nil {
		return errors.Trace(err)
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
		return time.Time{}, errors.Trace(err)
	}
	defer release()

	// Look up the last line in the model log file and get the timestamp.
	modelOwnerAndName := corelogger.ModelFilePrefix(model.Owner().Id(), model.Name())
	modelLogFile := corelogger.ModelLogFile(api.logDir, model.UUID(), modelOwnerAndName)

	f, err := os.Open(modelLogFile)
	if err != nil && !os.IsNotExist(err) {
		return time.Time{}, errors.Annotatef(err, "opening file %q", modelLogFile)
	} else if err != nil {
		return time.Time{}, nil
	}
	defer func() {
		_ = f.Close()
	}()

	fs, err := f.Stat()
	if err != nil {
		return time.Time{}, errors.Trace(err)
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
		return time.Time{}, errors.Annotatef(err, "invalid log timestamp %q", timeStr)
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
		return errors.Trace(err)
	}
	st, err := api.pool.Get(tag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()

	m, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	var ra environs.ResourceAdopter
	if m.Type() == state.ModelTypeCAAS {
		ra, err = api.getCAASBroker(m, api.cloudService, api.credentialService)
	} else {
		ra, err = api.getEnviron(m, api.cloudService, api.credentialService)
	}
	if err != nil {
		return errors.Trace(err)
	}

	invalidatorFunc, err := api.credentialInvalidatorGetter()
	if err != nil {
		return errors.Trace(err)
	}
	callCtx := environscontext.WithCredentialInvalidator(ctx, invalidatorFunc)
	err = ra.AdoptResources(callCtx, st.ControllerUUID(), args.SourceControllerVersion)
	if errors.Is(err, errors.NotImplemented) {
		return nil
	}
	return errors.Trace(err)
}

// CheckMachines compares the machines in state with the ones reported
// by the provider and reports any discrepancies.
func (api *API) CheckMachines(ctx context.Context, args params.ModelArgs) (params.ErrorResults, error) {
	tag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	st, err := api.pool.Get(tag.Id())
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	defer st.Release()

	// We don't want to check existing cloud instances for "manual" clouds.
	m, err := st.Model()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	cloud, err := api.cloudService.Cloud(ctx, m.CloudName())
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	credentialTag, isSet := m.CloudCredentialTag()
	if !isSet || credentialTag.IsZero() {
		return params.ErrorResults{}, nil
	}

	storedCredential, err := api.credentialService.CloudCredential(ctx, credential.KeyFromTag(credentialTag))
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if storedCredential.Invalid {
		return params.ErrorResults{}, errors.NotValidf("credential %q", storedCredential.Label)
	}

	callCtx, err := api.credentialCallContextGetter(ctx, coremodel.UUID(m.UUID()))
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	cred := jujucloud.NewCredential(storedCredential.AuthType(), storedCredential.Attributes())

	var result params.ErrorResults
	modelErrors, err := api.credentialValidator.Validate(ctx, callCtx, credential.KeyFromTag(credentialTag), &cred, cloud.Type != "manual")
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	result.Results = make([]params.ErrorResult, len(modelErrors))
	for i, err := range modelErrors {
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// CACert returns the certificate used to validate the state connection.
func (api *API) CACert(ctx context.Context) (params.BytesResult, error) {
	cfg, err := api.state.ControllerConfig()
	if err != nil {
		return params.BytesResult{}, errors.Trace(err)
	}
	caCert, _ := cfg.CACert()
	return params.BytesResult{Result: []byte(caCert)}, nil
}
