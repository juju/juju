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

// TestProviderNetAddressWithCIDRUsesIPMaskFormat verifies that when a provider
// address carries a CIDR, AddressValue is stored as "IP/mask" (e.g.
// "10.0.0.5/24") rather than the bare IP. This format is required by
// ip_address.address_value and by the subnet-matching logic in the network
// state.
func (s *providerNetInterfaceSuite) TestProviderNetAddressWithCIDRUsesIPMaskFormat(c *tc.C) {
	nics := ProviderNetInterfaces(corenetwork.InterfaceInfos{{
		InterfaceName: "eth0",
		InterfaceType: corenetwork.EthernetDevice,
		Addresses: corenetwork.ProviderAddresses{{
			MachineAddress: corenetwork.MachineAddress{
				Value:      "10.0.0.5",
				CIDR:       "10.0.0.0/24",
				Type:       corenetwork.IPv4Address,
				Scope:      corenetwork.ScopeCloudLocal,
				ConfigType: corenetwork.ConfigDHCP,
			},
		}},
	}})

	c.Assert(nics, tc.HasLen, 1)
	c.Assert(nics[0].Addrs, tc.HasLen, 1)
	c.Check(nics[0].Addrs[0].AddressValue, tc.Equals, "10.0.0.5/24")
}

// TestProviderNetAddressWithoutCIDRUsesBareIP verifies that when a provider
// address has no CIDR (e.g. a floating/shadow IP), AddressValue falls back to
// the bare IP value.
func (s *providerNetInterfaceSuite) TestProviderNetAddressWithoutCIDRUsesBareIP(c *tc.C) {
	nics := ProviderNetInterfaces(corenetwork.InterfaceInfos{{
		InterfaceName: "eth0",
		InterfaceType: corenetwork.EthernetDevice,
		ShadowAddresses: corenetwork.ProviderAddresses{{
			MachineAddress: corenetwork.MachineAddress{
				Value: "54.1.2.3",
				Type:  corenetwork.IPv4Address,
				Scope: corenetwork.ScopePublic,
			},
		}},
	}})

	c.Assert(nics, tc.HasLen, 1)
	c.Assert(nics[0].Addrs, tc.HasLen, 1)
	c.Check(nics[0].Addrs[0].AddressValue, tc.Equals, "54.1.2.3")
	c.Check(nics[0].Addrs[0].IsShadow, tc.IsTrue)
}

// TestProviderNetAddressIPv6WithCIDR verifies that IPv6 addresses with a CIDR
// are also stored in IP/mask format.
func (s *providerNetInterfaceSuite) TestProviderNetAddressIPv6WithCIDR(c *tc.C) {
	nics := ProviderNetInterfaces(corenetwork.InterfaceInfos{{
		InterfaceName: "eth0",
		InterfaceType: corenetwork.EthernetDevice,
		Addresses: corenetwork.ProviderAddresses{{
			MachineAddress: corenetwork.MachineAddress{
				Value:      "2001:db8::1",
				CIDR:       "2001:db8::/32",
				Type:       corenetwork.IPv6Address,
				Scope:      corenetwork.ScopeCloudLocal,
				ConfigType: corenetwork.ConfigDHCP,
			},
		}},
	}})

	c.Assert(nics, tc.HasLen, 1)
	c.Assert(nics[0].Addrs, tc.HasLen, 1)
	c.Check(nics[0].Addrs[0].AddressValue, tc.Equals, "2001:db8::1/32")
}
