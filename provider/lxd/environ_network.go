// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"net"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
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
