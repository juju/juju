// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"net"

	"github.com/juju/collections/set"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
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
	ProviderId() network.Id
	ProviderNetworkId() network.Id
	AvailabilityZones() []string
	SpaceName() string
	SpaceID() string
	Life() state.Life
}

// BackingSubnetInfo describes a single subnet to be added in the
// backing store.
//
// TODO(dimitern): Replace state.SubnetInfo with this and remove
// BackingSubnetInfo, once the rest of state backing methods and the
// following pre-reqs are done:
//   - Subnets need a reference count to calculate Status.
//   - ensure EC2 and MAAS providers accept empty IDs as Subnets() args
//     and return all subnets, including the AvailabilityZones (for EC2;
//     empty for MAAS as zones are orthogonal to networks).
type BackingSubnetInfo struct {
	// ProviderId is a provider-specific network id. This may be empty.
	ProviderId network.Id

	// ProviderNetworkId is the id of the network containing this
	// subnet from the provider's perspective. It can be empty if the
	// provider doesn't support distinct networks.
	ProviderNetworkId network.Id

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

	// Live holds the life of the subnet
	Life life.Value
}

// BackingSpace defines the methods supported by a Space entity stored
// persistently.
type BackingSpace interface {
	// Id returns the ID of the space.
	Id() string

	// Name returns the space name.
	Name() string

	// NetworkSpace maps the space into network.SpaceInfo
	NetworkSpace() (network.SpaceInfo, error)

	// ProviderId returns the network ID of the provider
	ProviderId() network.Id
}

// NetworkBacking defines the methods needed by the API facade to store and
// retrieve information from the underlying persistence layer (state
// DB).
type NetworkBacking interface {
	// ModelConfig returns the current model configuration.
	ModelConfig() (*config.Config, error)

	// CloudSpec returns a cloud specification.
	CloudSpec() (environscloudspec.CloudSpec, error)

	// AvailabilityZones returns all cached availability zones (i.e.
	// not from the provider, but in state).
	AvailabilityZones() (network.AvailabilityZones, error)

	// SetAvailabilityZones replaces the cached list of availability
	// zones with the given zones.
	SetAvailabilityZones(network.AvailabilityZones) error

	// AddSpace creates a space
	AddSpace(string, network.Id, []string, bool) (BackingSpace, error)

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
		SpaceTag:          names.NewSpaceTag(subnet.SpaceName()).String(),
		Life:              subnet.Life().Value(),
	}
}

func SubnetInfoToParamsSubnet(subnet network.SubnetInfo) params.Subnet {
	return params.Subnet{
		CIDR:              subnet.CIDR,
		VLANTag:           subnet.VLANTag,
		ProviderId:        subnet.ProviderId.String(),
		ProviderNetworkId: subnet.ProviderNetworkId.String(),
		Zones:             subnet.AvailabilityZones,
		SpaceTag:          names.NewSpaceTag(subnet.SpaceName).String(),
		Life:              subnet.Life,
	}
}

// NetworkInterfacesToStateArgs splits the given interface list into a slice of
// state.LinkLayerDeviceArgs and a slice of state.LinkLayerDeviceAddress.
func NetworkInterfacesToStateArgs(devs network.InterfaceInfos) (
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

		if dev.PrimaryAddress().Value == "" {
			continue
		}
		devicesAddrs = append(devicesAddrs, networkAddressesToStateArgs(dev, dev.Addresses)...)
	}
	logger.Tracef("seen devices: %+v", seenDeviceNames.SortedValues())
	logger.Tracef("network interface list transformed to state args:\n%+v\n%+v", devicesArgs, devicesAddrs)
	return devicesArgs, devicesAddrs
}

func networkDeviceToStateArgs(dev network.InterfaceInfo) state.LinkLayerDeviceArgs {
	var mtu uint
	if dev.MTU >= 0 {
		mtu = uint(dev.MTU)
	}

	return state.LinkLayerDeviceArgs{
		Name:            dev.InterfaceName,
		MTU:             mtu,
		ProviderID:      dev.ProviderId,
		Type:            dev.InterfaceType,
		MACAddress:      dev.MACAddress,
		IsAutoStart:     !dev.NoAutoStart,
		IsUp:            !dev.Disabled,
		ParentName:      dev.ParentInterfaceName,
		VirtualPortType: dev.VirtualPortType,
	}
}

// networkAddressStateArgsForDevice accommodates the
// fact that network configuration is sometimes supplied
// with a duplicated device for each address.
// This is a normalisation that returns state args for all
// addresses of interfaces with the input name.
func networkAddressStateArgsForDevice(devs network.InterfaceInfos, name string) []state.LinkLayerDeviceAddress {
	var res []state.LinkLayerDeviceAddress

	for _, dev := range devs.GetByName(name) {
		if dev.PrimaryAddress().Value == "" {
			continue
		}
		res = append(res, networkAddressesToStateArgs(dev, dev.Addresses)...)
	}

	return res
}

func networkAddressesToStateArgs(
	dev network.InterfaceInfo, addrs []network.ProviderAddress,
) []state.LinkLayerDeviceAddress {
	var res []state.LinkLayerDeviceAddress

	for _, addr := range addrs {
		cidrAddress, err := addr.ValueWithMask()
		if err != nil {
			logger.Infof("ignoring address %q for device %q: %v", addr.Value, dev.InterfaceName, err)
			continue
		}

		// Prefer the config method supplied with the address.
		// Fallback first to the device, then to "static".
		configType := addr.AddressConfigType()
		if configType == network.ConfigUnknown {
			configType = dev.ConfigType
		}
		if configType == network.ConfigUnknown {
			configType = network.ConfigStatic
		}

		res = append(res, state.LinkLayerDeviceAddress{
			DeviceName:        dev.InterfaceName,
			ProviderID:        dev.ProviderAddressId,
			ProviderNetworkID: dev.ProviderNetworkId,
			ProviderSubnetID:  dev.ProviderSubnetId,
			ConfigMethod:      configType,
			CIDRAddress:       cidrAddress,
			DNSServers:        dev.DNSServers.Values(),
			DNSSearchDomains:  dev.DNSSearchDomains,
			GatewayAddress:    dev.GatewayAddress.Value,
			IsDefaultGateway:  dev.IsDefaultGateway,
			Origin:            dev.Origin,
			IsSecondary:       addr.IsSecondary,
		})
	}

	return res
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
