// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// State provides access to the Machiner API facade.
type State struct {
	*common.EnvironWatcher
	*common.APIAddresser

	facade base.FacadeCaller
}

const provisionerFacade = "Provisioner"

// NewState creates a new client-side Machiner facade.
func NewState(caller base.APICaller) *State {
	facadeCaller := base.NewFacadeCaller(caller, provisionerFacade)
	return &State{
		EnvironWatcher: common.NewEnvironWatcher(facadeCaller),
		APIAddresser:   common.NewAPIAddresser(facadeCaller),
		facade:         facadeCaller}
}

// machineLife requests the lifecycle of the given machine from the server.
func (st *State) machineLife(tag names.MachineTag) (params.Life, error) {
	return common.Life(st.facade, tag)
}

// Machine provides access to methods of a state.Machine through the facade.
func (st *State) Machine(tag names.MachineTag) (*Machine, error) {
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
	err := st.facade.FacadeCall("WatchEnvironMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

func (st *State) WatchMachineErrorRetry() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := st.facade.FacadeCall("WatchMachineErrorRetry", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// StateAddresses returns the list of addresses used to connect to the state.
func (st *State) StateAddresses() ([]string, error) {
	var result params.StringsResult
	err := st.facade.FacadeCall("StateAddresses", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

// ContainerManagerConfig returns information from the environment config that is
// needed for configuring the container manager.
func (st *State) ContainerManagerConfig(args params.ContainerManagerConfigParams) (result params.ContainerManagerConfig, err error) {
	err = st.facade.FacadeCall("ContainerManagerConfig", args, &result)
	return result, err
}

// ContainerConfig returns information from the environment config that is
// needed for container cloud-init.
func (st *State) ContainerConfig() (result params.ContainerConfig, err error) {
	err = st.facade.FacadeCall("ContainerConfig", nil, &result)
	return result, err
}

// MachinesWithTransientErrors returns a slice of machines and corresponding status information
// for those machines which have transient provisioning errors.
func (st *State) MachinesWithTransientErrors() ([]*Machine, []params.StatusResult, error) {
	var results params.StatusResults
	err := st.facade.FacadeCall("MachinesWithTransientErrors", nil, &results)
	if err != nil {
		return nil, nil, err
	}
	machines := make([]*Machine, len(results.Results))
	for i, status := range results.Results {
		if status.Error != nil {
			continue
		}
		machines[i] = &Machine{
			tag:  names.NewMachineTag(status.Id),
			life: status.Life,
			st:   st,
		}
	}
	return machines, results.Results, nil
}

// FindTools returns al ist of tools matching the specified version number and
// series, and, if non-empty, arch.
func (st *State) FindTools(v version.Number, series string, arch *string) (tools.List, error) {
	args := params.FindToolsParams{
		Number:       v,
		Series:       series,
		MajorVersion: -1,
		MinorVersion: -1,
	}
	if arch != nil {
		args.Arch = *arch
	}
	var result params.FindToolsResult
	if err := st.facade.FacadeCall("FindTools", args, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return result.List, nil
}
