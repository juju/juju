// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package packet

import (
	"fmt"
	"net"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/names/v4"
)

var _ environs.Networking = (*environ)(nil)

// Subnets returns basic information about subnets known by the provider for
// the environment.
func (e *environ) Subnets(ctx context.ProviderCallContext, inst instance.Id, subnetIDs []network.Id) ([]network.SubnetInfo, error) {
	subnets := []network.SubnetInfo{}

	if inst != instance.UnknownId {
		device, _, err := e.packetClient.Devices.Get(string(inst), nil)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, n := range device.Network {
			subnetID, cidr, err := makeSubnetIDForNetwork("device-"+device.Hostname, n.Address, strconv.Itoa(n.CIDR))
			if err != nil {
				return nil, errors.Trace(err)
			}

			subnet := network.SubnetInfo{
				ProviderId:        network.Id(subnetID),
				ProviderNetworkId: network.Id(n.ID),
				CIDR:              cidr,
				VLANTag:           0,
			}

			subnets = append(subnets, subnet)
		}
	} else {
		projectID := e.cloud.Credential.Attributes()["project-id"]
		ips, _, err := e.packetClient.ProjectIPs.List(projectID, nil)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, ip := range ips {
			subnetID, cidr, err := makeSubnetIDForNetwork("project-"+projectID, ip.Address, strconv.Itoa(ip.CIDR))
			if err != nil {
				return nil, errors.Trace(err)
			}

			subnet := network.SubnetInfo{
				ProviderId:        network.Id(subnetID),
				ProviderNetworkId: network.Id(ip.ID), //TODO: figure out what the network ID should be???
				CIDR:              cidr,
				VLANTag:           0,
			}
			subnets = append(subnets, subnet)

		}

	}

	return subnets, nil
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

// SupportsSpaces returns whether the current environment supports
// spaces. The returned error satisfies errors.IsNotSupported(),
// unless a general API failure occurs.
func (e *environ) SupportsSpaces(context.ProviderCallContext) (bool, error) {
	return false, nil
}

// NetworkInterfaces returns a slice with the network interfaces that
// correspond to the given instance IDs. If no instances where found, but there
// was no other error, it will return ErrNoInstances. If some but not all of
// the instances were found, the returned slice will have some nil slots, and
// an ErrPartialInstances error will be returned.
func (e *environ) NetworkInterfaces(ctx context.ProviderCallContext, ids []instance.Id) ([]network.InterfaceInfos, error) {

	return nil, nil
}

// SuperSubnets returns information about aggregated subnet.
func (*environ) SuperSubnets(context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

// SupportsContainerAddresses returns true if the current environment is
// able to allocate addaddresses for containers.
func (*environ) SupportsContainerAddresses(context.ProviderCallContext) (bool, error) {
	return false, nil
}

// SSHAddresses filters the input addaddresses to those suitable for SSH use.
func (*environ) SSHAddresses(ctx context.ProviderCallContext, addresses network.SpaceAddresses) (network.SpaceAddresses, error) {
	return addresses, nil
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

// AllocateContainerAddresses allocates a static addsubnetss for each of the
// container NICs in preparedInfo, hosted by the hostInstanceID. Returns the
// network config including all allocated addaddresses on success.
func (e *environ) AllocateContainerAddresses(context.ProviderCallContext, instance.Id, names.MachineTag, network.InterfaceInfos) (network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("container address allocation")
}

// AreSpacesRoutable returns whether the communication between the
// two spaces can use cloud-local addaddresses.
func (*environ) AreSpacesRoutable(context.ProviderCallContext, *environs.ProviderSpaceInfo, *environs.ProviderSpaceInfo) (bool, error) {
	return false, errors.NotSupportedf("spaces")
}

// ProviderSpaceInfo returns the details of the space requested as
// a ProviderSpaceInfo.
func (*environ) ProviderSpaceInfo(context.ProviderCallContext, *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("spaces")
}

// ReleaseContainerAddresses releases the previously allocated
// addaddresses matching the interface details passed in.
func (*environ) ReleaseContainerAddresses(context.ProviderCallContext, []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container address allocation")
}
