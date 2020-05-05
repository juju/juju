// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The networkconfigapi package implements the network config parts
// common to machiner and provisioner interface

package networkingcommon

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

var logger = loggo.GetLogger("juju.apiserver.common.networkingcommon")

type NetworkConfigAPI struct {
	st           *state.State
	getCanModify common.GetAuthFunc
}

func NewNetworkConfigAPI(st *state.State, getCanModify common.GetAuthFunc) *NetworkConfigAPI {
	return &NetworkConfigAPI{
		st:           st,
		getCanModify: getCanModify,
	}
}

// SetObservedNetworkConfig reads the network config for the machine identified
// by the input args. This config is merged with the new network config supplied
// in the same args and updated if it has changed.
func (api *NetworkConfigAPI) SetObservedNetworkConfig(args params.SetMachineNetworkConfig) error {
	m, err := api.getMachineForSettingNetworkConfig(args.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	if m.IsContainer() {
		return nil
	}
	observedConfig := args.Config
	logger.Tracef("observed network config of machine %q: %+v", m.Id(), observedConfig)
	if len(observedConfig) == 0 {
		logger.Infof("not updating machine %q network config: no observed network config found", m.Id())
		return nil
	}

	mergedConfig, err := api.fixUpFanSubnets(observedConfig)
	if err != nil {
		return errors.Trace(err)
	}

	ifaces := params.InterfaceInfoFromNetworkConfig(mergedConfig)
	return api.setLinkLayerDevicesAndAddresses(m, ifaces)
}

// fixUpFanSubnets takes network config and updates FAN subnets with proper CIDR, providerId and providerSubnetId.
// The method how fan overlay is cut into segments is described in network/fan.go.
func (api *NetworkConfigAPI) fixUpFanSubnets(networkConfig []params.NetworkConfig) ([]params.NetworkConfig, error) {
	subnets, err := api.st.AllSubnets()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fanSubnets []*state.Subnet
	var fanCIDRs []*net.IPNet
	for _, subnet := range subnets {
		if subnet.FanOverlay() != "" {
			fanSubnets = append(fanSubnets, subnet)
			_, aNet, err := net.ParseCIDR(subnet.CIDR())
			if err != nil {
				return nil, errors.Trace(err)
			}
			fanCIDRs = append(fanCIDRs, aNet)
		}
	}
	for i := range networkConfig {
		localIP := net.ParseIP(networkConfig[i].Address)
		for j, fanSubnet := range fanSubnets {
			if len(fanCIDRs) >= j && fanCIDRs[j].Contains(localIP) {
				networkConfig[i].CIDR = fanSubnet.CIDR()
				networkConfig[i].ProviderId = string(fanSubnet.ProviderId())
				networkConfig[i].ProviderSubnetId = string(fanSubnet.ProviderNetworkId())
				break
			}
		}
	}
	logger.Tracef("Final network config after fixing up FAN subnets %+v", networkConfig)
	return networkConfig, nil
}

// SetProviderNetworkConfig sets the provider supplied network configuration
// contained in the input args against each machine supplied with said args.
func (api *NetworkConfigAPI) SetProviderNetworkConfig(args params.Entities) (params.ErrorResults, error) {
	logger.Tracef("SetProviderNetworkConfig %+v", args)
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	for i, arg := range args.Entities {
		m, err := api.getMachineForSettingNetworkConfig(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		if m.IsContainer() {
			continue
		}

		providerConfig, err := api.getOneMachineProviderNetworkConfig(m)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		} else if len(providerConfig) == 0 {
			continue
		}
		logger.Tracef("provider network config for %q: %+v", m.Id(), providerConfig)

		ifaces := params.InterfaceInfoFromNetworkConfig(providerConfig)
		if err := api.setLinkLayerDevicesAndAddresses(m, ifaces); err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}

func (api *NetworkConfigAPI) getMachineForSettingNetworkConfig(machineTag string) (*state.Machine, error) {
	canModify, err := api.getCanModify()
	if err != nil {
		return nil, errors.Trace(err)
	}

	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !canModify(tag) {
		return nil, errors.Trace(common.ErrPerm)
	}

	m, err := api.getMachine(tag)
	if errors.IsNotFound(err) {
		return nil, errors.Trace(common.ErrPerm)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if m.IsContainer() {
		logger.Debugf("not updating network config for container %q", m.Id())
	}

	return m, nil
}

func (api *NetworkConfigAPI) getMachine(tag names.MachineTag) (*state.Machine, error) {
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Machine), nil
}

func (api *NetworkConfigAPI) getOneMachineProviderNetworkConfig(m *state.Machine) ([]params.NetworkConfig, error) {
	manual, err := m.IsManual()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if manual {
		logger.Infof("provider network config not supported on manually provisioned machines")
		return nil, nil
	}

	model, err := api.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	netEnviron, err := NetworkingEnvironFromModelConfig(
		stateenvirons.EnvironConfigGetter{
			Model: model,
		},
	)
	if errors.IsNotSupported(err) {
		logger.Infof("provider network config not supported: %v", err)
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get provider network config")
	}

	instId, err := m.InstanceId()
	if err != nil {
		return nil, errors.Trace(err)
	}

	interfaceInfos, err := netEnviron.NetworkInterfaces(context.CallContext(api.st), []instance.Id{instId})
	if errors.IsNotSupported(err) {
		// It's possible to have a networking environ, but not support
		// NetworkInterfaces(). In leiu of adding SupportsNetworkInterfaces():
		logger.Infof("provider network interfaces not supported: %v", err)
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot get network interfaces of %q", instId)
	}
	if len(interfaceInfos) == 0 || len(interfaceInfos[0]) == 0 {
		logger.Infof("no provider network interfaces found")
		return nil, nil
	}

	providerConfig := params.NetworkConfigFromInterfaceInfo(interfaceInfos[0])
	logger.Tracef("provider network config instance %q: %+v", instId, providerConfig)

	return providerConfig, nil
}

func (api *NetworkConfigAPI) setLinkLayerDevicesAndAddresses(
	m *state.Machine, interfaceInfos []network.InterfaceInfo,
) error {
	devicesArgs, devicesAddrs := NetworkInterfacesToStateArgs(interfaceInfos)

	logger.Debugf("setting devices: %+v", devicesArgs)
	if err := m.SetParentLinkLayerDevicesBeforeTheirChildren(devicesArgs); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("setting addresses: %+v", devicesAddrs)
	if err := m.SetDevicesAddressesIdempotently(devicesAddrs); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("updated machine %q network config", m.Id())
	return nil
}
