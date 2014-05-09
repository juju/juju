// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/tools"
)

// State provides access to the Machiner API facade.
type State struct {
	*common.EnvironWatcher
	*common.APIAddresser

	caller base.Caller
}

const provisionerFacade = "Provisioner"

// NewState creates a new client-side Machiner facade.
func NewState(caller base.Caller) *State {
	return &State{
		EnvironWatcher: common.NewEnvironWatcher(provisionerFacade, caller),
		APIAddresser:   common.NewAPIAddresser(provisionerFacade, caller),
		caller:         caller}
}

func (st *State) call(method string, params, result interface{}) error {
	return st.caller.Call(provisionerFacade, "", method, params, result)
}

// machineLife requests the lifecycle of the given machine from the server.
func (st *State) machineLife(tag string) (params.Life, error) {
	return common.Life(st.caller, provisionerFacade, tag)
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

// WatchEnvironMachines returns a StringsWatcher that notifies of
// changes to the lifecycles of the machines (but not containers) in
// the current environment.
func (st *State) WatchEnvironMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := st.call("WatchEnvironMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(st.caller, result)
	return w, nil
}

func (st *State) WatchMachineErrorRetry() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := st.call("WatchMachineErrorRetry", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(st.caller, result)
	return w, nil
}

// StateAddresses returns the list of addresses used to connect to the state.
func (st *State) StateAddresses() ([]string, error) {
	var result params.StringsResult
	err := st.call("StateAddresses", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

// Tools returns the agent tools for the given entity.
func (st *State) Tools(tag string) (*tools.Tools, error) {
	var results params.ToolsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.call("Tools", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// ContainerManagerConfig returns information from the environment config that is
// needed for configuring the container manager.
func (st *State) ContainerManagerConfig(args params.ContainerManagerConfigParams) (result params.ContainerManagerConfig, err error) {
	err = st.call("ContainerManagerConfig", args, &result)
	return result, err
}

// ContainerConfig returns information from the environment config that is
// needed for container cloud-init.
func (st *State) ContainerConfig() (result params.ContainerConfig, err error) {
	err = st.call("ContainerConfig", nil, &result)
	return result, err
}

// MachinesWithTransientErrors returns a slice of machines and corresponding status information
// for those machines which have transient provisioning errors.
func (st *State) MachinesWithTransientErrors() ([]*Machine, []params.StatusResult, error) {
	var results params.StatusResults
	err := st.call("MachinesWithTransientErrors", nil, &results)
	if err != nil {
		return nil, nil, err
	}
	machines := make([]*Machine, len(results.Results))
	for i, status := range results.Results {
		if status.Error != nil {
			continue
		}
		machines[i] = &Machine{
			tag:  names.MachineTag(status.Id),
			life: status.Life,
			st:   st,
		}
	}
	return machines, results.Results, nil
}
