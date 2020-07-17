// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"net"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// BackingSubnet defines the methods supported by a Subnet entity
// stored persistently.
//
// TODO(dimitern): Once the state backing is implemented, remove this
// and just use *state.Subnet.
type BackingSubnet interface {
	ID() string
	CIDR() string
	VLANTag() int
	ProviderId() corenetwork.Id
	ProviderNetworkId() corenetwork.Id
	AvailabilityZones() []string
	Status() string
	SpaceName() string
	SpaceID() string
	Life() life.Value
}

// BackingSubnetInfo describes a single subnet to be added in the
// backing store.
//
// TODO(dimitern): Replace state.SubnetInfo with this and remove
// BackingSubnetInfo, once the rest of state backing methods and the
// following pre-reqs are done:
// * Subnets need a reference count to calculate Status.
// * ensure EC2 and MAAS providers accept empty IDs as Subnets() args
//   and return all subnets, including the AvailabilityZones (for EC2;
//   empty for MAAS as zones are orthogonal to networks).
type BackingSubnetInfo struct {
	// ProviderId is a provider-specific network id. This may be empty.
	ProviderId corenetwork.Id

	// ProviderNetworkId is the id of the network containing this
	// subnet from the provider's perspective. It can be empty if the
	// provider doesn't support distinct networks.
	ProviderNetworkId corenetwork.Id

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for normal
	// networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// AvailabilityZones describes which availability zone(s) this
	// subnet is in. It can be empty if the provider does not support
	// availability zones.
	AvailabilityZones []string

	// SpaceName holds the juju network space this subnet is
	// associated with. Can be empty if not supported.
	SpaceName string
	SpaceID   string

	// Status holds the status of the subnet. Normally this will be
	// calculated from the reference count and Life of a subnet.
	Status string

	// Live holds the life of the subnet
	Life life.Value
}

// BackingSpace defines the methods supported by a Space entity stored
// persistently.
type BackingSpace interface {
	// ID returns the ID of the space.
	Id() string

	// Name returns the space name.
	Name() string

	// Subnets returns the subnets in the space
	Subnets() ([]BackingSubnet, error)

	// ProviderId returns the network ID of the provider
	ProviderId() corenetwork.Id
}

// NetworkBacking defines the methods needed by the API facade to store and
// retrieve information from the underlying persistence layer (state
// DB).
type NetworkBacking interface {
	environs.EnvironConfigGetter

	// AvailabilityZones returns all cached availability zones (i.e.
	// not from the provider, but in state).
	AvailabilityZones() (corenetwork.AvailabilityZones, error)

	// SetAvailabilityZones replaces the cached list of availability
	// zones with the given zones.
	SetAvailabilityZones(corenetwork.AvailabilityZones) error

	// AddSpace creates a space
	AddSpace(string, corenetwork.Id, []string, bool) (BackingSpace, error)

	// AllSpaces returns all known Juju network spaces.
	AllSpaces() ([]BackingSpace, error)

	// AddSubnet creates a backing subnet for an existing subnet.
	AddSubnet(BackingSubnetInfo) (BackingSubnet, error)

	// AllSubnets returns all backing subnets.
	AllSubnets() ([]BackingSubnet, error)

	SubnetByCIDR(cidr string) (BackingSubnet, error)

	// ModelTag returns the tag of the model this state is associated to.
	ModelTag() names.ModelTag
}

// BackingSubnetToParamsSubnetV2 converts a network backing subnet to the new
// version of the subnet API parameter.
func BackingSubnetToParamsSubnetV2(subnet BackingSubnet) params.SubnetV2 {
	return params.SubnetV2{
		ID:     subnet.ID(),
		Subnet: BackingSubnetToParamsSubnet(subnet),
	}
}

func BackingSubnetToParamsSubnet(subnet BackingSubnet) params.Subnet {
	return params.Subnet{
		CIDR:              subnet.CIDR(),
		VLANTag:           subnet.VLANTag(),
		ProviderId:        subnet.ProviderId().String(),
		ProviderNetworkId: subnet.ProviderNetworkId().String(),
		Zones:             subnet.AvailabilityZones(),
		Status:            subnet.Status(),
		SpaceTag:          names.NewSpaceTag(subnet.SpaceName()).String(),
		Life:              subnet.Life(),
	}
}

// NetworkInterfacesToStateArgs splits the given interface list into a slice of
// state.LinkLayerDeviceArgs and a slice of state.LinkLayerDeviceAddress.
func NetworkInterfacesToStateArgs(devs corenetwork.InterfaceInfos) (
	[]state.LinkLayerDeviceArgs,
	[]state.LinkLayerDeviceAddress,
) {
	var devicesArgs []state.LinkLayerDeviceArgs
	var devicesAddrs []state.LinkLayerDeviceAddress

	logger.Tracef("transforming network interface list to state args: %+v", devs)
	seenDeviceNames := set.NewStrings()
	for _, dev := range devs {
		logger.Tracef("transforming device %q", dev.InterfaceName)
		if !seenDeviceNames.Contains(dev.InterfaceName) {
			// First time we see this, add it to devicesArgs.
			seenDeviceNames.Add(dev.InterfaceName)

			args := networkDeviceToStateArgs(dev)
			logger.Tracef("state device args for device: %+v", args)
			devicesArgs = append(devicesArgs, args)
		}

		addr, err := networkAddressToStateArgs(dev, dev.PrimaryAddress())
		if err != nil {
			logger.Warningf("ignoring address for device %q: %v", dev.InterfaceName, err)
			continue
		}

		logger.Tracef("state address args for device: %+v", addr)
		devicesAddrs = append(devicesAddrs, addr)
	}
	logger.Tracef("seen devices: %+v", seenDeviceNames.SortedValues())
	logger.Tracef("network interface list transformed to state args:\n%+v\n%+v", devicesArgs, devicesAddrs)
	return devicesArgs, devicesAddrs
}

func networkDeviceToStateArgs(dev corenetwork.InterfaceInfo) state.LinkLayerDeviceArgs {
	var mtu uint
	if dev.MTU >= 0 {
		mtu = uint(dev.MTU)
	}

	return state.LinkLayerDeviceArgs{
		Name:            dev.InterfaceName,
		MTU:             mtu,
		ProviderID:      dev.ProviderId,
		Type:            corenetwork.LinkLayerDeviceType(dev.InterfaceType),
		MACAddress:      dev.MACAddress,
		IsAutoStart:     !dev.NoAutoStart,
		IsUp:            !dev.Disabled,
		ParentName:      dev.ParentInterfaceName,
		VirtualPortType: dev.VirtualPortType,
	}
}

// networkAddressStateArgsForHWAddr accommodates the fact that network
// configuration is sometimes supplied with a duplicated device for each
// address.
// This is a normalisation that returns state args for all primary addresses
// of interfaces with the input hardware address.
func networkAddressStateArgsForHWAddr(devs corenetwork.InterfaceInfos, hwAddr string) []state.LinkLayerDeviceAddress {
	var res []state.LinkLayerDeviceAddress

	for _, dev := range devs.GetByHardwareAddress(hwAddr) {
		addr, err := networkAddressToStateArgs(dev, dev.PrimaryAddress())
		if err != nil {
			logger.Warningf("ignoring address for device %q: %v", dev.InterfaceName, err)
			continue
		}
		res = append(res, addr)
	}

	return res
}

func networkAddressToStateArgs(
	dev corenetwork.InterfaceInfo, addr corenetwork.ProviderAddress,
) (state.LinkLayerDeviceAddress, error) {
	cidrAddress, err := addr.ValueForCIDR(dev.CIDR)
	if err != nil {
		return state.LinkLayerDeviceAddress{}, errors.Trace(err)
	}

	var derivedConfigMethod corenetwork.AddressConfigMethod
	switch method := corenetwork.AddressConfigMethod(dev.ConfigType); method {
	case corenetwork.StaticAddress, corenetwork.DynamicAddress,
		corenetwork.LoopbackAddress, corenetwork.ManualAddress:
		derivedConfigMethod = method
	case "dhcp": // awkward special case
		derivedConfigMethod = corenetwork.DynamicAddress
	default:
		derivedConfigMethod = corenetwork.StaticAddress
	}

	return state.LinkLayerDeviceAddress{
		DeviceName:        dev.InterfaceName,
		ProviderID:        dev.ProviderAddressId,
		ProviderNetworkID: dev.ProviderNetworkId,
		ProviderSubnetID:  dev.ProviderSubnetId,
		ConfigMethod:      derivedConfigMethod,
		CIDRAddress:       cidrAddress,
		DNSServers:        dev.DNSServers.ToIPAddresses(),
		DNSSearchDomains:  dev.DNSSearchDomains,
		GatewayAddress:    dev.GatewayAddress.Value,
		IsDefaultGateway:  dev.IsDefaultGateway,
	}, nil
}

// NetworkConfigSource defines the necessary calls to obtain the network
// configuration of a machine.
type NetworkConfigSource interface {
	// SysClassNetPath returns the Linux kernel userspace SYSFS path used by
	// this source. DefaultNetworkConfigSource() uses network.SysClassNetPath.
	SysClassNetPath() string

	// Interfaces returns information about all network interfaces on the
	// machine as []net.Interface.
	Interfaces() ([]net.Interface, error)

	// InterfaceAddresses returns information about all addresses assigned to
	// the network interface with the given name.
	InterfaceAddresses(name string) ([]net.Addr, error)
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

func MachineNetworkInfoResultToNetworkInfoResult(inResult state.MachineNetworkInfoResult) params.NetworkInfoResult {
	if inResult.Error != nil {
		return params.NetworkInfoResult{Error: apiservererrors.ServerError(inResult.Error)}
	}
	infos := make([]params.NetworkInfo, len(inResult.NetworkInfos))
	for i, info := range inResult.NetworkInfos {
		infos[i] = networkToParamsNetworkInfo(info)
	}
	return params.NetworkInfoResult{
		Info: infos,
	}
}

func FanConfigToFanConfigResult(config network.FanConfig) params.FanConfigResult {
	result := params.FanConfigResult{Fans: make([]params.FanConfigEntry, len(config))}
	for i, entry := range config {
		result.Fans[i] = params.FanConfigEntry{Underlay: entry.Underlay.String(), Overlay: entry.Overlay.String()}
	}
	return result
}

func FanConfigResultToFanConfig(config params.FanConfigResult) (network.FanConfig, error) {
	rv := make(network.FanConfig, len(config.Fans))
	for i, entry := range config.Fans {
		_, ipNet, err := net.ParseCIDR(entry.Underlay)
		if err != nil {
			return nil, err
		}
		rv[i].Underlay = ipNet
		_, ipNet, err = net.ParseCIDR(entry.Overlay)
		if err != nil {
			return nil, err
		}
		rv[i].Overlay = ipNet
	}
	return rv, nil
}
