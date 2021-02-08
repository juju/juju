// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package packet

import (
	"fmt"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/lxc/lxd/shared/logger"
	"github.com/packethost/packngo"
)

type packetDevice struct {
	e *environ

	*packngo.Device
}

var _ instances.Instance = (*packetDevice)(nil)

func (device *packetDevice) String() string {
	return string(device.ID)
}

func (device *packetDevice) Id() instance.Id {
	return instance.Id(device.ID)
}

func (device *packetDevice) Status(ctx context.ProviderCallContext) instance.Status {
	var jujuStatus status.Status

	switch device.State {
	case "provisioning":
		jujuStatus = status.Pending
	case "active":
		jujuStatus = status.Running
	case "shutting-down", "terminated", "stopping", "stopped":
		jujuStatus = status.Empty
	default:
		jujuStatus = status.Empty
	}

	return instance.Status{
		Status:  jujuStatus,
		Message: device.State,
	}

}

// Addresses implements network.Addresses() returning generic address
// details for the instance, and requerying the ec2 api if required.
func (device *packetDevice) Addresses(ctx context.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	var addresses []corenetwork.ProviderAddress

	for _, netw := range device.Network {
		address := corenetwork.ProviderAddress{}
		address.Value = netw.Address
		address.CIDR = fmt.Sprintf("%s/%d", netw.Network, netw.CIDR)

		if netw.Public {
			address.Scope = corenetwork.ScopePublic
		} else {
			address.Scope = corenetwork.ScopeCloudLocal
		}

		if netw.AddressFamily == 4 {
			address.Type = network.IPv4Address
		} else {
			address.Type = network.IPv6Address
			logger.Infof("skipping IPv6 Address %s", netw.Address)

			continue
		}

		addresses = append(addresses, address)
	}

	return addresses, nil
}
