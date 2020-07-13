// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The resumer package implements the API interface
// used by the resumer worker.
package resumer

import (
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// ResumerAPI implements the API used by the resumer worker.
type ResumerAPI struct {
	st   stateInterface
	auth facade.Authorizer
}

// NewResumerAPI creates a new instance of the Resumer API.
func NewResumerAPI(st *state.State, _ facade.Resources, authorizer facade.Authorizer) (*ResumerAPI, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &ResumerAPI{
		st:   getState(st),
		auth: authorizer,
	}, nil
}

func (api *ResumerAPI) ResumeTransactions() error {
	return api.st.ResumeTransactions()
}
