// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

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

// MachineNetworkConfigToDomain transforms network config wire params to network
// interfaces recognised by the network domain.
func MachineNetworkConfigToDomain(args []params.NetworkConfig) ([]domainnetwork.NetInterface, error) {
	nics := make([]domainnetwork.NetInterface, len(args))

	for i, arg := range args {
		nics[i] = domainnetwork.NetInterface{
			Name:             arg.InterfaceName,
			MTU:              nilIfEmpty(int64(arg.MTU)),
			MACAddress:       nilIfEmpty(arg.MACAddress),
			ProviderID:       nilIfEmpty(network.Id(arg.ProviderId)),
			Type:             network.LinkLayerDeviceType(arg.InterfaceType),
			VirtualPortType:  network.VirtualPortType(arg.VirtualPortType),
			IsAutoStart:      !arg.NoAutoStart,
			IsEnabled:        !arg.Disabled,
			ParentDeviceName: arg.ParentInterfaceName,
			GatewayAddress:   nilIfEmpty(arg.GatewayAddress),
			IsDefaultGateway: false,
			VLANTag:          uint64(arg.VLANTag),
			DNSSearchDomains: arg.DNSSearchDomains,
			DNSAddresses:     arg.DNSServers,
		}
	}

	return nics, nil
}

func nilIfEmpty[T comparable](in T) *T {
	var empty T
	if in == empty {
		return nil
	}
	return &in
}

// NetworkInterfacesToStateArgs splits the given interface list into a slice of
// state.LinkLayerDeviceArgs and a slice of state.LinkLayerDeviceAddress.
func NetworkInterfacesToStateArgs(devs network.InterfaceInfos) (
	[]state.LinkLayerDeviceArgs,
	[]state.LinkLayerDeviceAddress,
) {
	var devicesArgs []state.LinkLayerDeviceArgs
	var devicesAddrs []state.LinkLayerDeviceAddress

	ctx := context.TODO()

	logger.Tracef(ctx, "transforming network interface list to state args: %+v", devs)
	seenDeviceNames := set.NewStrings()
	for _, dev := range devs {
		logger.Tracef(ctx, "transforming device %q", dev.InterfaceName)
		if !seenDeviceNames.Contains(dev.InterfaceName) {
			// First time we see this, add it to devicesArgs.
			seenDeviceNames.Add(dev.InterfaceName)

			args := networkDeviceToStateArgs(dev)
			logger.Tracef(ctx, "state device args for device: %+v", args)
			devicesArgs = append(devicesArgs, args)
		}

		if dev.PrimaryAddress().Value == "" {
			continue
		}
		devicesAddrs = append(devicesAddrs, networkAddressesToStateArgs(ctx, dev, dev.Addresses)...)
	}
	logger.Tracef(ctx, "seen devices: %+v", seenDeviceNames.SortedValues())
	logger.Tracef(ctx, "network interface list transformed to state args:\n%+v\n%+v", devicesArgs, devicesAddrs)
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
func networkAddressStateArgsForDevice(ctx context.Context, devs network.InterfaceInfos, name string) []state.LinkLayerDeviceAddress {
	var res []state.LinkLayerDeviceAddress

	for _, dev := range devs.GetByName(name) {
		if dev.PrimaryAddress().Value == "" {
			continue
		}
		res = append(res, networkAddressesToStateArgs(ctx, dev, dev.Addresses)...)
	}

	return res
}

func networkAddressesToStateArgs(
	ctx context.Context,
	dev network.InterfaceInfo, addrs []network.ProviderAddress,
) []state.LinkLayerDeviceAddress {
	var res []state.LinkLayerDeviceAddress

	for _, addr := range addrs {
		cidrAddress, err := addr.ValueWithMask()
		if err != nil {
			logger.Infof(ctx, "ignoring address %q for device %q: %v", addr.Value, dev.InterfaceName, err)
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
