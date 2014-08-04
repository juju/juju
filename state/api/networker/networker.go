// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
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
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return results.Results[0].Info, nil
}

// WatchInterfaces returns a NotifyWatcher that notifies of changes to network
// interfaces on the machine.
func (st *State) WatchInterfaces(machineTag string) (watcher.NotifyWatcher, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: machineTag}},
	}
	var results params.NotifyWatchResults
	err := st.facade.FacadeCall("WatchInterfaces", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		err = fmt.Errorf("expected one result, got %d", len(results.Results))
		return nil, err
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}
