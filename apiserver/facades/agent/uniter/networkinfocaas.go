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
)

// NetworkInfoCAAS is used to provide network info for CAAS units.
type NetworkInfoCAAS struct {
	*NetworkInfoBase
}

func (n *NetworkInfoCAAS) ProcessAPIRequest(args params.NetworkInfoParams) (params.NetworkInfoResults, error) {
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
		binding, ok := n.bindings[endpoint]
		if ok {
			spaces.Add(binding)
			bindings[endpoint] = binding
		} else {
			// If default binding is not explicitly defined, use the default space.
			// This should no longer be the case....
			if endpoint == "" {
				bindings[endpoint] = corenetwork.AlphaSpaceId
			} else {
				err := errors.NewNotValid(nil, fmt.Sprintf("binding name %q not defined by the unit's charm", endpoint))
				result.Results[endpoint] = params.NetworkInfoResult{Error: common.ServerError(err)}
			}
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

	var (
		defaultIngressAddresses []string
	)

	// For CAAS units, we build up a minimal result struct
	// based on the default space and unit public/private addresses,
	// ie the addresses of the CAAS service.
	addrs, err := n.unit.AllAddresses()
	if err != nil {
		return params.NetworkInfoResults{}, err
	}
	corenetwork.SortAddresses(addrs)

	// We record the interface addresses as the machine local ones - these
	// are used later as the binding addresses.
	// For CAAS models, we need to default ingress addresses to all available
	// addresses so record those in the default ingress address slice.
	var interfaceAddr []network.InterfaceAddress
	for _, a := range addrs {
		if a.Scope == corenetwork.ScopeMachineLocal {
			interfaceAddr = append(interfaceAddr, network.InterfaceAddress{Address: a.Value})
		} else {
			defaultIngressAddresses = append(defaultIngressAddresses, a.Value)
		}
	}

	networkInfos := make(map[string]machineNetworkInfoResult)
	networkInfos[corenetwork.AlphaSpaceId] = machineNetworkInfoResult{
		NetworkInfos: []network.NetworkInfo{{Addresses: interfaceAddr}},
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

		// If there is no ingress address explicitly defined for a given
		// binding, set the ingress addresses to either any defaults set above,
		// or the binding addresses.
		if len(info.IngressAddresses) == 0 {
			info.IngressAddresses = defaultIngressAddresses
		}

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
