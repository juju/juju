// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/retry"

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

		info.EgressSubnets = endpointEgressSubnets[endpoint]
		info.IngressAddresses = endpointIngressAddresses[endpoint].Values()

		if len(info.IngressAddresses) == 0 {
			ingress := spaceAddressesFromNetworkInfo(networkInfos[space].NetworkInfos)
			corenetwork.SortAddresses(ingress)
			info.IngressAddresses = ingress.Values()
		}

		if len(info.EgressSubnets) == 0 {
			if info.EgressSubnets, err = n.getEgressFromIngress(info.IngressAddresses); err != nil {
				return result, errors.Trace(err)
			}
		}

		result.Results[endpoint] = info
	}

	return result, nil
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

// NetworksForRelation returns the ingress and egress addresses for
// a relation and unit.
// The ingress addresses depend on if the relation is cross-model
// and whether the relation endpoint is bound to a space.
func (n *NetworkInfoIAAS) NetworksForRelation(
	binding string, rel *state.Relation, _ bool,
) (string, corenetwork.SpaceAddresses, []string, error) {
	boundSpace, err := n.spaceForBinding(binding)
	if err != nil && !errors.IsNotValid(err) {
		return "", nil, nil, errors.Trace(err)
	}

	// If the endpoint for this relation is not bound to a space,
	// or is bound to the default space, populate ingress
	// addresses the input relation and pollPublic flag.
	var ingress corenetwork.SpaceAddresses
	if boundSpace == corenetwork.AlphaSpaceId || err != nil {
		addrs, err := n.maybeGetUnitAddress(rel)
		if err != nil {
			return "", nil, nil, errors.Trace(err)
		}
		ingress = addrs
	}

	// We don't yet have any ingress addresses,
	// so pick one from the space to which the endpoint is bound.
	if len(ingress) == 0 {
		networkInfos, err := n.machineNetworkInfos(boundSpace)
		if err != nil {
			return "", nil, nil, errors.Trace(err)
		}
		ingress = spaceAddressesFromNetworkInfo(networkInfos[boundSpace].NetworkInfos)
	}

	corenetwork.SortAddresses(ingress)

	egress, err := n.getEgressForRelation(rel, ingress)
	if err != nil {
		return "", nil, nil, errors.Trace(err)
	}

	return boundSpace, ingress, egress, nil
}

// spaceForBinding returns the space id
// associated with the specified endpoint.
func (n *NetworkInfoBase) spaceForBinding(endpoint string) (string, error) {
	boundSpace, known := n.bindings[endpoint]
	if !known {
		// If default binding is not explicitly defined, use the default space.
		// This should no longer be the case....
		if endpoint == "" {
			return corenetwork.AlphaSpaceId, nil
		}
		return "", errors.NewNotValid(nil, fmt.Sprintf("binding id %q not defined by the unit's charm", endpoint))
	}
	return boundSpace, nil
}

// machineNetworkInfos returns network info for the unit's machine based on
// devices with addresses in the input spaces.
func (n *NetworkInfoBase) machineNetworkInfos(spaceIDs ...string) (map[string]machineNetworkInfoResult, error) {
	machineID, err := n.unit.AssignedMachineId()
	if err != nil {
		return nil, errors.Trace(err)
	}
	machine, err := n.st.Machine(machineID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	spaceSet := set.NewStrings(spaceIDs...)

	results := make(map[string]machineNetworkInfoResult)

	var privateIPAddress string

	if spaceSet.Contains(corenetwork.AlphaSpaceId) {
		var err error
		privateMachineAddress, err := n.pollForAddress(machine.PrivateAddress)
		if err != nil {
			results[corenetwork.AlphaSpaceId] = machineNetworkInfoResult{Error: errors.Annotatef(
				err, "getting machine %q preferred private address", machine.MachineTag())}

			// Remove this ID to prevent further processing.
			spaceSet.Remove(corenetwork.AlphaSpaceId)
		} else {
			privateIPAddress = privateMachineAddress.Value
		}
	}

	// Link-layer devices are set in a single transaction for all devices
	// observed on the machine, so the first result will include them all.
	var addresses []*state.Address
	retryArg := n.retryFactory()
	retryArg.Func = func() error {
		var err error
		addresses, err = machine.AllAddresses()
		return err
	}
	retryArg.IsFatalError = func(err error) bool {
		return err != nil
	}
	if err := retry.Call(retryArg); err != nil {
		result := machineNetworkInfoResult{Error: errors.Annotate(err, "getting devices addresses")}
		for _, id := range spaceSet.Values() {
			if _, ok := results[id]; !ok {
				results[id] = result
			}
		}
		return results, nil
	}

	logger.Debugf("Looking for address from %v in spaces %v", addresses, spaceIDs)

	var privateLinkLayerAddress *state.Address
	for _, addr := range addresses {
		subnet, err := addr.Subnet()
		switch {
		case errors.IsNotFound(err):
			logger.Debugf("skipping %s: not linked to a known subnet (%v)", addr, err)

			// For a space-less model, we will not have subnets populated,
			// and will therefore not find a subnet for the address.
			// Capture the link-layer information for machine private address
			// so that we can return as much information as possible.
			// TODO (manadart 2020-02-21): This will not be required once
			// discovery (or population of subnets by other means) is
			// introduced for the non-space IAAS providers (LXD, manual, etc).
			if addr.Value() == privateIPAddress {
				privateLinkLayerAddress = addr
			}
		case err != nil:
			logger.Errorf("cannot get subnet for address %q - %q", addr, err)
		default:
			if spaceSet.Contains(subnet.SpaceID()) {
				r := results[subnet.SpaceID()]
				r.NetworkInfos, err = addAddressToResult(r.NetworkInfos, addr)
				if err != nil {
					r.Error = err
				} else {
					results[subnet.SpaceID()] = r
				}
			}

			// TODO (manadart 2020-02-21): This reflects the behaviour prior
			// to the introduction of the alpha space.
			// It mimics the old behaviour for the empty space ("").
			// If that was passed in, we included the machine's preferred
			// local-cloud address no matter what space it was in,
			// treating the request as space-agnostic.
			// To preserve this behaviour, we return the address as a result
			// in the alpha space no matter its *real* space if addresses in
			// the alpha space were requested.
			// This should be removed with the institution of universal mutable
			// spaces.
			if spaceSet.Contains(corenetwork.AlphaSpaceId) && addr.Value() == privateIPAddress {
				r := results[corenetwork.AlphaSpaceId]
				r.NetworkInfos, err = addAddressToResult(r.NetworkInfos, addr)
				if err != nil {
					r.Error = err
				} else {
					results[corenetwork.AlphaSpaceId] = r
				}
			}
		}
	}

	// If addresses in the alpha space were requested and we populated none,
	// then we are working with a space-less provider.
	// If we found a link-layer device for the machine's private address,
	// use that information, otherwise return the minimal result based on
	// the IP.
	// TODO (manadart 2020-02-21): As mentioned above, this is not required
	// when we have subnets populated for all providers.
	if r, ok := results[corenetwork.AlphaSpaceId]; !ok && spaceSet.Contains(corenetwork.AlphaSpaceId) {
		if privateLinkLayerAddress != nil {
			r.NetworkInfos, _ = addAddressToResult(r.NetworkInfos, privateLinkLayerAddress)
		} else {
			r.NetworkInfos = []network.NetworkInfo{{
				Addresses: []network.InterfaceAddress{{
					Address: privateIPAddress,
				}},
			}}
		}

		results[corenetwork.AlphaSpaceId] = r
	}

	for _, id := range spaceSet.Values() {
		if _, ok := results[id]; !ok {
			results[id] = machineNetworkInfoResult{
				Error: errors.Errorf("machine %q has no devices in space %q", machineID, id),
			}
		}
	}
	return results, nil
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
