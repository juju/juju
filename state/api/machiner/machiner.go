// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"github.com/juju/names"

	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/common"
	"github.com/juju/juju/state/api/params"
)

const machinerFacade = "Machiner"

// State provides access to the Machiner API facade.
type State struct {
	caller base.Caller
	*common.APIAddresser
}

func (st *State) call(method string, params, result interface{}) error {
	return st.caller.Call(machinerFacade, "", method, params, result)
}

// NewState creates a new client-side Machiner facade.
func NewState(caller base.Caller) *State {
	return &State{
		caller:       caller,
		APIAddresser: common.NewAPIAddresser(machinerFacade, caller),
	}

}

// machineLife requests the lifecycle of the given machine from the server.
func (st *State) machineLife(tag names.Tag) (params.Life, error) {
	return common.Life(st.caller, machinerFacade, tag)
}

// Machine provides access to methods of a state.Machine through the facade.
func (st *State) Machine(machineTag string) (*Machine, error) {
	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return nil, err
	}
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
