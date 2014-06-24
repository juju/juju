// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"github.com/juju/names"

	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/common"
)

const machinerFacade = "Machiner"

// State provides access to the Machiner API facade.
type State struct {
	facade base.FacadeCaller
	*common.APIAddresser
}

// NewState creates a new client-side Machiner facade.
func NewState(caller base.APICaller) *State {
	facadeCaller := base.NewFacadeCaller(caller, machinerFacade)
	return &State{
		facade:       facadeCaller,
		APIAddresser: common.NewAPIAddresser(facadeCaller),
	}

}

// Machine provides access to methods of a state.Machine through the facade.
func (st *State) Machine(machineTag string) (*Machine, error) {
	life, err := common.Life(st.facade, machineTag)
	if err != nil {
		return nil, err
	}
	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}
