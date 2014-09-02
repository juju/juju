// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// State provides access to a worker's view of the state.
type State struct {
	facade base.FacadeCaller
}

// NewState returns a version of the state that provides functionality required by the worker.
func NewState(caller base.APICaller) *State {
	return &State{base.NewFacadeCaller(caller, "CharmRevisionUpdater")}
}

// UpdateLatestRevisions retrieves charm revision info from a repository
// and updates the revision info in state.
func (st *State) UpdateLatestRevisions() error {
	result := new(params.ErrorResult)
	err := st.facade.FacadeCall("UpdateLatestRevisions", nil, result)
	if err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}
