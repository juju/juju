// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/packethost/packngo"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
)

var _ environs.Networking = (*environ)(nil)

// Subnets returns basic information about subnets known by the provider for
// the environment.
func (e *environ) Subnets(ctx envcontext.ProviderCallContext, inst instance.Id, subnetIDs []network.Id) ([]network.SubnetInfo, error) {
	attrs := e.cloud.Credential.Attributes()
	if attrs == nil {
		return nil, errors.Trace(fmt.Errorf("empty attribute credentials"))
	}
	// We checked the presence of project-id when we were verifying the credentials.
	projectID := attrs["project-id"]
	ips, err := e.listIPsByProjectIDAndRegion(projectID, e.cloud.Region)
	if err != nil {
		return nil, errors.Trace(err)
	}

	filterSet := set.NewStrings()
	for _, i := range subnetIDs {
		filterSet.Add(i.String())
	}

	var projectSubnets []network.SubnetInfo
	for _, ipblock := range ips {
		subnetID, cidr, err := makeSubnetIDForNetwork(ipblock.ID, ipblock.Network, ipblock.CIDR)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if !filterSet.IsEmpty() && !filterSet.Contains(subnetID) {
			continue
		}

		subnet := network.SubnetInfo{
			ProviderId:        network.Id(subnetID),
			ProviderNetworkId: network.Id(ipblock.ID),
			CIDR:              cidr, VLANTag: 0,
			AvailabilityZones: []string{e.cloud.Region},
		}
		projectSubnets = append(projectSubnets, subnet)
	}

	if inst == instance.UnknownId {
		return projectSubnets, nil
	}

	device, _, err := e.equinixClient.Devices.Get(string(inst), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var instanceSubnets []network.SubnetInfo
nextSubnet: // API client limitation since we can't get the actual blocks for individual instance we have to do this
	for _, psub := range projectSubnets {
		_, ipnet, err := net.ParseCIDR(psub.CIDR)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, n := range device.Network {
			if ipnet.Contains(net.ParseIP(n.Address)) {
				instanceSubnets = append(instanceSubnets, psub)
				continue nextSubnet
			}
		}
	}

	return instanceSubnets, nil
}

func makeSubnetIDForNetwork(networkName, address string, mask int) (string, string, error) {
	_, netCIDR, err := net.ParseCIDR(fmt.Sprintf("%s/%d", address, mask))
	if err != nil {
		return "", "", errors.Annotatef(err, "calculating CIDR for network %q", networkName)
	}

	cidr := netCIDR.String()
	subnetID := fmt.Sprintf("subnet-%s", networkName)
	return subnetID, cidr, nil
}

// SupportsSpaces returns whether the current environment supports
// spaces. The returned error satisfies errors.IsNotSupported(),
// unless a general API failure occurs.
func (e *environ) SupportsSpaces(envcontext.ProviderCallContext) (bool, error) {
	return true, nil
}

// NetworkInterfaces returns a slice with the network interfaces that
// correspond to the given instance IDs. If no instances where found, but there
// was no other error, it will return ErrNoInstances. If some but not all of
// the instances were found, the returned slice will have some nil slots, and
// an ErrPartialInstances error will be returned.
func (e *environ) NetworkInterfaces(ctx envcontext.ProviderCallContext, ids []instance.Id) ([]network.InterfaceInfos, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}
	infos := make([]network.InterfaceInfos, len(ids))
	for i, id := range ids {
		device, _, err := e.equinixClient.Devices.Get(string(id), nil)
		if err != nil {
			return nil, err
		}

		dev := newInstance(device, e)

		subnets, err := e.Subnets(ctx, id, nil)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if len(subnets) == 0 {
			return nil, errors.Trace(fmt.Errorf("Instance does not have subnets"))
		}

		deviceAddresses, err := dev.Addresses(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for portIdx, port := range device.NetworkPorts {
			current := network.InterfaceInfo{
				DeviceIndex:       portIdx,
				ProviderId:        network.Id(port.ID),
				AvailabilityZones: subnets[0].AvailabilityZones,
				InterfaceType:     network.EthernetDevice,
				Disabled:          false,
				NoAutoStart:       false,
				// Equinix Metal only provides DHCP for the public IPV4
				ConfigType:    network.ConfigStatic,
				Origin:        network.OriginProvider,
				InterfaceName: port.Name,
				MACAddress:    port.Data.MAC,
			}

			if strings.HasPrefix(port.Name, "bond") {
				current.InterfaceType = network.BondDevice
			}

			if port.Name == "bond0" && port.Data.Bonded {
				current.Addresses = deviceAddresses
			}

			// Even if this looks mutually exclusive we know from a domain
			// prospective that if bond0 is bonded eth0 is always bonded as
			// well.
			if port.Name == "eth0" && port.Data.Bonded == false {
				current.Addresses = deviceAddresses
			}

			if port.NetworkType == "layer2" {
				current.ConfigType = network.ConfigManual
				current.Addresses = network.ProviderAddresses{}
			}

			infos[i] = append(infos[i], current)
		}
	}
	return infos, nil
}

// SuperSubnets returns information about the reserved private subnets that can
// be used as underlays when setting up FAN networking.
func (e *environ) SuperSubnets(envcontext.ProviderCallContext) ([]string, error) {
	attrs := e.cloud.Credential.Attributes()
	if attrs == nil {
		return nil, errors.Trace(fmt.Errorf("empty attribute credentials"))
	}
	// We checked the presence of project-id when we were verifying the credentials.
	projectID := attrs["project-id"]

	ips, err := e.listIPsByProjectIDAndRegion(projectID, e.cloud.Region)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var privateCIDRs []string
	for _, ipblock := range ips {
		if ipblock.Public {
			continue // we are only interested in private block reservations from the right region
		}
		privateCIDRs = append(privateCIDRs, fmt.Sprintf("%s/%d", ipblock.Network, ipblock.CIDR))
	}

	return privateCIDRs, nil
}

// SupportsContainerAddresses returns true if the current environment is
// able to allocate addaddresses for containers.
func (*environ) SupportsContainerAddresses(envcontext.ProviderCallContext) (bool, error) {
	return false, nil
}

// SupportsSpaceDiscovery returns whether the current environment
// supports discovering spaces from the provider. The returned error
// satisfies errors.IsNotSupported(), unless a general API failure occurs.
func (*environ) SupportsSpaceDiscovery(envcontext.ProviderCallContext) (bool, error) {
	return false, nil
}

// Spaces returns a slice of network.SpaceInfo with info, including
// details of all associated subnets, about all spaces known to the
// provider that have subnets available.
func (*environ) Spaces(envcontext.ProviderCallContext) (network.SpaceInfos, error) {
	return nil, errors.NotSupportedf("spaces")
}

// AllocateContainerAddresses allocates a static addsubnetss for each of the
// container NICs in preparedInfo, hosted by the hostInstanceID. Returns the
// network config including all allocated addaddresses on success.
func (e *environ) AllocateContainerAddresses(envcontext.ProviderCallContext, instance.Id, names.MachineTag, network.InterfaceInfos) (network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("container address allocation")
}

// AreSpacesRoutable returns whether the communication between the
// two spaces can use cloud-local addaddresses.
func (*environ) AreSpacesRoutable(envcontext.ProviderCallContext, *environs.ProviderSpaceInfo, *environs.ProviderSpaceInfo) (bool, error) {
	return false, errors.NotSupportedf("spaces")
}

// ProviderSpaceInfo returns the details of the space requested as
// a ProviderSpaceInfo.
func (*environ) ProviderSpaceInfo(envcontext.ProviderCallContext, *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("spaces")
}

// ReleaseContainerAddresses releases the previously allocated
// addaddresses matching the interface details passed in.
func (*environ) ReleaseContainerAddresses(envcontext.ProviderCallContext, []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container address allocation")
}

// listIPsByProjectIDAndRegion returns a list of reserved ip addressed filtered by projectID and region
func (e *environ) listIPsByProjectIDAndRegion(projectID, region string) ([]packngo.IPAddressReservation, error) {
	ips, _, err := e.equinixClient.ProjectIPs.List(projectID, &packngo.ListOptions{
		Includes: []string{"available_in_metros"},
	})
	if err != nil {
		return nil, err
	}
	var result []packngo.IPAddressReservation
	for _, ipblock := range ips {
		metro := ""
		if ipblock.Metro != nil {
			metro = ipblock.Metro.Code
		} else if ipblock.Facility != nil && ipblock.Facility.Metro != nil {
			metro = ipblock.Facility.Metro.Code
		}
		if metro == e.cloud.Region {
			result = append(result, ipblock)
		}
	}
	return result, nil
}
