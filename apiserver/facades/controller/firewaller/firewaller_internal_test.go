// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

var _ = gc.Suite(&UnitToCIDRMappingSuite{})

type UnitToCIDRMappingSuite struct {
	testing.IsolationSuite
}

func (s *UnitToCIDRMappingSuite) TestBindingMapping(c *gc.C) {
	unitName := "u/0"
	portRangesByEndpoint := map[string][]network.PortRange{
		"foo": []network.PortRange{
			network.MustParsePortRange("123/tcp"),
			network.MustParsePortRange("456/tcp"),
		},
		"bar": []network.PortRange{
			// The descending ordering here is intentional so that
			// we can verify that the output list entries do get
			// sorted.
			network.MustParsePortRange("777/tcp"),
			network.MustParsePortRange("123/tcp"),
		},
	}
	endpointBindings := map[string]string{
		"": network.AlphaSpaceId,
		// Both endpoints bound to same space
		"foo": "42",
		"bar": "42",
	}

	subnetCIDRsBySpaceID := map[string][]string{
		network.AlphaSpaceId: []string{
			"10.0.0.0/24",
			"10.0.1.0/24",
		},
		"42": []string{
			"192.168.0.0/24",
			"192.168.1.0/24",
		},
	}

	got := mapUnitPortsToSubnetCIDRs(unitName, portRangesByEndpoint, endpointBindings, subnetCIDRsBySpaceID)
	exp := map[string][]network.PortRange{
		"192.168.0.0/24": []network.PortRange{
			network.MustParsePortRange("123/tcp"),
			network.MustParsePortRange("456/tcp"),
			network.MustParsePortRange("777/tcp"),
		},
		"192.168.1.0/24": []network.PortRange{
			network.MustParsePortRange("123/tcp"),
			network.MustParsePortRange("456/tcp"),
			network.MustParsePortRange("777/tcp"),
		},
	}

	c.Assert(got, gc.DeepEquals, exp)
}

func (s *UnitToCIDRMappingSuite) TestWildcardExpansion(c *gc.C) {
	unitName := "u/0"
	portRangesByEndpoint := map[string][]network.PortRange{
		"": []network.PortRange{
			// These ranges should be added to the CIDRs of each
			// bound endpoint (so, both alpha and "42").
			network.MustParsePortRange("123/tcp"),
			network.MustParsePortRange("456/tcp"),
		},
		"bar": []network.PortRange{
			network.MustParsePortRange("999/tcp"),
		},
	}
	endpointBindings := map[string]string{
		"":    network.AlphaSpaceId,
		"foo": network.AlphaSpaceId,
		"bar": "42",
	}

	subnetCIDRsBySpaceID := map[string][]string{
		network.AlphaSpaceId: []string{
			"10.0.0.0/24",
			"10.0.1.0/24",
		},
		"42": []string{
			"192.168.0.0/24",
			"192.168.1.0/24",
		},
	}

	got := mapUnitPortsToSubnetCIDRs(unitName, portRangesByEndpoint, endpointBindings, subnetCIDRsBySpaceID)
	exp := map[string][]network.PortRange{
		"10.0.0.0/24": []network.PortRange{
			network.MustParsePortRange("123/tcp"),
			network.MustParsePortRange("456/tcp"),
		},
		"10.0.1.0/24": []network.PortRange{
			network.MustParsePortRange("123/tcp"),
			network.MustParsePortRange("456/tcp"),
		},
		"192.168.0.0/24": []network.PortRange{
			network.MustParsePortRange("123/tcp"),
			network.MustParsePortRange("456/tcp"),
			network.MustParsePortRange("999/tcp"),
		},
		"192.168.1.0/24": []network.PortRange{
			network.MustParsePortRange("123/tcp"),
			network.MustParsePortRange("456/tcp"),
			network.MustParsePortRange("999/tcp"),
		},
	}

	c.Assert(got, gc.DeepEquals, exp)
}
