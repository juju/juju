// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestParamsNetworkConfigToDomain(c *tc.C) {
	args := []params.NetworkConfig{
		{
			InterfaceName:       "eth1",
			MTU:                 9000,
			MACAddress:          "00:11:22:33:44:55",
			InterfaceType:       "ethernet",
			VirtualPortType:     "",
			ParentInterfaceName: "br0",
			GatewayAddress:      "192.168.1.1",
			VLANTag:             42,
			DNSSearchDomains:    []string{"example.com"},
			DNSServers:          []string{"8.8.8.8"},
			Addresses: []params.Address{
				{
					Value:      "192.168.1.100",
					Type:       "ipv4",
					ConfigType: "dhcp",
					Scope:      "local-cloud",
					CIDR:       "192.168.1.0/24",
				},
			},
		},
	}

	result, err := ParamsNetworkConfigToDomain(args, network.OriginMachine)
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.HasLen, 1)

	nic := result[0]
	c.Check(nic.Name, tc.Equals, "eth1")
	c.Check(*nic.MTU, tc.Equals, int64(9000))
	c.Check(*nic.MACAddress, tc.Equals, "00:11:22:33:44:55")
	c.Check(nic.ProviderID, tc.IsNil)
	c.Check(nic.Type, tc.Equals, network.EthernetDevice)
	c.Check(nic.VirtualPortType, tc.Equals, network.NonVirtualPort)
	c.Check(nic.IsAutoStart, tc.Equals, true)
	c.Check(nic.IsEnabled, tc.Equals, true)
	c.Check(nic.ParentDeviceName, tc.Equals, "br0")
	c.Check(*nic.GatewayAddress, tc.Equals, "192.168.1.1")
	c.Check(nic.VLANTag, tc.Equals, uint64(42))
	c.Check(nic.DNSSearchDomains, tc.DeepEquals, []string{"example.com"})
	c.Check(nic.DNSAddresses, tc.DeepEquals, []string{"8.8.8.8"})

	c.Assert(nic.Addrs, tc.HasLen, 1)
	addr := nic.Addrs[0]
	c.Check(addr.InterfaceName, tc.Equals, "eth1")
	c.Check(addr.ProviderID, tc.IsNil)
	c.Check(addr.AddressValue, tc.Equals, "192.168.1.100/24")
	c.Check(addr.ProviderSubnetID, tc.IsNil)
	c.Check(addr.AddressType, tc.Equals, network.IPv4Address)
	c.Check(addr.ConfigType, tc.Equals, network.ConfigDHCP)
	c.Check(addr.Origin, tc.Equals, network.OriginMachine)
	c.Check(addr.Scope, tc.Equals, network.ScopeCloudLocal)
	c.Check(addr.IsSecondary, tc.Equals, false)
	c.Check(addr.IsShadow, tc.Equals, false)
}
