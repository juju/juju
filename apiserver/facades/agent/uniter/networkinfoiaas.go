// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// NetworkInfoIAAS is used to provide network info for IAAS units.
type NetworkInfoIAAS struct {
	*NetworkInfoBase
}

// TODO (manadart 2020-08-20): The logic below was moved over from the state
// package when machine.GetNetworkInfoForSpaces was removed from state and
// implemented here. It is an unnecessary convolution and should be factored
// out in favour of a simpler return from machineNetworkInfos.

// MachineNetworkInfoResult contains an error or a list of NetworkInfo
// structures for a specific space.
type machineNetworkInfoResult struct {
	NetworkInfos []network.NetworkInfo
	Error        error
}

// Add address to a device in list or create a new device with this address.
func addAddressToResult(networkInfos []network.NetworkInfo, address *state.Address) ([]network.NetworkInfo, error) {
	deviceAddr := network.InterfaceAddress{
		Address: address.Value(),
		CIDR:    address.SubnetCIDR(),
	}
	for i := range networkInfos {
		networkInfo := &networkInfos[i]
		if networkInfo.InterfaceName == address.DeviceName() {
			networkInfo.Addresses = append(networkInfo.Addresses, deviceAddr)
			return networkInfos, nil
		}
	}

	var MAC string
	device, err := address.Device()
	if err == nil {
		MAC = device.MACAddress()
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	networkInfo := network.NetworkInfo{
		InterfaceName: address.DeviceName(),
		MACAddress:    MAC,
		Addresses:     []network.InterfaceAddress{deviceAddr},
	}
	return append(networkInfos, networkInfo), nil
}

func machineNetworkInfoResultToNetworkInfoResult(inResult machineNetworkInfoResult) params.NetworkInfoResult {
	if inResult.Error != nil {
		return params.NetworkInfoResult{Error: common.ServerError(inResult.Error)}
	}
	infos := make([]params.NetworkInfo, len(inResult.NetworkInfos))
	for i, info := range inResult.NetworkInfos {
		infos[i] = networkToParamsNetworkInfo(info)
	}
	return params.NetworkInfoResult{
		Info: infos,
	}
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
