// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"testing"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"

	"github.com/juju/juju/core/network"
)

type brokerSuite struct {
	jujutesting.IsolationSuite
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
