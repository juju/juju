// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
)

var logger loggo.Logger

func init() {
	common.RegisterStandardFacade("MigrationTarget", 1, NewAPI)
}

// API implements the API required for the model migration
// master worker when communicating with the target controller.
type API struct {
	state      *state.State
	authorizer common.Authorizer
	resources  *common.Resources
}

// NewAPI returns a new API.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
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

func checkAuth(authorizer common.Authorizer, st *state.State) error {
	if !authorizer.AuthClient() {
		return errors.Trace(common.ErrPerm)
	}

	// Type assertion is fine because AuthClient is true.
	apiUser := authorizer.GetAuthTag().(names.UserTag)
	if isAdmin, err := st.IsControllerAdministrator(apiUser); err != nil {
		return errors.Trace(err)
	} else if !isAdmin {
		// The entire facade is only accessible to controller administrators.
		return errors.Trace(common.ErrPerm)
	}
	return nil
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
	return err
}
