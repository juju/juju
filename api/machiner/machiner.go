// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
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

// machineLife requests the lifecycle of the given machine from the server.
func (st *State) machineLife(tag names.MachineTag) (params.Life, error) {
	return common.Life(st.facade, tag)
}

// Machine provides access to methods of a state.Machine through the facade.
func (st *State) Machine(tag names.MachineTag) (*Machine, error) {
	life, err := st.machineLife(tag)
	if err != nil {
		return nil, errors.Annotate(err, "can't get life for machine")
	}
	return &Machine{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}
