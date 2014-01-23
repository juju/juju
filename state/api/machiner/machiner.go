// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

// State provides access to the Machiner API facade.
type State struct {
	caller base.Caller
}

// NewState creates a new client-side Machiner facade.
func NewState(caller base.Caller) *State {
	return &State{caller}
}

// machineLife requests the lifecycle of the given machine from the server.
func (st *State) machineLife(tag string) (params.Life, error) {
	return common.Life(st.caller, "Machiner", tag)
}

// Machine provides access to methods of a state.Machine through the facade.
func (st *State) Machine(tag string) (*Machine, error) {
	life, err := st.machineLife(tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}
