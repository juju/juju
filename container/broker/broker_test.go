// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type brokerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&brokerSuite{})

func (s *brokerSuite) TestAssociateDNSConfigSetsDomainsAndServers(c *gc.C) {
	nics := network.InterfaceInfos{
		{
			InterfaceName: "eth0",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("192.168.10.6", network.WithCIDR("192.168.10.0/24")).AsProviderAddress(),
			},
		},
		{
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("192.168.20.6", network.WithCIDR("192.168.20.0/24")).AsProviderAddress(),
			},
		},
	}
	dnsCfg := &network.DNSConfig{
		Nameservers: []network.ProviderAddress{
			network.NewMachineAddress("192.168.20.2").AsProviderAddress(),
			network.NewMachineAddress("192.168.10.2").AsProviderAddress(),
			network.NewMachineAddress("8.8.8.8").AsProviderAddress(),
			network.NewMachineAddress("1.1.1.1").AsProviderAddress(),
		},
		SearchDomains: []string{"example.com"},
	}

	results := associateDNSConfig(nics, dnsCfg)
	c.Assert(results, gc.HasLen, 2)

	c.Assert(results[0].DNSSearchDomains, gc.DeepEquals, []string{"example.com"})
	c.Assert(results[0].DNSServers.Values(), gc.DeepEquals, []string{"192.168.10.2", "8.8.8.8", "1.1.1.1"})

	c.Assert(results[1].DNSSearchDomains, gc.DeepEquals, []string{"example.com"})
	c.Assert(results[1].DNSServers.Values(), gc.DeepEquals, []string{"192.168.20.2", "8.8.8.8", "1.1.1.1"})
}
