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

func (s *brokerSuite) TestAssociateDNSConfigFallbackDedupes(c *gc.C) {
	nics := network.InterfaceInfos{
		{
			InterfaceName: "eth0",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("10.0.0.5", network.WithCIDR("10.0.0.0/24")).AsProviderAddress(),
			},
		},
	}

	dnsCfg := &network.DNSConfig{
		Nameservers: []network.ProviderAddress{
			// This matches the subnet and will be added during main association.
			network.NewMachineAddress("10.0.0.2").AsProviderAddress(),

			// Duplicate of the above — if dedupe fails, it will appear twice.
			network.NewMachineAddress("10.0.0.2").AsProviderAddress(),

			// Fallback DNS
			network.NewMachineAddress("8.8.8.8").AsProviderAddress(),

			// Duplicate fallback DNS
			network.NewMachineAddress("8.8.8.8").AsProviderAddress(),
		},
	}

	results := associateDNSConfig(nics, dnsCfg)
	c.Assert(results, gc.HasLen, 1)

	values := results[0].DNSServers.Values()

	// If dedupe is broken, we would get:
	// ["10.0.0.2", "10.0.0.2", "8.8.8.8"]
	// We expect only one instance of 10.0.0.2
	c.Assert(values, gc.DeepEquals, []string{
		"10.0.0.2",
		"8.8.8.8",
	})
}
