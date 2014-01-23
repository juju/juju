// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"

	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/tools"
)

const provisioner = "Provisioner"

// State provides access to the Machiner API facade.
type State struct {
	*common.EnvironWatcher

	caller base.Caller
}

// NewState creates a new client-side Machiner facade.
func NewState(caller base.Caller) *State {
	return &State{
		EnvironWatcher: common.NewEnvironWatcher(provisioner, caller),

		caller: caller}
}

// machineLife requests the lifecycle of the given machine from the server.
func (st *State) machineLife(tag string) (params.Life, error) {
	return common.Life(st.caller, provisioner, tag)
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
	err := st.caller.Call(provisioner, "", "WatchEnvironMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(st.caller, result)
	return w, nil
}

// StateAddresses returns the list of addresses used to connect to the state.
func (st *State) StateAddresses() ([]string, error) {
	var result params.StringsResult
	err := st.caller.Call(provisioner, "", "StateAddresses", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

// APIAddresses returns the list of addresses used to connect to the API.
func (st *State) APIAddresses() ([]string, error) {
	var result params.StringsResult
	err := st.caller.Call(provisioner, "", "APIAddresses", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

// CACert returns the certificate used to validate the state connection.
func (st *State) CACert() ([]byte, error) {
	var result params.BytesResult
	err := st.caller.Call(provisioner, "", "CACert", nil, &result)
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
	err := st.caller.Call(provisioner, "", "Tools", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// ContainerConfig returns information from the environment config that are
// needed for container cloud-init.
func (st *State) ContainerConfig() (result params.ContainerConfig, err error) {
	err = st.caller.Call(provisioner, "", "ContainerConfig", nil, &result)
	return result, err
}
