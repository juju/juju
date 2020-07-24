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
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
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
		return nil, errors.Trace(apiservererrors.ErrPerm)
	}

	m, err := api.getMachine(tag)
	if errors.IsNotFound(err) {
		return nil, errors.Trace(apiservererrors.ErrPerm)
	} else if err != nil {
		return nil, errors.Trace(err)
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

func (api *NetworkConfigAPI) setLinkLayerDevicesAndAddresses(
	m *state.Machine, interfaceInfos network.InterfaceInfos,
) error {
	devicesArgs, devicesAddrs := NetworkInterfacesToStateArgs(interfaceInfos)

	logger.Debugf("setting devices: %+v", devicesArgs)
	if err := m.SetParentLinkLayerDevicesBeforeTheirChildren(devicesArgs); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("setting addresses: %+v", devicesAddrs)
	if err := m.SetDevicesAddresses(devicesAddrs...); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("updated machine %q network config", m.Id())
	return nil
}
