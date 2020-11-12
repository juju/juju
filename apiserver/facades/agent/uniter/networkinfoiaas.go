// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// NetworkInfoIAAS is used to provide network info for IAAS units.
type NetworkInfoIAAS struct {
	*NetworkInfoBase
}

// ProcessAPIRequest handles a request to the uniter API NetworkInfo method.
func (n *NetworkInfoIAAS) ProcessAPIRequest(args params.NetworkInfoParams) (params.NetworkInfoResults, error) {
	spaces := set.NewStrings()
	bindings := make(map[string]string)
	endpointEgressSubnets := make(map[string][]string)

	result := params.NetworkInfoResults{
		Results: make(map[string]params.NetworkInfoResult),
	}
	// For each of the endpoints in the request, get the bound space and
	// initialise the endpoint egress map with the model's configured
	// egress subnets. Keep track of the spaces that we observe.
	for _, endpoint := range args.Endpoints {
		if binding, ok := n.bindings[endpoint]; ok {
			spaces.Add(binding)
			bindings[endpoint] = binding
		} else {
			err := errors.NewNotValid(nil, fmt.Sprintf("binding name %q not defined by the unit's charm", endpoint))
			result.Results[endpoint] = params.NetworkInfoResult{Error: common.ServerError(err)}
		}
		endpointEgressSubnets[endpoint] = n.defaultEgress
	}

	endpointIngressAddresses := make(map[string]corenetwork.SpaceAddresses)

	// If we are working in a relation context, get the network information for
	// the relation and set it for the relation's binding.
	if args.RelationId != nil {
		endpoint, space, ingress, egress, err := n.getRelationNetworkInfo(*args.RelationId)
		if err != nil {
			return params.NetworkInfoResults{}, err
		}

		spaces.Add(space)
		if len(egress) > 0 {
			endpointEgressSubnets[endpoint] = egress
		}
		endpointIngressAddresses[endpoint] = ingress
	}

	// TODO (manadart 2019-09-10): This looks like it might be called
	// twice in some cases - getRelationNetworkInfo (called above)
	// calls NetworksForRelation, which also calls this method.
	networkInfos, err := n.machineNetworkInfos(spaces.Values()...)
	if err != nil {
		return params.NetworkInfoResults{}, err
	}

	for endpoint, space := range bindings {
		// The binding address information based on link layer devices.
		info := machineNetworkInfoResultToNetworkInfoResult(networkInfos[space])

		// Set egress and ingress address information.
		info.EgressSubnets = endpointEgressSubnets[endpoint]

		ingressAddrs := make([]string, len(endpointIngressAddresses[endpoint]))
		for i, addr := range endpointIngressAddresses[endpoint] {
			ingressAddrs[i] = addr.Value
		}
		info.IngressAddresses = ingressAddrs

		if len(info.IngressAddresses) == 0 {
			ingress := spaceAddressesFromNetworkInfo(networkInfos[space].NetworkInfos)
			corenetwork.SortAddresses(ingress)
			info.IngressAddresses = make([]string, len(ingress))
			for i, addr := range ingress {
				info.IngressAddresses[i] = addr.Value
			}
		}

		// If there is no egress subnet explicitly defined for a given binding,
		// default to the first ingress address. This matches the behaviour when
		// there's a relation in place.
		if len(info.EgressSubnets) == 0 && len(info.IngressAddresses) > 0 {
			var err error
			info.EgressSubnets, err = network.FormatAsCIDR([]string{info.IngressAddresses[0]})
			if err != nil {
				return result, errors.Trace(err)
			}
		}

		result.Results[endpoint] = info
	}

	return dedupNetworkInfoResults(result), nil
}

// getRelationNetworkInfo returns the endpoint name, network space
// and ingress/egress addresses for the input relation ID.
func (n *NetworkInfoIAAS) getRelationNetworkInfo(
	relationId int,
) (string, string, corenetwork.SpaceAddresses, []string, error) {
	rel, endpoint, err := n.getRelationAndEndpointName(relationId)
	if err != nil {
		return "", "", nil, nil, errors.Trace(err)
	}

	space, ingress, egress, err := n.NetworksForRelation(endpoint, rel, true)
	return endpoint, space, ingress, egress, errors.Trace(err)
}

// TODO (manadart 2020-08-20): The logic below was moved over from the state
// package when machine.GetNetworkInfoForSpaces was removed from state and
// implemented here. It is an unnecessary convolution and should be factored
// out in favour of a simpler return from machineNetworkInfos.

// MachineNetworkInfoResult contains an error or a list of NetworkInfo
// structures for a specific space.
type machineNetworkInfoResult struct {
	NetworkInfos []network.NetworkInfo
	Error        error
}

// Add address to a device in list or create a new device with this address.
func addAddressToResult(networkInfos []network.NetworkInfo, address *state.Address) ([]network.NetworkInfo, error) {
	deviceAddr := network.InterfaceAddress{
		Address: address.Value(),
		CIDR:    address.SubnetCIDR(),
	}
	for i := range networkInfos {
		networkInfo := &networkInfos[i]
		if networkInfo.InterfaceName == address.DeviceName() {
			networkInfo.Addresses = append(networkInfo.Addresses, deviceAddr)
			return networkInfos, nil
		}
	}

	var MAC string
	device, err := address.Device()
	if err == nil {
		MAC = device.MACAddress()
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	networkInfo := network.NetworkInfo{
		InterfaceName: address.DeviceName(),
		MACAddress:    MAC,
		Addresses:     []network.InterfaceAddress{deviceAddr},
	}
	return append(networkInfos, networkInfo), nil
}

func machineNetworkInfoResultToNetworkInfoResult(inResult machineNetworkInfoResult) params.NetworkInfoResult {
	if inResult.Error != nil {
		return params.NetworkInfoResult{Error: common.ServerError(inResult.Error)}
	}
	infos := make([]params.NetworkInfo, len(inResult.NetworkInfos))
	for i, info := range inResult.NetworkInfos {
		infos[i] = networkToParamsNetworkInfo(info)
	}
	return params.NetworkInfoResult{
		Info: infos,
	}
}

func networkToParamsNetworkInfo(info network.NetworkInfo) params.NetworkInfo {
	addresses := make([]params.InterfaceAddress, len(info.Addresses))
	for i, addr := range info.Addresses {
		addresses[i] = params.InterfaceAddress{
			Address: addr.Address,
			CIDR:    addr.CIDR,
		}
	}
	return params.NetworkInfo{
		MACAddress:    info.MACAddress,
		InterfaceName: info.InterfaceName,
		Addresses:     addresses,
	}
}
