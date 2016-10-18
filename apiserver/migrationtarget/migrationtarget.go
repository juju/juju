// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("MigrationTarget", 1, NewAPI)
}

// API implements the API required for the model migration
// master worker when communicating with the target controller.
type API struct {
	state      *state.State
	authorizer facade.Authorizer
	resources  facade.Resources
}

// NewAPI returns a new API.
func NewAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	if err := checkAuth(authorizer, st); err != nil {
		return nil, errors.Trace(err)
	}
	return &API{
		state:      st,
		authorizer: authorizer,
		resources:  resources,
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
	return migration.TargetPrecheck(
		migration.PrecheckShim(api.state),
		coremigration.ModelInfo{
			UUID:         model.UUID,
			Name:         model.Name,
			Owner:        ownerTag,
			AgentVersion: model.AgentVersion,
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

func (api *API) getModel(args params.ModelArgs) (*state.Model, error) {
	tag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := api.state.GetModel(tag)
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
	model, err := api.getModel(args)
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

// Activate sets the migration mode of the model to "active". It is an error to
// attempt to Abort a model that has a migration mode other than importing.
func (api *API) Activate(args params.ModelArgs) error {
	model, err := api.getModel(args)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(fwereade) - need to validate binaries here.
	return model.SetMigrationMode(state.MigrationModeNone)
}
