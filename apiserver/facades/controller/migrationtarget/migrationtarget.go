// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/status"
)

// API implements the API required for the model migration
// master worker when communicating with the target controller.
type API struct {
	state      *state.State
	pool       *state.StatePool
	authorizer facade.Authorizer
	resources  facade.Resources
	getEnviron stateenvirons.NewEnvironFunc
}

// NewFacade is used for API registration.
func NewFacade(ctx facade.Context) (*API, error) {
	return NewAPI(ctx, stateenvirons.GetNewEnvironFunc(environs.New))
}

// NewAPI returns a new API. Accepts a NewEnvironFunc for testing
// purposes.
func NewAPI(ctx facade.Context, getEnviron stateenvirons.NewEnvironFunc) (*API, error) {
	auth := ctx.Auth()
	st := ctx.State()
	if err := checkAuth(auth, st); err != nil {
		return nil, errors.Trace(err)
	}
	return &API{
		state:      st,
		pool:       ctx.StatePool(),
		authorizer: auth,
		resources:  ctx.Resources(),
		getEnviron: getEnviron,
	}, nil
}

func checkAuth(authorizer facade.Authorizer, st *state.State) error {
	if !authorizer.AuthClient() {
		return errors.Trace(common.ErrPerm)
	}

	if isAdmin, err := authorizer.HasPermission(permission.SuperuserAccess, st.ControllerTag()); err != nil {
		return errors.Trace(err)
	} else if !isAdmin {
		// The entire facade is only accessible to controller administrators.
		return errors.Trace(common.ErrPerm)
	}
	return nil
}

// Prechecks ensure that the target controller is ready to accept a
// model migration.
func (api *API) Prechecks(model params.MigrationModelInfo) error {
	ownerTag, err := names.ParseUserTag(model.OwnerTag)
	if err != nil {
		return errors.Trace(err)
	}
	backend, err := migration.PrecheckShim(api.state, api.pool.SystemState())
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
	)
}

// Import takes a serialized Juju model, deserializes it, and
// recreates it in the receiving controller.
func (api *API) Import(serialized params.SerializedModel) error {
	_, st, err := migration.ImportModel(api.state, serialized.Bytes)
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

func (api *API) getImportingModel(args params.ModelArgs) (*state.Model, func(), error) {
	model, release, err := api.getModel(args.ModelTag)
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
	model, releaseModel, err := api.getImportingModel(args)
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
func (api *API) Activate(args params.ModelArgs) error {
	model, release, err := api.getImportingModel(args)
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
	env, err := api.getEnviron(st.State)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(env.AdoptResources(st.ControllerUUID(), args.SourceControllerVersion))
}

// CheckMachines compares the machines in state with the ones reported
// by the provider and reports any discrepancies.
func (api *API) CheckMachines(args params.ModelArgs) (params.ErrorResults, error) {
	var empty params.ErrorResults
	tag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return empty, errors.Trace(err)
	}
	st, err := api.pool.Get(tag.Id())
	if err != nil {
		return empty, errors.Trace(err)
	}
	defer st.Release()

	machines, err := st.AllMachines()
	if err != nil {
		return empty, errors.Trace(err)
	}
	machinesByInstance := make(map[string]string)
	for _, machine := range machines {
		if machine.IsContainer() {
			// Containers don't correspond to instances at the
			// provider level.
			continue
		}
		if manual, err := machine.IsManual(); err != nil {
			return empty, errors.Trace(err)
		} else if manual {
			continue
		}
		instanceId, err := machine.InstanceId()
		if err != nil {
			return empty, errors.Annotatef(
				err, "getting instance id for machine %s", machine.Id())
		}
		machinesByInstance[string(instanceId)] = machine.Id()
	}

	env, err := api.getEnviron(st.State)
	if err != nil {
		return empty, errors.Trace(err)
	}
	instances, err := env.AllInstances()
	if err != nil {
		return empty, errors.Trace(err)
	}

	var results []params.ErrorResult

	instanceIds := set.NewStrings()
	for _, instance := range instances {
		id := string(instance.Id())
		instanceIds.Add(id)
		if _, found := machinesByInstance[id]; !found {
			results = append(results, errorResult("no machine with instance %q", id))
		}
	}

	for instanceId, name := range machinesByInstance {
		if !instanceIds.Contains(instanceId) {
			results = append(results, errorResult(
				"couldn't find instance %q for machine %s", instanceId, name))
		}
	}

	return params.ErrorResults{Results: results}, nil
}

func errorResult(format string, args ...interface{}) params.ErrorResult {
	return params.ErrorResult{Error: common.ServerError(errors.Errorf(format, args...))}
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
