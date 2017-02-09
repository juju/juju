// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"time"

	"github.com/juju/errors"
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
)

func init() {
	common.RegisterStandardFacade("MigrationTarget", 1, newAPIWithRealEnviron)
}

// API implements the API required for the model migration
// master worker when communicating with the target controller.
type API struct {
	state      *state.State
	authorizer facade.Authorizer
	resources  facade.Resources
	pool       *state.StatePool
	getEnviron stateenvirons.NewEnvironFunc
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
		authorizer: auth,
		resources:  ctx.Resources(),
		pool:       ctx.StatePool(),
		getEnviron: getEnviron,
	}, nil
}

// newAPIWithRealEnviron creates an API with a real environ factory
// function.
func newAPIWithRealEnviron(ctx facade.Context) (*API, error) {
	return NewAPI(ctx, stateenvirons.GetNewEnvironFunc(environs.New))
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
	backend, err := migration.PrecheckShim(api.state)
	if err != nil {
		return errors.Annotate(err, "creating backend")
	}
	return migration.TargetPrecheck(
		backend,
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

func (api *API) getModel(modelTag string) (*state.Model, error) {
	tag, err := names.ParseModelTag(modelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := api.state.GetModel(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return model, nil
}

func (api *API) getImportingModel(args params.ModelArgs) (*state.Model, error) {
	model, err := api.getModel(args.ModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.MigrationMode() != state.MigrationModeImporting {
		return nil, errors.New("migration mode for the model is not importing")
	}
	return model, nil
}

// Abort removes the specified model from the database. It is an error to
// attempt to Abort a model that has a migration mode other than importing.
func (api *API) Abort(args params.ModelArgs) error {
	model, err := api.getImportingModel(args)
	if err != nil {
		return errors.Trace(err)
	}

	st, err := api.state.ForModel(model.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Close()

	return st.RemoveImportingModelDocs()
}

// Activate sets the migration mode of the model to "none", meaning it
// is ready for use. It is an error to attempt to Abort a model that
// has a migration mode other than importing.
func (api *API) Activate(args params.ModelArgs) error {
	model, err := api.getImportingModel(args)
	if err != nil {
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
	model, err := api.getModel(args.ModelTag)
	if err != nil {
		return time.Time{}, errors.Trace(err)
	}
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
	model, err := api.getModel(args.ModelTag)
	if err != nil {
		return errors.Trace(err)
	}
	st, release, err := api.pool.Get(model.UUID())
	if err != nil {
		return errors.Trace(err)
	}
	defer release()
	env, err := api.getEnviron(st)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(env.AdoptResources(model.ControllerUUID(), args.SourceControllerVersion))
}
