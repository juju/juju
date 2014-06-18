// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver/common"
)

func init() {
	common.RegisterStandardFacade("Environment", 0, NewEnvironmentAPI)
}

// EnvironmentAPI implements the API used by the machine environment worker.
type EnvironmentAPI struct {
	*common.EnvironWatcher
}

// NewEnvironmentAPI creates a new instance of the Environment API.
func NewEnvironmentAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*EnvironmentAPI, error) {
	// Can always watch for environ changes.
	getCanWatch := common.AuthAlways(true)
	// Does not get the secrets.
	getCanReadSecrets := common.AuthAlways(false)
	return &EnvironmentAPI{
		EnvironWatcher: common.NewEnvironWatcher(st, resources, getCanWatch, getCanReadSecrets),
	}, nil
}
