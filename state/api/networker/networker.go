// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/params"
)

const networkerFacade = "Networker"

// State provides access to an networker worker's view of the state.
type State struct {
	facade base.FacadeCaller
}

// NewState creates a new client-side Machiner facade.
func NewState(caller base.APICaller) *State {
	return &State{base.NewFacadeCaller(caller, networkerFacade)}
}

// MachineNetworkInfo returns information about networks to setup only for a single machine.
func (st *State) MachineNetworkInfo(machineTag string) ([]network.Info, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: machineTag}},
	}
	var results params.MachineNetworkInfoResults
	err := st.facade.FacadeCall("MachineNetworkInfo", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		err = fmt.Errorf("expected one result, got %d", len(results.Results))
		return nil, err
	}
	return results.Results[0].Info, results.Results[0].Error
}
