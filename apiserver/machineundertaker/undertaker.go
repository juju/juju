// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker

import (
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.machineundertaker")

func init() {
	common.RegisterStandardFacade("MachineUndertaker", 1, NewMachineUndertakerAPI)
}

type MachineUndertakerAPI struct {
	st        State
	resources facade.Resources
}

// NewMachineUndertakerAPI implements the API used by the machine
// undertaker worker to find out what provider-level resources need to
// be cleaned up when a machine goes away.
func NewMachineUndertakerAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*MachineUndertakerAPI, error) {
	return newMachineUndertakerAPI(&stateShim{st}, resources, authorizer)
}

func newMachineUndertakerAPI(st State, resources facade.Resources, authorizer facade.Authorizer) (*MachineUndertakerAPI, error) {
	if !authorizer.AuthModelManager() {
		return nil, common.ErrPerm
	}
	api := &MachineUndertakerAPI{
		st:        st,
		resources: resources,
	}
	return api, nil
}

func (m *MachineUndertakerAPI) AllMachineRemovals() (params.MachineRemovalsResults, error) {
	result := params.MachineRemovalsResults{}
	removals, err := m.st.AllMachineRemovals()
	if err != nil {
		return result, err
	}
	result.Results = removalsToParams(removals)
	return result, nil
}

func (m *MachineUndertakerAPI) ClearMachineRemovals(args params.Entities) error {
	machineIDs := make([]string, len(args.Entities))
	for i := range args.Entities {
		tag, err := names.ParseMachineTag(args.Entities[i].Tag)
		if err != nil {
			return err
		}
		machineIDs[i] = tag.Id()
	}
	return m.st.ClearMachineRemovals(machineIDs)
}

func (m *MachineUndertakerAPI) WatchMachineRemovals() params.NotifyWatchResult {
	watch := m.st.WatchMachineRemovals()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: m.resources.Register(watch),
		}
	}
	return params.NotifyWatchResult{
		Error: common.ServerError(watcher.EnsureErr(watch)),
	}
}

func removalsToParams(removals []MachineRemoval) []params.MachineRemoval {
	var result []params.MachineRemoval
	for _, removal := range removals {
		paramRemoval := params.MachineRemoval{
			MachineTag:       names.NewMachineTag(removal.MachineID()).String(),
			LinkLayerDevices: devicesToNetworkConfigs(removal.LinkLayerDevices()),
		}
		result = append(result, paramRemoval)
	}
	return result
}

func devicesToNetworkConfigs(devices []LinkLayerDevice) []params.NetworkConfig {
	var result []params.NetworkConfig
	for _, device := range devices {
		networkConfig := params.NetworkConfig{
			InterfaceName: device.Name(),
			MACAddress:    device.MACAddress(),
			InterfaceType: string(device.Type()),
			MTU:           int(device.MTU()),
			ProviderId:    string(device.ProviderID()),
		}
		result = append(result, networkConfig)
	}
	return result
}
