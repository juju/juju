// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/watcher"
)

// State provides access to the Machiner API facade.
type State struct {
	*common.ModelWatcher
	*common.APIAddresser
	*common.ControllerConfigAPI

	facade base.FacadeCaller
}

const provisionerFacade = "Provisioner"

// NewState creates a new client-side Machiner facade.
func NewState(caller base.APICaller) *State {
	facadeCaller := base.NewFacadeCaller(caller, provisionerFacade)
	return &State{
		ModelWatcher:        common.NewModelWatcher(facadeCaller),
		APIAddresser:        common.NewAPIAddresser(facadeCaller),
		ControllerConfigAPI: common.NewControllerConfig(facadeCaller),
		facade:              facadeCaller,
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
		return nil, err
	}
	return &Machine{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// WatchModelMachines returns a StringsWatcher that notifies of
// changes to the lifecycles of the machines (but not containers) in
// the current model.
func (st *State) WatchModelMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := st.facade.FacadeCall("WatchModelMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
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
	w := apiwatcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
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

// ContainerManagerConfig returns information from the model config that is
// needed for configuring the container manager.
func (st *State) ContainerManagerConfig(args params.ContainerManagerConfigParams) (result params.ContainerManagerConfig, err error) {
	err = st.facade.FacadeCall("ContainerManagerConfig", args, &result)
	return result, err
}

// ContainerConfig returns information from the model config that is
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
// series, and, arch. If arch is blank, a default will be used.
func (st *State) FindTools(v version.Number, series string, arch string) (tools.List, error) {
	args := params.FindToolsParams{
		Number:       v,
		Series:       series,
		MajorVersion: -1,
		MinorVersion: -1,
	}
	if arch != "" {
		args.Arch = arch
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

// ReleaseContainerAddresses releases a static IP address allocated to a
// container.
func (st *State) ReleaseContainerAddresses(containerTag names.MachineTag) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot release static addresses for %q", containerTag.Id())
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: containerTag.String()}},
	}
	if err := st.facade.FacadeCall("ReleaseContainerAddresses", args, &result); err != nil {
		return err
	}
	return result.OneError()
}

// PrepareContainerInterfaceInfo allocates an address and returns information to
// configure networking for a container. It accepts container tags as arguments.
func (st *State) PrepareContainerInterfaceInfo(containerTag names.MachineTag) ([]network.InterfaceInfo, error) {
	return st.prepareOrGetContainerInterfaceInfo(containerTag, true)
}

// GetContainerInterfaceInfo returns information to configure networking
// for a container. It accepts container tags as arguments.
func (st *State) GetContainerInterfaceInfo(containerTag names.MachineTag) ([]network.InterfaceInfo, error) {
	return st.prepareOrGetContainerInterfaceInfo(containerTag, false)
}

// prepareOrGetContainerInterfaceInfo returns the necessary information to
// configure network interfaces of a container with allocated static
// IP addresses.
//
// TODO(dimitern): Before we start using this, we need to rename both
// the method and the network.InterfaceInfo type to be called
// InterfaceConfig.
func (st *State) prepareOrGetContainerInterfaceInfo(
	containerTag names.MachineTag, allocateNewAddress bool) (
	[]network.InterfaceInfo, error) {
	var result params.MachineNetworkConfigResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: containerTag.String()}},
	}
	methodName := ""
	if allocateNewAddress {
		methodName = "PrepareContainerInterfaceInfo"
	} else {
		methodName = "GetContainerInterfaceInfo"
	}
	if err := st.facade.FacadeCall(methodName, args, &result); err != nil {
		return nil, err
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nil, err
	}
	machineConf := result.Results[0]
	ifaceInfo := make([]network.InterfaceInfo, len(machineConf.Config))
	for i, cfg := range machineConf.Config {
		ifaceInfo[i] = network.InterfaceInfo{
			DeviceIndex:         cfg.DeviceIndex,
			MACAddress:          cfg.MACAddress,
			CIDR:                cfg.CIDR,
			MTU:                 cfg.MTU,
			ProviderId:          network.Id(cfg.ProviderId),
			ProviderSubnetId:    network.Id(cfg.ProviderSubnetId),
			ProviderSpaceId:     network.Id(cfg.ProviderSpaceId),
			ProviderVLANId:      network.Id(cfg.ProviderVLANId),
			ProviderAddressId:   network.Id(cfg.ProviderAddressId),
			VLANTag:             cfg.VLANTag,
			InterfaceName:       cfg.InterfaceName,
			ParentInterfaceName: cfg.ParentInterfaceName,
			InterfaceType:       network.InterfaceType(cfg.InterfaceType),
			Disabled:            cfg.Disabled,
			NoAutoStart:         cfg.NoAutoStart,
			ConfigType:          network.InterfaceConfigType(cfg.ConfigType),
			Address:             network.NewAddress(cfg.Address),
			DNSServers:          network.NewAddresses(cfg.DNSServers...),
			DNSSearchDomains:    cfg.DNSSearchDomains,
			GatewayAddress:      network.NewAddress(cfg.GatewayAddress),
		}
	}
	return ifaceInfo, nil
}

func (st *State) HostChangesForContainers(containerTag names.MachineTag) ([]network.DeviceToBridge, error) {
	var result params.HostNetworkChangeResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: containerTag.String()}},
	}
	if err := st.facade.FacadeCall("HostChangesForContainers", args, &result); err != nil {
		return nil, err
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nil, err
	}
	newBridges := result.Results[0].NewBridges
	res := make([]network.DeviceToBridge, len(newBridges))
	for i, bridgeInfo := range newBridges {
		res[i].BridgeName = bridgeInfo.BridgeName
		res[i].DeviceName = bridgeInfo.HostDeviceName
	}
	return res, nil
}
