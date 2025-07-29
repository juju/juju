// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/facades"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// API implements the API required for the model migration
// master worker when communicating with the target controller.
type API struct {
	state                           *state.State
	pool                            *state.StatePool
	authorizer                      facade.Authorizer
	resources                       facade.Resources
	presence                        facade.Presence
	getClaimer                      migration.ClaimerFunc
	getEnviron                      stateenvirons.NewEnvironFunc
	CAASbrokerProvider              stateenvirons.NewCAASBrokerFunc
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

// NewAPI returns a new APIV1. Accepts a NewEnvironFunc and context.ProviderCallContext
// for testing purposes.
func NewAPI(
	ctx facade.Context,
	getEnviron stateenvirons.NewEnvironFunc,
	CAASbrokerProvider stateenvirons.NewCAASBrokerFunc,
	requiredMigrationFacadeVersions facades.FacadeVersions,
) (*API, error) {
	auth := ctx.Auth()
	st := ctx.State()
	if err := checkAuth(auth, st); err != nil {
		return nil, errors.Trace(err)
	}
	return &API{
		state:                           st,
		pool:                            ctx.StatePool(),
		authorizer:                      auth,
		resources:                       ctx.Resources(),
		presence:                        ctx.Presence(),
		getClaimer:                      ctx.LeadershipClaimer,
		getEnviron:                      getEnviron,
		CAASbrokerProvider:              CAASbrokerProvider,
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
func (api *API) Prechecks(model params.MigrationModelInfo) error {
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
	)
}

// Import takes a serialized Juju model, deserializes it, and
// recreates it in the receiving controller.
func (api *API) Import(serialized params.SerializedModel) error {
	controller := state.NewController(api.pool)
	_, st, err := migration.ImportModel(controller, api.getClaimer, serialized.Bytes)
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
func (api *API) Abort(args params.ModelArgs) error {
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
func (api *APIV1) Activate(args params.ModelArgs) error {
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
func (api *API) Activate(args params.ActivateModelArgs) error {
	model, release, err := api.getImportingModel(args.ModelTag)
	if err != nil {
		return errors.Trace(err)
	}
	defer release()

	// Add any required external controller records if there are cross
	// model relations to the source controller that were local but
	// now need to be external after migration.
	ec := api.state.NewExternalControllers()
	if len(args.CrossModelUUIDs) > 0 {
		cTag, err := names.ParseControllerTag(args.ControllerTag)
		if err != nil {
			return errors.Trace(err)
		}
		_, err = ec.Save(crossmodel.ControllerInfo{
			ControllerTag: cTag,
			Alias:         args.ControllerAlias,
			Addrs:         args.SourceAPIAddrs,
			CACert:        args.SourceCACert,
		}, args.CrossModelUUIDs...)
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
		extInfo, err := ec.ControllerForModel(app.SourceModel().Id())
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
		if err == nil {
			sourceControllerUUID = extInfo.ControllerInfo().ControllerTag.Id()
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
func (api *API) LatestLogTime(args params.ModelArgs) (time.Time, error) {
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
func (api *API) AdoptResources(args params.AdoptResourcesArgs) error {
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
		ra, err = api.CAASbrokerProvider(m)
	} else {
		ra, err = api.getEnviron(m)
	}
	if err != nil {
		return errors.Trace(err)
	}

	err = ra.AdoptResources(context.CallContext(m.State()), st.ControllerUUID(), args.SourceControllerVersion)
	if errors.IsNotImplemented(err) {
		return nil
	}
	return errors.Trace(err)
}

// CheckMachines compares the machines in state with the ones reported
// by the provider and reports any discrepancies.
func (api *API) CheckMachines(args params.ModelArgs) (params.ErrorResults, error) {
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
	model, err := st.Model()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	cloud, err := model.Cloud()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return credentialcommon.ValidateExistingModelCredential(
		credentialcommon.NewPersistentBackend(st.State),
		context.CallContext(st.State),
		cloud.Type != "manual",
		true,
	)
}

// CACert returns the certificate used to validate the state connection.
func (api *API) CACert() (params.BytesResult, error) {
	cfg, err := api.state.ControllerConfig()
	if err != nil {
		return params.BytesResult{}, errors.Trace(err)
	}
	caCert, _ := cfg.CACert()
	return params.BytesResult{Result: []byte(caCert)}, nil
}
