// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"testing"

	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
)

type providerNetInterfaceSuite struct{}

func TestProviderNetInterfaceSuite(t *testing.T) {
	tc.Run(t, &providerNetInterfaceSuite{})
}

func (s *providerNetInterfaceSuite) TestProviderNetInterfacesUsesInterfaceType(c *tc.C) {
	nics := ProviderNetInterfaces(corenetwork.InterfaceInfos{{
		InterfaceName: "eth0",
		InterfaceType: corenetwork.EthernetDevice,
		ConfigType:    corenetwork.ConfigDHCP,
		Addresses: corenetwork.ProviderAddresses{
			{
				MachineAddress: corenetwork.MachineAddress{
					CIDR: "10.0.0.0/24",
					Type: corenetwork.HostName,
				},
			},
			{
				MachineAddress: corenetwork.MachineAddress{
					Value:      "10.0.0.2",
					CIDR:       "10.0.0.0/24",
					Type:       corenetwork.IPv4Address,
					Scope:      corenetwork.ScopeCloudLocal,
					ConfigType: corenetwork.ConfigDHCP,
				},
			},
		},
	}})

	c.Assert(nics, tc.HasLen, 1)
	c.Check(nics[0].Type, tc.Equals, corenetwork.EthernetDevice)
	c.Assert(nics[0].Addrs, tc.HasLen, 1)
	c.Check(nics[0].Addrs[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}
