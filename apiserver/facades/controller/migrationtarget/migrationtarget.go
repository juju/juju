// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"context"
	"fmt"
	coreuser "github.com/juju/juju/core/user"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/facades"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/credential"
	credentialservice "github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/environs"
	environscontext "github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

type ModelImporter interface {
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

// UserService provides a subset of the user domain service methods.
type UserService interface {
	GetAllUsers(ctx context.Context) ([]coreuser.User, error)
	GetUserByName(ctx context.Context, name string) (coreuser.User, error)
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
	ctx facade.Context,
	authorizer facade.Authorizer,
	controllerConfigService ControllerConfigService,
	externalControllerService ExternalControllerService,
	upgradeService UpgradeService,
	cloudService common.CloudService,
	credentialService credentialcommon.CredentialService,
	userService UserService,
	validator credentialservice.CredentialValidator,
	credentialCallContextGetter credentialservice.ValidationContextGetter,
	credentialInvalidatorGetter environscontext.ModelCredentialInvalidatorGetter,
	getEnviron stateenvirons.NewEnvironFunc,
	getCAASBroker stateenvirons.NewCAASBrokerFunc,
	requiredMigrationFacadeVersions facades.FacadeVersions,
) (*API, error) {
	var (
		st   = ctx.State()
		pool = ctx.StatePool()

		scope         = modelmigration.NewScope(changestream.NewTxnRunnerFactory(ctx.ControllerDB), nil)
		controller    = state.NewController(pool)
		modelImporter = migration.NewModelImporter(controller, scope, controllerConfigService, userService)
	)

	return &API{
		state:                           st,
		modelImporter:                   modelImporter,
		pool:                            pool,
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

	if err := model.SetStatus(status.StatusInfo{Status: status.Available}); err != nil {
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

	if err := model.SetStatus(status.StatusInfo{Status: status.Available}); err != nil {
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
// For performance reasons, not every time is tracked, so if the
// target controller died during the transfer the latest log time
// might be up to 2 minutes earlier. If the transfer was interrupted
// in some other way (like the source controller going away or a
// network partition) the time will be up-to-date.
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

	tracker := state.NewLastSentLogTracker(api.state, model.UUID(), "migration-logtransfer")
	defer tracker.Close()
	_, timestamp, err := tracker.Get()
	if errors.Cause(err) == state.ErrNeverForwarded {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, errors.Trace(err)
	}
	return time.Unix(0, timestamp).In(time.UTC), nil
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
	cloud, err := api.cloudService.Get(ctx, m.CloudName())
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	credentialTag, isSet := m.CloudCredentialTag()
	if !isSet || credentialTag.IsZero() {
		return params.ErrorResults{}, nil
	}

	storedCredential, err := api.credentialService.CloudCredential(ctx, credential.IdFromTag(credentialTag))
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if storedCredential.Invalid {
		return params.ErrorResults{}, errors.NotValidf("credential %q", storedCredential.Label)
	}

	callCtx, err := api.credentialCallContextGetter(ctx, model.UUID(m.UUID()))
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	cred := jujucloud.NewCredential(storedCredential.AuthType(), storedCredential.Attributes())

	var result params.ErrorResults
	modelErrors, err := api.credentialValidator.Validate(ctx, callCtx, credential.IdFromTag(credentialTag), &cred, cloud.Type != "manual")
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
