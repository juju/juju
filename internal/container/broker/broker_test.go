// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type brokerSuite struct {
	testhelpers.IsolationSuite
}

func TestBrokerSuite(t *testing.T) {
	tc.Run(t, &brokerSuite{})
}

func (s *brokerSuite) TestAssociateDNSConfigSetsDomainsAndServers(c *tc.C) {
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
		Nameservers: []string{
			"192.168.20.2",
			"192.168.10.2",
			"8.8.8.8",
			"1.1.1.1",
		},
		SearchDomains: []string{"example.com"},
	}

	results := associateDNSConfig(c.Context(), nics, dnsCfg)
	c.Assert(results, tc.HasLen, 2)

	c.Assert(results[0].DNSSearchDomains, tc.DeepEquals, []string{"example.com"})
	c.Assert(results[0].DNSServers, tc.DeepEquals, []string{"192.168.10.2", "8.8.8.8", "1.1.1.1"})

	c.Assert(results[1].DNSSearchDomains, tc.DeepEquals, []string{"example.com"})
	c.Assert(results[1].DNSServers, tc.DeepEquals, []string{"192.168.20.2", "8.8.8.8", "1.1.1.1"})
}

func (s *brokerSuite) TestAssociateDNSConfigFallbackDedupes(c *tc.C) {
	nics := network.InterfaceInfos{
		{
			InterfaceName: "eth0",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("10.0.0.5", network.WithCIDR("10.0.0.0/24")).AsProviderAddress(),
			},
		},
	}

	dnsCfg := &network.DNSConfig{
		Nameservers: []string{
			// This matches the subnet and will be added during main association.
			"10.0.0.2",

			// Duplicate of the above — if dedupe fails, it will appear twice.
			"10.0.0.2",

			// Fallback DNS
			"8.8.8.8",

			// Duplicate fallback DNS
			"8.8.8.8",
		},
	}

	results := associateDNSConfig(c.Context(), nics, dnsCfg)
	c.Assert(results, tc.HasLen, 1)

	values := results[0].DNSServers

	// If dedupe is broken, we would get:
	// ["10.0.0.2", "10.0.0.2", "8.8.8.8"]
	// We expect only one instance of 10.0.0.2
	c.Assert(values, tc.DeepEquals, []string{
		"10.0.0.2",
		"8.8.8.8",
	})
}
