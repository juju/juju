// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
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
	var result params.GetMachinesResultsV1
	args := params.GetMachinesV1{[]string{tag.String()}}
	err := st.facade.FacadeCall("GetMachines", args, &result)
	if err != nil {
		if isCallNotImplementedError(err) {
			// Version is lower than expected.
			life, err := st.machineLife(tag)
			if err != nil {
				return nil, err
			}
			// TODO(mue) Retrieve "isManual" in a V0 compatible way.
			return &Machine{
				tag:      tag,
				life:     life,
				isManual: false,
				st:       st,
			}, nil
		}
		return nil, err
	}
	// Server runs V1 or higher.
	if n := len(result.Machines); n != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", n)
	}
	machineResult := result.Machines[0]
	if machineResult.Error != nil {
		return nil, machineResult.Error
	}
	return &Machine{
		tag:      tag,
		life:     machineResult.Life,
		isManual: machineResult.IsManual,
		st:       st,
	}, nil
}

// isCallNotImplementedError checks if a returned error shows, that
// the performed facade call is not implemented on the server.
func isCallNotImplementedError(err error) bool {
	perr, ok := err.(*params.Error)
	if !ok {
		return false
	}
	return perr.Code == rpc.CodeNotImplemented
}
