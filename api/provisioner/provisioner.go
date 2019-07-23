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
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/network"
	"github.com/juju/juju/tools"
)

// State provides access to the Machiner API facade.
type State struct {
	*common.ModelWatcher
	*common.APIAddresser
	*common.ControllerConfigAPI

	facade base.FacadeCaller
}

// MachineResult provides a found Machine and any Error related to
// finding it.
type MachineResult struct {
	Machine MachineProvisioner
	Err     *params.Error
}

// MachineStatusResult provides a found Machine and Status Results
// for it.
type MachineStatusResult struct {
	Machine MachineProvisioner
	Status  params.StatusResult
}

// DistributionGroupResult provides a slice of machine.Ids in the
// distribution group and any Error related to finding it.
type DistributionGroupResult struct {
	MachineIds []string
	Err        *params.Error
}

// LXDProfileResult provides a charm.LXDProfile, adding the name.
type LXDProfileResult struct {
	Config      map[string]string            `json:"config" yaml:"config"`
	Description string                       `json:"description" yaml:"description"`
	Devices     map[string]map[string]string `json:"devices" yaml:"devices"`
	Name        string                       `json:"name" yaml:"name"`
}

const provisionerFacade = "Provisioner"

// NewState creates a new provisioner facade using the input caller.
func NewState(caller base.APICaller) *State {
	facadeCaller := base.NewFacadeCaller(caller, provisionerFacade)
	return NewStateFromFacade(facadeCaller)
}

// NewStateFromFacade creates a new provisioner facade using the input
// facade caller.
func NewStateFromFacade(facadeCaller base.FacadeCaller) *State {
	return &State{
		ModelWatcher:        common.NewModelWatcher(facadeCaller),
		APIAddresser:        common.NewAPIAddresser(facadeCaller),
		ControllerConfigAPI: common.NewControllerConfig(facadeCaller),
		facade:              facadeCaller,
	}
}

// machineLife requests the lifecycle of the given machine from the server.
func (st *State) machineLife(tag names.MachineTag) (params.Life, error) {
	return common.OneLife(st.facade, tag)
}

// Machine provides access to methods of a state.Machine through the facade
// for the given tags.
func (st *State) Machines(tags ...names.MachineTag) ([]MachineResult, error) {
	lenTags := len(tags)
	genericTags := make([]names.Tag, lenTags)
	for i, t := range tags {
		genericTags[i] = t
	}
	result, err := common.Life(st.facade, genericTags)
	if err != nil {
		return []MachineResult{}, err
	}
	machines := make([]MachineResult, lenTags)
	for i, r := range result {
		if r.Error == nil {
			machines[i].Machine = &Machine{
				tag:  tags[i],
				life: r.Life,
				st:   st,
			}
		} else {
			machines[i].Err = r.Error
		}
	}
	return machines, nil
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
	if st.facade.BestAPIVersion() < 6 {
		return st.containerConfigV5()
	}
	err = st.facade.FacadeCall("ContainerConfig", nil, &result)
	return result, err
}

func (st *State) containerConfigV5() (params.ContainerConfig, error) {
	var result params.ContainerConfigV5
	if err := st.facade.FacadeCall("ContainerConfig", nil, &result); err != nil {
		return params.ContainerConfig{}, err
	}
	return params.ContainerConfig{
		ProviderType:            result.ProviderType,
		AuthorizedKeys:          result.AuthorizedKeys,
		SSLHostnameVerification: result.SSLHostnameVerification,
		LegacyProxy:             result.Proxy,
		AptProxy:                result.AptProxy,
		// JujuProxy is zero value.
		// SnapProxy is zero value,
		AptMirror:                  result.AptMirror,
		CloudInitUserData:          result.CloudInitUserData,
		ContainerInheritProperties: result.ContainerInheritProperties,
		UpdateBehavior:             result.UpdateBehavior,
	}, nil
}

// MachinesWithTransientErrors returns a slice of machines and corresponding status information
// for those machines which have transient provisioning errors.
func (st *State) MachinesWithTransientErrors() ([]MachineStatusResult, error) {
	var results params.StatusResults
	err := st.facade.FacadeCall("MachinesWithTransientErrors", nil, &results)
	if err != nil {
		return []MachineStatusResult{}, err
	}
	machines := make([]MachineStatusResult, len(results.Results))
	for i, status := range results.Results {
		if status.Error != nil {
			continue
		}
		machines[i].Machine = &Machine{
			tag:  names.NewMachineTag(status.Id),
			life: status.Life,
			st:   st,
		}
		machines[i].Status = status
	}
	return machines, nil
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
		routes := make([]network.Route, len(cfg.Routes))
		for j, route := range cfg.Routes {
			routes[j] = network.Route{
				DestinationCIDR: route.DestinationCIDR,
				GatewayIP:       route.GatewayIP,
				Metric:          route.Metric,
			}
		}
		ifaceInfo[i] = network.InterfaceInfo{
			DeviceIndex:         cfg.DeviceIndex,
			MACAddress:          cfg.MACAddress,
			CIDR:                cfg.CIDR,
			MTU:                 cfg.MTU,
			ProviderId:          corenetwork.Id(cfg.ProviderId),
			ProviderSubnetId:    corenetwork.Id(cfg.ProviderSubnetId),
			ProviderSpaceId:     corenetwork.Id(cfg.ProviderSpaceId),
			ProviderVLANId:      corenetwork.Id(cfg.ProviderVLANId),
			ProviderAddressId:   corenetwork.Id(cfg.ProviderAddressId),
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
			Routes:              routes,
		}
	}
	return ifaceInfo, nil
}

// SetHostMachineNetworkConfig sets the network configuration of the
// machine with netConfig
func (st *State) SetHostMachineNetworkConfig(hostMachineTag names.MachineTag, netConfig []params.NetworkConfig) error {
	args := params.SetMachineNetworkConfig{
		Tag:    hostMachineTag.String(),
		Config: netConfig,
	}
	err := st.facade.FacadeCall("SetHostMachineNetworkConfig", args, nil)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *State) HostChangesForContainer(containerTag names.MachineTag) ([]network.DeviceToBridge, int, error) {
	var result params.HostNetworkChangeResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: containerTag.String()}},
	}
	if err := st.facade.FacadeCall("HostChangesForContainers", args, &result); err != nil {
		return nil, 0, err
	}
	if len(result.Results) != 1 {
		return nil, 0, errors.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nil, 0, err
	}
	newBridges := result.Results[0].NewBridges
	res := make([]network.DeviceToBridge, len(newBridges))
	for i, bridgeInfo := range newBridges {
		res[i].BridgeName = bridgeInfo.BridgeName
		res[i].DeviceName = bridgeInfo.HostDeviceName
		res[i].MACAddress = bridgeInfo.MACAddress
	}
	return res, result.Results[0].ReconfigureDelay, nil
}

// DistributionGroupByMachineId returns a slice of machine.Ids
// that belong to the same distribution group as the given
// Machine. The provisioner may use this information
// to distribute instances for high availability.
func (st *State) DistributionGroupByMachineId(tags ...names.MachineTag) ([]DistributionGroupResult, error) {
	var stringResults params.StringsResults
	entities := make([]params.Entity, len(tags))
	for i, t := range tags {
		entities[i] = params.Entity{Tag: t.String()}
	}
	err := st.facade.FacadeCall("DistributionGroupByMachineId", params.Entities{Entities: entities}, &stringResults)
	if err != nil {
		return []DistributionGroupResult{}, err
	}
	results := make([]DistributionGroupResult, len(tags))
	for i, stringResult := range stringResults.Results {
		results[i] = DistributionGroupResult{MachineIds: stringResult.Result, Err: stringResult.Error}
	}
	return results, nil
}

// CACert returns the certificate used to validate the API and state connections.
func (a *State) CACert() (string, error) {
	var result params.BytesResult
	err := a.facade.FacadeCall("CACert", nil, &result)
	if err != nil {
		return "", err
	}
	return string(result.Result), nil
}

// GetContainerProfileInfo returns a slice of ContainerLXDProfile, 1 for each unit's charm
// which contains an lxd-profile.yaml.
func (st *State) GetContainerProfileInfo(containerTag names.MachineTag) ([]*LXDProfileResult, error) {
	var result params.ContainerProfileResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: containerTag.String()}},
	}
	if err := st.facade.FacadeCall("GetContainerProfileInfo", args, &result); err != nil {
		return nil, err
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nil, err
	}
	profiles := result.Results[0].LXDProfiles
	var res []*LXDProfileResult
	for _, p := range profiles {
		if p == nil {
			continue
		}
		res = append(res, &LXDProfileResult{
			Config:      p.Profile.Config,
			Description: p.Profile.Description,
			Devices:     p.Profile.Devices,
			Name:        p.Name,
		})
	}
	return res, nil
}
