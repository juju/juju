// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The resumer package implements the API interface
// used by the resumer worker.
package resumer

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Resumer", 1, NewResumerAPI)
}

var logger = loggo.GetLogger("juju.apiserver.resumer")

// ResumerAPI implements the API used by the resumer worker.
type ResumerAPI struct {
	st   stateInterface
	auth common.Authorizer
}

// NewResumerAPI creates a new instance of the Resumer API.
func NewResumerAPI(st *state.State, _ *common.Resources, authorizer common.Authorizer) (*ResumerAPI, error) {
	if !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	return &ResumerAPI{
		st:   getState(st),
		auth: authorizer,
	}, nil
}

func (api *ResumerAPI) ResumeTransactions() error {
	return api.st.ResumeTransactions()
}
