// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"

	lxdapi "github.com/canonical/lxd/shared/api"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

var _ environs.Networking = (*environ)(nil)

// Subnets returns basic information about subnets known by the provider for
// the environment.
func (e *environ) Subnets(ctx context.Context, subnetIDs []network.Id) ([]network.SubnetInfo, error) {
	srv := e.server()

	availabilityZones, err := e.AvailabilityZones(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "retrieving lxd availability zones")
	}

	networks, err := srv.GetNetworks()
	if err != nil {
		if isErrMissingAPIExtension(err, "network") {
			return nil, errors.NewNotSupported(nil, `subnet discovery requires the "network" extension to be enabled on the lxd server`)
		}
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
		subnets       []network.SubnetInfo
		uniqueSubnets = set.NewStrings()
	)
	for _, networkDetails := range networks {
		if networkDetails.Type != "bridge" {
			continue
		}

		networkName := networkDetails.Name
		state, err := srv.GetNetworkState(networkName)
		if err != nil {
			if isErrMissingAPIExtension(err, "network_state") {
				return nil, errors.Errorf("network_state extension unsupported; upgrade to a newer version of LXD")
			}
			return nil, errors.Annotatef(err, "querying lxd server for state of network %q", networkName)
		}

		// We are only interested in networks that are up.
		if state.State != "up" {
			continue
		}

		for _, stateAddr := range state.Addresses {
			netAddr := network.NewMachineAddress(stateAddr.Address).AsProviderAddress()
			if netAddr.Scope == network.ScopeLinkLocal || netAddr.Scope == network.ScopeMachineLocal {
				continue
			}

			_, netCIDR, err := net.ParseCIDR(fmt.Sprintf("%s/%s", stateAddr.Address, stateAddr.Netmask))
			if err != nil {
				return nil, errors.Annotatef(err, "calculating CIDR for  %q", stateAddr.Address)
			}
			cidr := netCIDR.String()

			if uniqueSubnets.Contains(cidr) {
				continue
			} else if keepList != nil && !keepList.Contains(cidr) {
				continue
			}

			uniqueSubnets.Add(cidr)
			subnets = append(subnets, makeSubnetInfo(cidr, networkName, availabilityZones))
		}
	}

	return subnets, nil
}

func makeSubnetInfo(cidr, networkName string, availabilityZones network.AvailabilityZones) network.SubnetInfo {
	azNames := transform.Slice(availabilityZones, func(az network.AvailabilityZone) string { return az.Name() })
	return network.SubnetInfo{
		ProviderId:        network.Id(cidr),
		ProviderNetworkId: network.Id(networkName),
		CIDR:              cidr,
		VLANTag:           0,
		AvailabilityZones: azNames,
	}
}

// NetworkInterfaces returns a slice with the network interfaces that
// correspond to the given instance IDs. If no instances where found, but there
// was no other error, it will return ErrNoInstances. If some but not all of
// the instances were found, the returned slice will have some nil slots, and
// an ErrPartialInstances error will be returned.
func (e *environ) NetworkInterfaces(_ context.Context, ids []instance.Id) ([]network.InterfaceInfos, error) {
	var (
		missing int
		srv     = e.server()
		res     = make([]network.InterfaceInfos, len(ids))
	)

	for instIdx, id := range ids {
		state, _, err := srv.GetInstanceState(string(id))
		if err != nil {
			if isErrNotFound(err) {
				missing++
				continue
			}
			return nil, errors.Annotatef(err, "retrieving network interface info for instance %q", id)
		} else if len(state.Network) == 0 {
			continue
		}

		// Sort interfaces by name to ensure consistent device indexes
		// across calls when we iterate the container's network map.
		interfaceNames := make([]string, 0, len(state.Network))
		for net := range state.Network {
			interfaceNames = append(interfaceNames, net)
		}
		sort.Strings(interfaceNames)

		var devIdx int
		for _, interfaceName := range interfaceNames {
			netInfo := state.Network[interfaceName]

			// Ignore loopback devices
			if detectInterfaceType(netInfo.Type) == network.LoopbackDevice {
				continue
			}

			ni, err := makeInterfaceInfo(interfaceName, netInfo)
			if err != nil {
				return nil, errors.Annotatef(err, "retrieving network interface info for instance %q", id)
			} else if len(ni.Addresses) == 0 {
				continue
			}

			ni.DeviceIndex = devIdx
			devIdx++
			res[instIdx] = append(res[instIdx], ni)
		}
	}

	if missing > 0 {
		// Found at least one instance.
		if missing != len(res) {
			return res, environs.ErrPartialInstances
		}

		return nil, environs.ErrNoInstances
	}
	return res, nil
}

func makeInterfaceInfo(interfaceName string, netInfo lxdapi.InstanceStateNetwork) (network.InterfaceInfo, error) {
	var ni = network.InterfaceInfo{
		MACAddress:    netInfo.Hwaddr,
		MTU:           netInfo.Mtu,
		InterfaceName: interfaceName,
		InterfaceType: detectInterfaceType(netInfo.Type),
		Origin:        network.OriginProvider,
	}

	// We cannot tell from the API response whether the
	// interface uses a static or DHCP configuration.
	// Assume static unless this is a loopback device.
	configType := network.ConfigStatic
	if ni.InterfaceType == network.LoopbackDevice {
		configType = network.ConfigLoopback
	}

	// Iterate the list of addresses assigned to this interface ignoring
	// any link-local ones. The first non link-local address is treated as
	// the primary address and is used to populate the interface CIDR and
	// subnet ID fields.
	for _, addr := range netInfo.Addresses {
		netAddr := network.NewMachineAddress(addr.Address).AsProviderAddress()
		if netAddr.Scope == network.ScopeLinkLocal || netAddr.Scope == network.ScopeMachineLocal {
			continue
		}

		_, netCIDR, err := net.ParseCIDR(fmt.Sprintf("%s/%s", addr.Address, addr.Netmask))
		if err != nil {
			return network.InterfaceInfo{}, errors.Annotatef(err, "calculating CIDR for interface %q", interfaceName)
		}

		cidr := netCIDR.String()
		netAddr.CIDR = cidr
		netAddr.ConfigType = configType
		ni.Addresses = append(ni.Addresses, netAddr)
	}

	return ni, nil
}

func detectInterfaceType(lxdIfaceType string) network.LinkLayerDeviceType {
	switch lxdIfaceType {
	case "bridge":
		return network.BridgeDevice
	case "broadcast":
		return network.EthernetDevice
	case "loopback":
		return network.LoopbackDevice
	default:
		return network.UnknownDevice
	}
}

// isErrNotFound returns true if the LXD server returned back a "not found" error.
func isErrNotFound(err error) bool {
	// Unfortunately the lxd client does not expose error
	// codes so we need to match against a string here.
	return err != nil && strings.Contains(err.Error(), "not found")
}

// isErrMissingAPIExtension returns true if the LXD server returned back an
// "API extension not found" error.
func isErrMissingAPIExtension(err error, ext string) bool {
	// Unfortunately the lxd client does not expose error
	// codes so we need to match against a string here.
	return err != nil && strings.Contains(err.Error(), fmt.Sprintf("server is missing the required %q API extension", ext))
}

// SupportsSpaces returns whether the current environment supports
// spaces. The returned error satisfies errors.IsNotSupported(),
// unless a general API failure occurs.
func (e *environ) SupportsSpaces() (bool, error) {
	// Really old lxd versions (e.g. xenial/ppc64) do not even support the
	// network API extension so the subnet discovery code path will not
	// work there.
	return e.server().HasExtension("network"), nil
}
