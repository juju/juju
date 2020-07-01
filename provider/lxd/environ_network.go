// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	lxdapi "github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

// Subnets returns basic information about subnets known by the provider for
// the environment.
func (e *environ) Subnets(ctx context.ProviderCallContext, inst instance.Id, subnetIDs []network.Id) ([]network.SubnetInfo, error) {
	srv := e.server()

	// All containers will have the same view on the LXD network. If an
	// instance ID is provided, the best we can do is to also ensure the
	// container actually exists at the cost of an additional API call.
	if inst != instance.UnknownId {
		contList, err := srv.FilterContainers(string(inst))
		if err != nil {
			return nil, errors.Trace(err)
		} else if len(contList) == 0 {
			return nil, errors.NotFoundf("container with instance ID %q", inst)
		}
	}

	networkNames, err := srv.GetNetworkNames()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var keepList set.Strings
	if len(subnetIDs) != 0 {
		keepList = set.NewStrings()
		for _, id := range subnetIDs {
			keepList.Add(string(id))
		}
	}

	var (
		subnets         []network.SubnetInfo
		uniqueSubnetIDs = set.NewStrings()
	)
	for _, networkName := range networkNames {
		state, err := srv.GetNetworkState(networkName)
		if err != nil {
			return nil, errors.Annotatef(err, "querying lxd server for state of network %q", networkName)
		}

		// We are only interested in non-loopback networks that are up.
		if state.Type == "loopback" || state.State != "up" {
			continue
		}

		for _, stateAddr := range state.Addresses {
			netAddr := network.NewProviderAddress(stateAddr.Address)
			if netAddr.Scope == network.ScopeLinkLocal || netAddr.Scope == network.ScopeMachineLocal {
				continue
			}

			subnetID, cidr, err := makeSubnetIDForNetwork(networkName, stateAddr.Address, stateAddr.Netmask)
			if err != nil {
				return nil, errors.Trace(err)
			}

			if uniqueSubnetIDs.Contains(subnetID) {
				continue
			} else if keepList != nil && !keepList.Contains(subnetID) {
				continue
			}

			uniqueSubnetIDs.Add(subnetID)
			subnets = append(subnets, makeSubnetInfo(network.Id(subnetID), makeNetworkID(networkName), cidr))
		}
	}

	return subnets, nil
}

func makeNetworkID(networkName string) network.Id {
	return network.Id(fmt.Sprintf("net-%s", networkName))
}

func makeSubnetIDForNetwork(networkName, address, mask string) (string, string, error) {
	_, netCIDR, err := net.ParseCIDR(fmt.Sprintf("%s/%s", address, mask))
	if err != nil {
		return "", "", errors.Annotatef(err, "calculating CIDR for network %q", networkName)
	}

	cidr := netCIDR.String()
	subnetID := fmt.Sprintf("subnet-%s-%s", networkName, cidr)
	return subnetID, cidr, nil
}

func makeSubnetInfo(subnetID network.Id, networkID network.Id, cidr string) network.SubnetInfo {
	return network.SubnetInfo{
		ProviderId:        subnetID,
		ProviderNetworkId: networkID,
		CIDR:              cidr,
		VLANTag:           0,
	}
}

// NetworkInterfaces returns a slice with the network interfaces that
// correspond to the given instance IDs. If no instances where found, but there
// was no other error, it will return ErrNoInstances. If some but not all of
// the instances were found, the returned slice will have some nil slots, and
// an ErrPartialInstances error will be returned.
func (e *environ) NetworkInterfaces(ctx context.ProviderCallContext, ids []instance.Id) ([]network.InterfaceInfos, error) {
	var (
		missing int
		srv     = e.server()
		res     = make([]network.InterfaceInfos, len(ids))
	)

	for instIdx, id := range ids {
		container, state, err := getContainerDetails(srv, string(id))
		if err != nil {
			if errors.IsNotFound(err) {
				missing++
				continue
			}
			return nil, errors.Annotatef(err, "retrieving network interface info for instance %q", id)
		} else if len(state.Network) == 0 {
			continue
		}

		// Sort interfaces by name to ensure consistent device indexes
		// across calls when we iterate the container's network map.
		networkNames := make([]string, 0, len(state.Network))
		for network := range state.Network {
			networkNames = append(networkNames, network)
		}
		sort.Strings(networkNames)

		var devIdx int
		for _, networkName := range networkNames {
			netInfo := state.Network[networkName]

			// Ignore loopback devices
			if detectInterfaceType(netInfo.Type) == network.LoopbackInterface {
				continue
			}

			ni, err := makeInterfaceInfo(container, networkName, netInfo)
			if err != nil {
				return nil, errors.Annotatef(err, "retrieving network interface info for instane %q", id)
			} else if len(ni.Addresses) == 0 {
				continue
			}

			ni.DeviceIndex = devIdx
			devIdx++
			res[instIdx] = append(res[instIdx], ni)
		}
	}

	if missing > 0 {
		// Found at least one instance
		if missing != len(res) {
			return res, environs.ErrPartialInstances
		}

		return nil, environs.ErrNoInstances
	}
	return res, nil
}

func makeInterfaceInfo(container *lxdapi.Container, networkName string, netInfo lxdapi.ContainerStateNetwork) (network.InterfaceInfo, error) {
	var ni = network.InterfaceInfo{
		MACAddress:          netInfo.Hwaddr,
		MTU:                 netInfo.Mtu,
		InterfaceName:       networkName,
		ParentInterfaceName: hostNetworkForGuestNetwork(container, networkName),
		InterfaceType:       detectInterfaceType(netInfo.Type),
		Origin:              network.OriginProvider,

		// We cannot tell from the API response whether the interface
		// uses a static or DHCP configuration; assume static unless
		// this is a loopback device (see below).
		ConfigType: network.ConfigStatic,
	}

	if ni.InterfaceType == network.LoopbackInterface {
		ni.ConfigType = network.ConfigLoopback
	}

	if ni.ParentInterfaceName != "" {
		ni.ProviderNetworkId = makeNetworkID(ni.ParentInterfaceName)
	}

	// Iterate the list of addresses assigned to this interface ignoring
	// any link-local ones. The first non link-local address is treated as
	// the primary address and is used to populate the interface CIDR and
	// subnet ID fields.
	for _, addr := range netInfo.Addresses {
		netAddr := network.NewProviderAddress(addr.Address)
		if netAddr.Scope == network.ScopeLinkLocal || netAddr.Scope == network.ScopeMachineLocal {
			continue
		}
		ni.Addresses = append(ni.Addresses, netAddr)

		if len(ni.Addresses) > 1 { // CIDR and subnetID already calculated
			continue
		}

		// Use the parent bridge name to match the subnet IDs reported
		// by the Subnets() method.
		subnetID, cidr, err := makeSubnetIDForNetwork(ni.ParentInterfaceName, addr.Address, addr.Netmask)
		if err != nil {
			return network.InterfaceInfo{}, errors.Trace(err)
		}

		ni.CIDR = cidr
		ni.ProviderSubnetId = network.Id(subnetID)
		ni.ProviderId = network.Id(fmt.Sprintf("nic-%s", netInfo.Hwaddr))
	}

	return ni, nil
}

func detectInterfaceType(lxdIfaceType string) network.InterfaceType {
	switch lxdIfaceType {
	case "bridge":
		return network.BridgeInterface
	case "broadcast":
		return network.EthernetInterface
	case "loopback":
		return network.LoopbackInterface
	default:
		return network.UnknownInterface
	}
}

func hostNetworkForGuestNetwork(container *lxdapi.Container, network string) string {
	if container.ExpandedDevices == nil {
		return ""
	}
	devInfo, found := container.ExpandedDevices[network]
	if !found {
		return ""
	}

	return devInfo["network"]
}

func getContainerDetails(srv Server, containerID string) (*lxdapi.Container, *lxdapi.ContainerState, error) {
	cont, _, err := srv.GetContainer(containerID)
	if err != nil {
		// Unfortunately the lxd client does not expose error
		// codes so we need to match against a string here.
		if strings.Contains(err.Error(), "not found") {
			return nil, nil, errors.NotFoundf("container %q", containerID)
		}
		return nil, nil, errors.Trace(err)
	}

	state, _, err := srv.GetContainerState(containerID)
	if err != nil {
		// Unfortunately the lxd client does not expose error
		// codes so we need to match against a string here.
		if strings.Contains(err.Error(), "not found") {
			return nil, nil, errors.NotFoundf("container %q", containerID)
		}
		return nil, nil, errors.Trace(err)
	}

	return cont, state, nil
}
