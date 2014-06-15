// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/params"
)

// State provides access to a worker's view of the state.
type State struct {
	base.FacadeCaller
}

// NewState returns a version of the state that provides functionality required by the worker.
func NewState(caller base.Caller) *State {
	return &State{base.GetFacadeCaller(caller, "CharmRevisionUpdater")}
}

// UpdateLatestRevisions retrieves charm revision info from a repository
// and updates the revision info in state.
func (st *State) UpdateLatestRevisions() error {
	result := new(params.ErrorResult)
	err := st.APICall("UpdateLatestRevisions", nil, result)
	if err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}
