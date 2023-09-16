// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"net"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type NetInfoAddress interface {
	network.Address

	// SpaceAddr returns the SpaceAddress representation for
	// the address, which was determined from its subnet.
	SpaceAddr() network.SpaceAddress

	// DeviceName is the name of the link-layer device
	// with which this address is associated.
	DeviceName() string

	// HWAddr returns the hardware address (MAC or Infiniband GUID)
	// for the device with which this address is associated.
	HWAddr() (string, error)

	// ParentDeviceName returns the name of the network device that
	// is the parent of this one.
	// An empty string is returned if no parent can be determined.
	ParentDeviceName() string
}

type netInfoAddress struct {
	network.SpaceAddress

	addr *state.Address
	dev  *state.LinkLayerDevice
}

// SpaceAddr implements NetInfoAddress by
// returning the embedded SpaceAddress.
func (a *netInfoAddress) SpaceAddr() network.SpaceAddress {
	return a.SpaceAddress
}

// DeviceName implements NetInfoAddress by returning the address' device name.
// For the case where we construct this from the machine's preferred
// private address, there will be no addr member, so return an empty string.
func (a *netInfoAddress) DeviceName() string {
	if a.addr == nil {
		return ""
	}
	return a.addr.DeviceName()
}

// HWAddr implements NetInfoAddress by returning
// the MAC for this address' device.
// For the case where we construct this from the machine's preferred
// private address, there will be no addr member, so return a not found error.
func (a *netInfoAddress) HWAddr() (string, error) {
	if a.addr == nil {
		return "", errors.NotFoundf("device hardware address")
	}

	if err := a.ensureDevice(); err != nil {
		return "", errors.Trace(err)
	}

	return a.dev.MACAddress(), nil
}

// ParentDeviceName implements NetInfoAddress by returning the device name of
// this NIC's parent. This is used in populateMachineAddresses to sort bridge
// devices before their bridged NICs. As such, an error return is not useful;
// just return empty if no determination can be made.
func (a *netInfoAddress) ParentDeviceName() string {
	if a.addr == nil {
		return ""
	}

	if err := a.ensureDevice(); err != nil {
		return ""
	}

	return a.dev.ParentID()
}

func (a *netInfoAddress) ensureDevice() error {
	if a.dev != nil {
		return nil
	}

	var err error
	a.dev, err = a.addr.Device()
	return errors.Trace(err)
}

// Machine describes methods required for interrogating
// the addresses of a machine in state.
type Machine interface {
	// Id returns the machine's model-unique identifier.
	Id() string

	// MachineTag returns the machine's tag, specifically typed.
	MachineTag() names.MachineTag

	// PrivateAddress returns the machine's preferred private address.
	PrivateAddress() (network.SpaceAddress, error)

	// AllDeviceAddresses returns the IP addresses for the machine's
	// link-layer devices.
	AllDeviceAddresses(subs network.SubnetInfos) ([]NetInfoAddress, error)
}

// machine shims the state representation of a machine in order to implement
// the Machine indirection above.
type machine struct {
	*state.Machine
}

func (m *machine) AllDeviceAddresses(subs network.SubnetInfos) ([]NetInfoAddress, error) {
	addrs, err := m.Machine.AllDeviceAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	res := make([]NetInfoAddress, len(addrs))
	for i, addr := range addrs {
		spaceAddr, err := network.ConvertToSpaceAddress(addr, subs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		res[i] = &netInfoAddress{SpaceAddress: spaceAddr, addr: addr}
	}
	return res, nil
}

// NetworkInfoIAAS is used to provide network info for IAAS units.
type NetworkInfoIAAS struct {
	*NetworkInfoBase

	// machine is where the unit resides.
	machine Machine

	// subs is the collection of subnets in the machine's model.
	subs network.SubnetInfos

	// machineAddress contains addresses for the unit's machine,
	// keyed by spaces that the unit is bound to.
	machineAddresses map[string][]NetInfoAddress
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
	if err = netInfo.populateMachineAddresses(); err != nil {
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
		info := n.networkInfoForSpace(space)

		info.EgressSubnets = endpointEgressSubnets[endpoint]
		info.IngressAddresses = endpointIngressAddresses[endpoint].Values()

		if len(info.IngressAddresses) == 0 {
			info.IngressAddresses = make([]string, len(n.machineAddresses[space]))
			for i, addr := range n.machineAddresses[space] {
				info.IngressAddresses[i] = addr.Host()
			}
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

	space, ingress, egress, err := n.NetworksForRelation(endpoint, rel)
	return endpoint, space, ingress, egress, errors.Trace(err)
}

// NetworksForRelation returns the ingress and egress addresses for
// a relation and unit.
// The ingress addresses depend on if the relation is cross-model
// and whether the relation endpoint is bound to a space.
func (n *NetworkInfoIAAS) NetworksForRelation(
	endpoint string, rel *state.Relation,
) (string, network.SpaceAddresses, []string, error) {
	// If NetworksForRelation is called during ProcessAPIRequest,
	// this is a second validation, but we need to do it for the cases
	// where NetworksForRelation is called directly by EnterScope.
	if err := n.validateEndpoint(endpoint); err != nil {
		return "", nil, nil, errors.Trace(err)
	}
	boundSpace := n.bindings[endpoint]

	// If the endpoint for this relation is not bound to a space,
	// or is bound to the default space, populate ingress addresses
	// based on the input relation and pollPublic flag.
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
		ingress = make(network.SpaceAddresses, len(n.machineAddresses[boundSpace]))
		for i, addr := range n.machineAddresses[boundSpace] {
			ingress[i] = addr.SpaceAddr()
		}
	}

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

	n.machine = &machine{m}
	return nil
}

// populateMachineAddresses sets addresses for the unit's machine
// based on devices with addresses in the unit's bound spaces.
func (n *NetworkInfoIAAS) populateMachineAddresses() error {
	spaceSet := set.NewStrings()
	for _, binding := range n.bindings {
		spaceSet.Add(binding)
	}

	var privateMachineAddress network.SpaceAddress
	n.machineAddresses = make(map[string][]NetInfoAddress)

	if spaceSet.Contains(network.AlphaSpaceId) {
		var err error
		privateMachineAddress, err = n.pollForAddress(n.machine.PrivateAddress)
		if err != nil {
			n.logger.Errorf("unable to obtain preferred private address for machine %q: %s", n.machine.Id(), err.Error())
			// Remove this ID to prevent further processing.
			spaceSet.Remove(network.AlphaSpaceId)
		}
	}

	addrs, err := n.machine.AllDeviceAddresses(n.subs)
	if err != nil {
		return errors.Annotatef(err, "getting machine %q addresses", n.machine.MachineTag())
	}
	sort.Slice(addrs, func(i, j int) bool {
		addr1 := addrs[i]
		addr2 := addrs[j]
		order1 := network.SortOrderMostPublic(addr1)
		order2 := network.SortOrderMostPublic(addr2)
		if order1 == order2 {
			// It is possible to get the same address on multiple devices when
			// we have bridged a device (effectively moving the IP onto the new
			// bridge), but the instance-poller is maintaining the provider's
			// view of the interface with the IP against the original device.
			// In this case, sort parent devices ahead of children.
			if addr1.Host() == addr2.Host() {
				return addr1.ParentDeviceName() < addr2.ParentDeviceName()
			}
			return addr1.Host() < addr2.Host()
		}
		return order1 < order2
	})

	n.logger.Debugf("Looking for address from %v in spaces %v", addrs, spaceSet.Values())

	var privateLinkLayerAddress NetInfoAddress
	for _, addr := range addrs {
		spaceID := addr.SpaceAddr().SpaceID

		if spaceID == "" {
			n.logger.Debugf("skipping %s: not linked to a known space.", addr)

			// For a space-less model, we will not have subnets populated,
			// and will therefore not find a space for the address.
			// Capture the link-layer information for machine private address
			// so that we can return as much information as possible.
			// TODO (manadart 2020-02-21): This will not be required once
			// discovery (or population of subnets by other means) is
			// introduced for the non-space IAAS providers (vSphere etc).
			if addr.Host() == privateMachineAddress.Host() {
				privateLinkLayerAddress = addr
			}
			continue
		}

		if spaceSet.Contains(spaceID) {
			n.addAddressToResult(spaceID, addr)
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
		if spaceSet.Contains(network.AlphaSpaceId) && addr.Host() == privateMachineAddress.Host() {
			n.addAddressToResult(network.AlphaSpaceId, addr)
		}
	}

	// If addresses in the alpha space were requested and we populated none,
	// then we are working with a space-less provider.
	// If we found a link-layer device for the machine's private address,
	// use that information, otherwise return the minimal result based on
	// the machine's preferred private address.
	// TODO (manadart 2020-02-21): As mentioned above, this is not required
	// when we have subnets populated for all providers.
	if _, ok := n.machineAddresses[network.AlphaSpaceId]; !ok && spaceSet.Contains(network.AlphaSpaceId) {
		if privateLinkLayerAddress != nil {
			n.addAddressToResult(network.AlphaSpaceId, privateLinkLayerAddress)
		} else {
			n.addAddressToResult(network.AlphaSpaceId, &netInfoAddress{SpaceAddress: privateMachineAddress})
		}
	}

	for _, id := range spaceSet.Values() {
		if _, ok := n.machineAddresses[id]; !ok {
			n.logger.Warningf("machine %q has no addresses in space %q", n.machine.Id(), id)
		}
	}

	return nil
}

func (n *NetworkInfoIAAS) addAddressToResult(spaceID string, address NetInfoAddress) {
	n.machineAddresses[spaceID] = append(n.machineAddresses[spaceID], address)
}

// networkInfoForSpace transforms the addresses in the input space into
// a NetworkInfoResult to return for the network info request.
func (n *NetworkInfoIAAS) networkInfoForSpace(spaceID string) params.NetworkInfoResult {
	res := params.NetworkInfoResult{}
	hosts := set.NewStrings()

	for _, addr := range n.machineAddresses[spaceID] {
		// If we have already added this address, then we are done.
		// The sort ordering means that we will use a parent device
		// over its child if we have it on more than one.
		if hosts.Contains(addr.Host()) {
			continue
		}
		hosts.Add(addr.Host())

		deviceAddr := params.InterfaceAddress{
			Address: addr.Host(),
			CIDR:    addr.AddressCIDR(),
		}

		// If we've added this address' device, just append the address to it.
		var found bool
		for i := range res.Info {
			if res.Info[i].InterfaceName == addr.DeviceName() {
				res.Info[i].Addresses = append(res.Info[i].Addresses, deviceAddr)
				found = true
				break
			}
		}
		if found {
			continue
		}

		// Otherwise, add a new device.
		var MAC string
		mac, err := addr.HWAddr()
		if err == nil {
			MAC = mac
		} else if !errors.Is(err, errors.NotFound) {
			res.Error = apiservererrors.ServerError(err)
			return res
		}

		res.Info = append(res.Info, params.NetworkInfo{
			InterfaceName: addr.DeviceName(),
			MACAddress:    MAC,
			Addresses:     []params.InterfaceAddress{deviceAddr},
		})
	}

	return res
}
