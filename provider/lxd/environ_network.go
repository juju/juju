// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/collections/set"
	lxdapi "github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

var _ environs.Networking = (*environ)(nil)

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
			// Unfortunately, LXD on bionic and earlier does not
			// support the network_state extension out of the box
			// so this call will fail. If that's the case then
			// use a fallback method for detecting subnets.
			if isErrMissingAPIExtension(err, "network_state") {
				return e.subnetDetectionFallback(srv, inst, keepList)
			}
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

// subnetDetectionFallback provides a fallback mechanism for subnet discovery
// on older LXD versions (e.g. the ones that ship with xenial and bionic) which
// do not come with the network_state API extension enabled.
//
// The fallback exploits the fact that subnet discovery is performed after the
// controller spins up. To this end, the method will query any of the available
// juju containers and attempt to reconstruct the subnet information based on
// the devices present inside the container.
//
// Caveat: this method offers lower data fidelity compared to Subnets() as it
// cannot accurately detect the CIDRs for any host devices that are not bridged
// into the container.
func (e *environ) subnetDetectionFallback(srv Server, inst instance.Id, keepSubnetIDs set.Strings) ([]network.SubnetInfo, error) {
	logger.Warningf("falling back to subnet discovery via introspection of devices bridged to the controller container; consider upgrading to a newer LXD version and running 'juju reload-spaces' to get full subnet discovery for the LXD host")

	// If no instance ID is specified, list the alive containers, query the
	// state of the first one on the list and use it to extrapolate the
	// subnet layout.
	if inst == instance.UnknownId {
		aliveConts, err := srv.AliveContainers("juju-")
		if err != nil {
			return nil, errors.Trace(err)
		} else if len(aliveConts) == 0 {
			return nil, errors.New("no alive containers detected")
		}
		inst = instance.Id(aliveConts[0].Name)
	}

	container, state, err := getContainerDetails(srv, string(inst))
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		subnets         []network.SubnetInfo
		uniqueSubnetIDs = set.NewStrings()
	)

	for guestNetworkName, netInfo := range state.Network {
		// Ignore loopback devices and NICs in down state.
		if detectInterfaceType(netInfo.Type) == network.LoopbackInterface || netInfo.State != "up" {
			continue
		}

		hostNetworkName := hostNetworkForGuestNetwork(container, guestNetworkName)

		for _, guestAddr := range netInfo.Addresses {
			netAddr := network.NewProviderAddress(guestAddr.Address)
			if netAddr.Scope == network.ScopeLinkLocal || netAddr.Scope == network.ScopeMachineLocal {
				continue
			}

			// Use the detected host network name and the guest
			// address details to generate a subnetID for the host.
			subnetID, cidr, err := makeSubnetIDForNetwork(hostNetworkName, guestAddr.Address, guestAddr.Netmask)
			if err != nil {
				return nil, errors.Trace(err)
			}

			if uniqueSubnetIDs.Contains(subnetID) {
				continue
			} else if keepSubnetIDs != nil && !keepSubnetIDs.Contains(subnetID) {
				continue
			}

			uniqueSubnetIDs.Add(subnetID)
			subnets = append(subnets, makeSubnetInfo(network.Id(subnetID), makeNetworkID(hostNetworkName), cidr))
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
		guestNetworkNames := make([]string, 0, len(state.Network))
		for network := range state.Network {
			guestNetworkNames = append(guestNetworkNames, network)
		}
		sort.Strings(guestNetworkNames)

		var devIdx int
		for _, guestNetworkName := range guestNetworkNames {
			netInfo := state.Network[guestNetworkName]

			// Ignore loopback devices
			if detectInterfaceType(netInfo.Type) == network.LoopbackInterface {
				continue
			}

			ni, err := makeInterfaceInfo(container, guestNetworkName, netInfo)
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

func makeInterfaceInfo(container *lxdapi.Container, guestNetworkName string, netInfo lxdapi.ContainerStateNetwork) (network.InterfaceInfo, error) {
	var ni = network.InterfaceInfo{
		MACAddress:          netInfo.Hwaddr,
		MTU:                 netInfo.Mtu,
		InterfaceName:       guestNetworkName,
		ParentInterfaceName: hostNetworkForGuestNetwork(container, guestNetworkName),
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

func hostNetworkForGuestNetwork(container *lxdapi.Container, guestNetwork string) string {
	if container.ExpandedDevices == nil {
		return ""
	}
	devInfo, found := container.ExpandedDevices[guestNetwork]
	if !found {
		return ""
	}

	if name, found := devInfo["network"]; found { // lxd 4+
		return name
	} else if name, found := devInfo["parent"]; found { // lxd 3
		return name
	}
	return ""
}

func getContainerDetails(srv Server, containerID string) (*lxdapi.Container, *lxdapi.ContainerState, error) {
	cont, _, err := srv.GetContainer(containerID)
	if err != nil {
		if isErrNotFound(err) {
			return nil, nil, errors.NotFoundf("container %q", containerID)
		}
		return nil, nil, errors.Trace(err)
	}

	state, _, err := srv.GetContainerState(containerID)
	if err != nil {
		if isErrNotFound(err) {
			return nil, nil, errors.NotFoundf("container %q", containerID)
		}
		return nil, nil, errors.Trace(err)
	}

	return cont, state, nil
}

// isErrNotFound returns true if the LXD server returned back a "not found" error.
func isErrNotFound(err error) bool {
	// Unfortunately the lxd client does not expose error
	// codes so we need to match against a string here.
	return strings.Contains(err.Error(), "not found")
}

// isErrMissingAPIExtension returns true if the LXD server returned back an
// "API extension not found" error.
func isErrMissingAPIExtension(err error, ext string) bool {
	// Unfortunately the lxd client does not expose error
	// codes so we need to match against a string here.
	return strings.Contains(err.Error(), fmt.Sprintf("server is missing the required %q API extension", ext))
}

// SuperSubnets returns information about aggregated subnet.
func (*environ) SuperSubnets(context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

// SupportsSpaces returns whether the current environment supports
// spaces. The returned error satisfies errors.IsNotSupported(),
// unless a general API failure occurs.
func (*environ) SupportsSpaces(context.ProviderCallContext) (bool, error) {
	return true, nil
}

// SupportsSpaceDiscovery returns whether the current environment
// supports discovering spaces from the provider. The returned error
// satisfies errors.IsNotSupported(), unless a general API failure occurs.
func (*environ) SupportsSpaceDiscovery(context.ProviderCallContext) (bool, error) {
	return false, nil
}

// Spaces returns a slice of network.SpaceInfo with info, including
// details of all associated subnets, about all spaces known to the
// provider that have subnets available.
func (*environ) Spaces(context.ProviderCallContext) ([]network.SpaceInfo, error) {
	return nil, errors.NotSupportedf("spaces")
}

// ProviderSpaceInfo returns the details of the space requested as
// a ProviderSpaceInfo.
func (*environ) ProviderSpaceInfo(context.ProviderCallContext, *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("spaces")
}

// AreSpacesRoutable returns whether the communication between the
// two spaces can use cloud-local addaddresses.
func (*environ) AreSpacesRoutable(context.ProviderCallContext, *environs.ProviderSpaceInfo, *environs.ProviderSpaceInfo) (bool, error) {
	return false, errors.NotSupportedf("spaces")
}

// SupportsContainerAddresses returns true if the current environment is
// able to allocate addaddresses for containers.
func (*environ) SupportsContainerAddresses(context.ProviderCallContext) (bool, error) {
	return false, nil
}

// AllocateContainerAddresses allocates a static addsubnetss for each of the
// container NICs in preparedInfo, hosted by the hostInstanceID. Returns the
// network config including all allocated addaddresses on success.
func (*environ) AllocateContainerAddresses(context.ProviderCallContext, instance.Id, names.MachineTag, network.InterfaceInfos) (network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("container address allocation")
}

// ReleaseContainerAddresses releases the previously allocated
// addaddresses matching the interface details passed in.
func (*environ) ReleaseContainerAddresses(context.ProviderCallContext, []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container address allocation")
}

// SSHAddresses filters the input addaddresses to those suitable for SSH use.
func (*environ) SSHAddresses(ctx context.ProviderCallContext, addresses network.SpaceAddresses) (network.SpaceAddresses, error) {
	return addresses, nil
}
