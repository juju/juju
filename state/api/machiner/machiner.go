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

// Machine provides access to methods of a state.Machine through the facade.
func (st *State) Machine(tag names.MachineTag) (*Machine, error) {
	var result params.GetMachinesResultsV0
	args := params.GetMachinesV0{[]string{tag.String()}}
	err := st.facade.FacadeCall("GetMachines", args, &result)
	if err != nil {
		return nil, err
	}
	if n := len(result.Machines); n != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", n)
	}
	machineResult := result.Machines[0]
	return &Machine{
		tag:      tag,
		life:     machineResult.Life,
		isManual: machineResult.IsManual,
		st:       st,
	}, nil
}
