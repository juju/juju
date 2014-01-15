// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

// State provides access to a worker's view of the state.
type State struct {
	caller common.Caller
}

// NewState returns a version of the state that provides functionality required by the worker.
func NewState(caller common.Caller) *State {
	return &State{caller}
}

// UpdateLatestRevisions retrieves charm revision info from a repository
// and updates the revision info in state.
func (st *State) UpdateLatestRevisions() error {
	result := new(params.ErrorResult)
	err := st.caller.Call("CharmRevisionUpdater", "", "UpdateLatestRevisions", nil, result)
	if err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}
