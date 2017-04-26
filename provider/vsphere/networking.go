// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/juju/juju/network"
)

func getNetworkInterfaces(vm *mo.VirtualMachine, ecfg *environConfig) ([]network.InterfaceInfo, error) {
	if vm.Guest == nil {
		return nil, errors.Errorf("vm guest is not initialized")
	}
	res := make([]network.InterfaceInfo, 0)
	for _, net := range vm.Guest.Net {
		ipScope := network.ScopeCloudLocal
		if net.Network == ecfg.externalNetwork() {
			ipScope = network.ScopePublic
		}
		res = append(res, network.InterfaceInfo{
			DeviceIndex:      int(net.DeviceConfigId),
			MACAddress:       net.MacAddress,
			Disabled:         !net.Connected,
			ProviderId:       network.Id(fmt.Sprintf("net-device%d", net.DeviceConfigId)),
			ProviderSubnetId: network.Id(net.Network),
			InterfaceName:    fmt.Sprintf("unsupported%d", net.DeviceConfigId),
			ConfigType:       network.ConfigDHCP,
			Address:          network.NewScopedAddress(net.IpAddress[0], ipScope),
		})
	}
	return res, nil
}

func subnets(vm *mo.VirtualMachine, ids []network.Id) ([]network.SubnetInfo, error) {
	if len(ids) == 0 {
		return nil, errors.Errorf("subnetIds must not be empty")
	}
	res := make([]network.SubnetInfo, 0)
	for _, vmNet := range vm.Guest.Net {
		existId := false
		for _, id := range ids {
			if string(id) == vmNet.Network {
				existId = true
				break
			}
		}
		if !existId {
			continue
		}
		res = append(res, network.SubnetInfo{
			ProviderId: network.Id(vmNet.Network),
		})
	}
	return res, nil
}
