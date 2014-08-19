// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/common"
	"github.com/juju/juju/state/api/params"
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

// machineIsManual requests the IsManual flag of the given machine from the server.
func (st *State) machineIsManual(tag names.MachineTag) (bool, error) {
	var result params.IsManualResults
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	}
	err := st.facade.FacadeCall("IsManual", args, &result)
	if err != nil {
		return false, err
	}
	if n := len(result.Results); n != 1 {
		return false, fmt.Errorf("expected 1 result, got %d", n)
	}
	// TODO(mue) Add check of possible results error.
	return result.Results[0].IsManual, nil
}

// Machine provides access to methods of a state.Machine through the facade.
func (st *State) Machine(tag names.MachineTag) (*Machine, error) {
	life, err := st.machineLife(tag)
	if err != nil {
		return nil, err
	}
	isManual, err := st.machineIsManual(tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:      tag,
		life:     life,
		isManual: isManual,
		st:       st,
	}, nil
}
