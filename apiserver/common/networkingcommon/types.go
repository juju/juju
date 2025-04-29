// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// NetworkBacking defines the methods needed by the API facade to store and
// retrieve information from the underlying persistence layer (state
// DB).
type NetworkBacking interface {
	environs.EnvironConfigGetter

	// AvailabilityZones returns all cached availability zones (i.e.
	// not from the provider, but in state).
	AvailabilityZones() (network.AvailabilityZones, error)

	// SetAvailabilityZones replaces the cached list of availability
	// zones with the given zones.
	SetAvailabilityZones(network.AvailabilityZones) error

	// ModelTag returns the tag of the model this state is associated to.
	ModelTag() names.ModelTag
}

// BackingSubnetToParamsSubnetV2 converts a network backing subnet to the new
// version of the subnet API parameter.
func BackingSubnetToParamsSubnetV2(subnet network.SubnetInfo) params.SubnetV2 {
	return params.SubnetV2{
		ID:     subnet.ID.String(),
		Subnet: BackingSubnetToParamsSubnet(subnet),
	}
}

func BackingSubnetToParamsSubnet(subnet network.SubnetInfo) params.Subnet {
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

	logger.Tracef(context.TODO(), "transforming network interface list to state args: %+v", devs)
	seenDeviceNames := set.NewStrings()
	for _, dev := range devs {
		logger.Tracef(context.TODO(), "transforming device %q", dev.InterfaceName)
		if !seenDeviceNames.Contains(dev.InterfaceName) {
			// First time we see this, add it to devicesArgs.
			seenDeviceNames.Add(dev.InterfaceName)

			args := networkDeviceToStateArgs(dev)
			logger.Tracef(context.TODO(), "state device args for device: %+v", args)
			devicesArgs = append(devicesArgs, args)
		}

		if dev.PrimaryAddress().Value == "" {
			continue
		}
		devicesAddrs = append(devicesAddrs, networkAddressesToStateArgs(dev, dev.Addresses)...)
	}
	logger.Tracef(context.TODO(), "seen devices: %+v", seenDeviceNames.SortedValues())
	logger.Tracef(context.TODO(), "network interface list transformed to state args:\n%+v\n%+v", devicesArgs, devicesAddrs)
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
			logger.Infof(context.TODO(), "ignoring address %q for device %q: %v", addr.Value, dev.InterfaceName, err)
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
			DeviceName:       dev.InterfaceName,
			ProviderID:       dev.ProviderAddressId,
			ProviderSubnetID: dev.ProviderSubnetId,
			ConfigMethod:     configType,
			CIDRAddress:      cidrAddress,
			DNSServers:       dev.DNSServers,
			DNSSearchDomains: dev.DNSSearchDomains,
			GatewayAddress:   dev.GatewayAddress.Value,
			IsDefaultGateway: dev.IsDefaultGateway,
			Origin:           dev.Origin,
			IsSecondary:      addr.IsSecondary,
		})
	}

	return res
}
