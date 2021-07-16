// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"net"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// Machine describes methods required for interrogating
// the addresses of a machine in state.
type Machine interface {
	// Id returns the machine's model-unique identifier.
	Id() string

	// MachineTag returns the machine's tag, specifically typed.
	MachineTag() names.MachineTag

	// PrivateAddress returns the machine's preferred private address.
	PrivateAddress() (network.SpaceAddress, error)

	// AllDeviceAddresses returns the state representation of a machine's
	// addresses.
	// TODO (manadart 2021-06-15): Indirect this too.
	AllDeviceAddresses() ([]*state.Address, error)
}

// machine shims the state representation of a machine in order to implement
// the Machine indirection above.
type machine struct {
	*state.Machine
}

// NetworkInfoIAAS is used to provide network info for IAAS units.
type NetworkInfoIAAS struct {
	*NetworkInfoBase

	// machine is where the unit resides.
	machine Machine

	// subs is the collection of subnets in the machine's model.
	subs network.SubnetInfos

	// machineNetworkInfos contains network info for the unit's machine,
	// keyed by spaces that the unit is bound to.
	machineNetworkInfos map[string]params.NetworkInfoResult
}

func newNetworkInfoIAAS(base *NetworkInfoBase) (*NetworkInfoIAAS, error) {
	spaces := set.NewStrings()
	for _, binding := range base.bindings {
		spaces.Add(binding)
	}

	netInfo := &NetworkInfoIAAS{NetworkInfoBase: base}

	var err error
	if err = netInfo.populateUnitMachine(); err != nil {
		return nil, errors.Trace(err)
	}
	if netInfo.subs, err = netInfo.st.AllSubnetInfos(); err != nil {
		return nil, errors.Trace(err)
	}
	if err = netInfo.populateMachineNetworkInfos(); err != nil {
		return nil, errors.Trace(err)
	}

	return netInfo, nil
}

// ProcessAPIRequest handles a request to the uniter API NetworkInfo method.
func (n *NetworkInfoIAAS) ProcessAPIRequest(args params.NetworkInfoParams) (params.NetworkInfoResults, error) {
	spaces := set.NewStrings()
	bindings := make(map[string]string)
	endpointEgressSubnets := make(map[string][]string)

	// For each of the valid endpoints in the request,
	// get the bound space and initialise the endpoint egress
	// map with the model's configured egress subnets.
	// Keep track of the spaces that we observe.
	validEndpoints, result := n.validateEndpoints(args.Endpoints)
	for _, endpoint := range validEndpoints.Values() {
		binding := n.bindings[endpoint]
		spaces.Add(binding)
		bindings[endpoint] = binding
		endpointEgressSubnets[endpoint] = n.defaultEgress
	}

	endpointIngressAddresses := make(map[string]network.SpaceAddresses)

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

	for endpoint, space := range bindings {
		// The binding address information based on link layer devices.
		info := n.machineNetworkInfos[space]

		info.EgressSubnets = endpointEgressSubnets[endpoint]
		info.IngressAddresses = endpointIngressAddresses[endpoint].Values()

		if len(info.IngressAddresses) == 0 {
			ingress := spaceAddressesFromNetworkInfo(n.machineNetworkInfos[space].Info)
			network.SortAddresses(ingress)
			info.IngressAddresses = ingress.Values()
		}

		if len(info.EgressSubnets) == 0 {
			info.EgressSubnets = subnetsForAddresses(info.IngressAddresses)
		}

		result.Results[endpoint] = n.resolveResultIngressHostNames(n.resolveResultInfoHostNames(info))
	}

	return result, nil
}

// getRelationNetworkInfo returns the endpoint name, network space
// and ingress/egress addresses for the input relation ID.
func (n *NetworkInfoIAAS) getRelationNetworkInfo(
	relationId int,
) (string, string, network.SpaceAddresses, []string, error) {
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
	endpoint string, rel *state.Relation, _ bool,
) (string, network.SpaceAddresses, []string, error) {
	// If NetworksForRelation is called during ProcessAPIRequest,
	// this is a second validation, but we need to do it for the cases
	// where NetworksForRelation is called directly by EnterScope.
	if err := n.validateEndpoint(endpoint); err != nil {
		return "", nil, nil, errors.Trace(err)
	}
	boundSpace := n.bindings[endpoint]

	// If the endpoint for this relation is not bound to a space,
	// or is bound to the default space, populate ingress
	// addresses the input relation and pollPublic flag.
	var ingress network.SpaceAddresses
	if boundSpace == network.AlphaSpaceId {
		addrs, err := n.maybeGetUnitAddress(rel, true)
		if err != nil {
			return "", nil, nil, errors.Trace(err)
		}
		ingress = addrs
	}

	// We don't yet have any ingress addresses,
	// so pick one from the space to which the endpoint is bound.
	if len(ingress) == 0 {
		ingress = spaceAddressesFromNetworkInfo(n.machineNetworkInfos[boundSpace].Info)
	}

	network.SortAddresses(ingress)

	egress, err := n.getEgressForRelation(rel, ingress)
	if err != nil {
		return "", nil, nil, errors.Trace(err)
	}

	return boundSpace, ingress, egress, nil
}

// resolveResultIngressHostNames returns a new NetworkInfoResult with host names
// in the `IngressAddresses` member resolved to IP addresses where possible.
// This is slightly different to the `Info` addresses above in that we do not
// include anything that does not resolve to a usable address.
func (n *NetworkInfoIAAS) resolveResultIngressHostNames(netInfo params.NetworkInfoResult) params.NetworkInfoResult {
	var newIngress []string
	for _, addr := range netInfo.IngressAddresses {
		if ip := net.ParseIP(addr); ip != nil {
			newIngress = append(newIngress, addr)
			continue
		}
		if ipAddr := n.resolveHostAddress(addr); ipAddr != "" {
			newIngress = append(newIngress, ipAddr)
		}
	}
	netInfo.IngressAddresses = newIngress

	return netInfo
}

func (n *NetworkInfoIAAS) populateUnitMachine() error {
	mID, err := n.unit.AssignedMachineId()
	if err != nil {
		return errors.Trace(err)
	}

	m, err := n.st.Machine(mID)
	if err != nil {
		return errors.Trace(err)
	}

	n.machine = machine{m}
	return nil
}

// populateMachineNetworkInfos sets network info for the unit's machine
// based on devices with addresses in the unit's bound spaces.
func (n *NetworkInfoIAAS) populateMachineNetworkInfos() error {
	spaceSet := set.NewStrings()
	for _, binding := range n.bindings {
		spaceSet.Add(binding)
	}

	var privateIPAddress string
	n.machineNetworkInfos = make(map[string]params.NetworkInfoResult)

	if spaceSet.Contains(network.AlphaSpaceId) {
		var err error
		privateMachineAddress, err := n.pollForAddress(n.machine.PrivateAddress)
		if err != nil {
			n.machineNetworkInfos[network.AlphaSpaceId] = params.NetworkInfoResult{Error: apiservererrors.ServerError(
				errors.Annotatef(err, "getting machine %q preferred private address", n.machine.MachineTag()))}

			// Remove this ID to prevent further processing.
			spaceSet.Remove(network.AlphaSpaceId)
		} else {
			privateIPAddress = privateMachineAddress.Value
		}
	}

	// This is not ideal. We need device information associated with the state
	// Address representation, but we also need the addresses in a form that
	// can be sorted for scope and primary/secondary status.
	// We create a map for the information we need to return, and a separate
	// sorted slice for iteration in the correct order.
	addrs, err := n.machine.AllDeviceAddresses()
	if err != nil {
		n.populateMachineNetworkInfoErrors(spaceSet, err)
		return nil
	}
	addrByIP := make(map[string]*state.Address)
	for _, addr := range addrs {
		addrByIP[addr.Value()] = addr
	}

	spaceAddrs := make([]network.SpaceAddress, len(addrs))
	for i, addr := range addrs {
		if spaceAddrs[i], err = network.ConvertToSpaceAddress(addr, n.subs); err != nil {
			n.populateMachineNetworkInfoErrors(spaceSet, err)
			return nil
		}
	}
	network.SortAddresses(spaceAddrs)

	logger.Debugf("Looking for address from %v in spaces %v", spaceAddrs, spaceSet.Values())

	var privateLinkLayerAddress *state.Address
	for _, spaceAddr := range spaceAddrs {
		addr, ok := addrByIP[spaceAddr.Value]
		if !ok {
			return errors.Errorf("address representations inconsistent; could not find %s", spaceAddr.Value)
		}

		if spaceAddr.SpaceID == "" {
			logger.Debugf("skipping %s: not linked to a known space.", spaceAddr)

			// For a space-less model, we will not have subnets populated,
			// and will therefore not find a space for the address.
			// Capture the link-layer information for machine private address
			// so that we can return as much information as possible.
			// TODO (manadart 2020-02-21): This will not be required once
			// discovery (or population of subnets by other means) is
			// introduced for the non-space IAAS providers (vSphere etc).
			if spaceAddr.Value == privateIPAddress {
				privateLinkLayerAddress = addr
			}
			continue
		}

		if spaceSet.Contains(spaceAddr.SpaceID) {
			n.addAddressToResult(spaceAddr.SpaceID, addr)
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
		if spaceSet.Contains(network.AlphaSpaceId) && addr.Value() == privateIPAddress {
			n.addAddressToResult(network.AlphaSpaceId, addr)
		}
	}

	// If addresses in the alpha space were requested and we populated none,
	// then we are working with a space-less provider.
	// If we found a link-layer device for the machine's private address,
	// use that information, otherwise return the minimal result based on
	// the IP.
	// TODO (manadart 2020-02-21): As mentioned above, this is not required
	// when we have subnets populated for all providers.
	if _, ok := n.machineNetworkInfos[network.AlphaSpaceId]; !ok && spaceSet.Contains(network.AlphaSpaceId) {
		if privateLinkLayerAddress != nil {
			n.addAddressToResult(network.AlphaSpaceId, privateLinkLayerAddress)
		} else {
			n.machineNetworkInfos[network.AlphaSpaceId] = params.NetworkInfoResult{
				Info: []params.NetworkInfo{{Addresses: []params.InterfaceAddress{{Address: privateIPAddress}}}},
			}
		}
	}

	for _, id := range spaceSet.Values() {
		if _, ok := n.machineNetworkInfos[id]; !ok {
			n.machineNetworkInfos[id] = params.NetworkInfoResult{Error: apiservererrors.ServerError(
				errors.Errorf("machine %q has no addresses in space %q", n.machine.Id(), id))}
		}
	}

	return nil
}

// populateMachineNetworkInfoErrors populates a network
// info result error for all of the input spaces.
func (n *NetworkInfoIAAS) populateMachineNetworkInfoErrors(spaces set.Strings, err error) {
	res := params.NetworkInfoResult{Error: apiservererrors.ServerError(
		errors.Annotate(err, "getting devices addresses"))}
	for _, id := range spaces.Values() {
		n.machineNetworkInfos[id] = res
	}
}

// addAddressToResult adds the network info representation
// of the input address to the results for the input space.
func (n *NetworkInfoIAAS) addAddressToResult(spaceID string, address *state.Address) {
	r := n.machineNetworkInfos[spaceID]

	deviceAddr := params.InterfaceAddress{
		Address: address.Value(),
		CIDR:    address.SubnetCIDR(),
	}
	for i := range r.Info {
		if r.Info[i].InterfaceName == address.DeviceName() {
			r.Info[i].Addresses = append(r.Info[i].Addresses, deviceAddr)
			n.machineNetworkInfos[spaceID] = r
			return
		}
	}

	var MAC string
	device, err := address.Device()
	if err == nil {
		MAC = device.MACAddress()
	} else if !errors.IsNotFound(err) {
		r.Error = apiservererrors.ServerError(err)
		n.machineNetworkInfos[spaceID] = r
		return
	}

	networkInfo := params.NetworkInfo{
		InterfaceName: address.DeviceName(),
		MACAddress:    MAC,
		Addresses:     []params.InterfaceAddress{deviceAddr},
	}
	r.Info = append(r.Info, networkInfo)
	n.machineNetworkInfos[spaceID] = r
}

// spaceAddressesFromNetworkInfo returns a SpaceAddresses collection
// from a slice of NetworkInfo.
// We need to construct sortable addresses from link-layer devices,
// which unlike addresses from the machines collection, do not have the scope
// information that we need.
// The best we can do here is identify fan addresses so that they are sorted
// after other addresses.
// TODO (manadart 2021-05-14): Phase this out by doing the following:
// - In populateMachineNetworkInfos, store the retrieved machine SpaceAddresses
//   as a member of NetworkInfoIAAS.
// - Use this in a sort method for []InterfaceAddress that works by:
//   - filtering the SpacesAddress to the same members/order as the input and;
//   - using the sort order of the SpaceAddresses to sort the input by proxy.
func spaceAddressesFromNetworkInfo(netInfos []params.NetworkInfo) network.SpaceAddresses {
	var addrs network.SpaceAddresses
	for _, nwInfo := range netInfos {
		scope := network.ScopeUnknown
		if strings.HasPrefix(nwInfo.InterfaceName, "fan-") {
			scope = network.ScopeFanLocal
		}

		for _, addr := range nwInfo.Addresses {
			addrs = append(addrs, network.NewSpaceAddress(addr.Address, network.WithScope(scope)))
		}
	}
	return addrs
}
