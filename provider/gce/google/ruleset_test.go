// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type RuleSetSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&RuleSetSuite{})

func (s *RuleSetSuite) TestGetFirewallRules(c *gc.C) {
	rule1 := network.MustNewIngressRule("tcp", 8000, 8099)
	rule2 := network.MustNewIngressRule("tcp", 80, 80)
	rule3 := network.MustNewIngressRule("tcp", 79, 81)
	rule4 := network.MustNewIngressRule("udp", 5123, 8099, "192.168.1.0/24")

	rs := newRuleSet(rule1, rule2, rule3, rule4)
	fwRules := rs.getFirewallRules("firewall")
	c.Assert(fwRules, jc.DeepEquals, map[string]protocolPorts{
		"firewall": {
			"tcp": []network.PortRange{{8000, 8099, "tcp"}, {80, 80, "tcp"}, {79, 81, "tcp"}},
		},
		"firewall-d01a82": {
			"udp": []network.PortRange{{5123, 8099, "udp"}},
		},
	})
	cidrs := rs.getCIDRs("firewall")
	c.Assert(cidrs, jc.DeepEquals, []string{"0.0.0.0/0"})
	cidrs = rs.getCIDRs("firewall-d01a82")
	c.Assert(cidrs, jc.DeepEquals, []string{"192.168.1.0/24"})
}

func (s *RuleSetSuite) TestProtocolPortsUnion(c *gc.C) {
	p1 := protocolPorts{"tcp": []network.PortRange{{8000, 8099, "tcp"}, {80, 80, "tcp"}, {79, 81, "tcp"}}}
	p2 := protocolPorts{
		"tcp": []network.PortRange{{80, 80, "tcp"}, {80, 100, "tcp"}, {443, 443, "tcp"}},
		"udp": []network.PortRange{{67, 67, "udp"}},
	}
	result := p1.union(p2)
	c.Assert(result, jc.DeepEquals, protocolPorts{
		"tcp": []network.PortRange{{8000, 8099, "tcp"}, {80, 80, "tcp"}, {79, 81, "tcp"}, {80, 100, "tcp"}, {443, 443, "tcp"}},
		"udp": []network.PortRange{{67, 67, "udp"}},
	})
}

func (s *RuleSetSuite) TestProtocolPortsRemove(c *gc.C) {
	p1 := protocolPorts{"tcp": []network.PortRange{{8000, 8099, "tcp"}, {80, 80, "tcp"}, {79, 81, "tcp"}}}
	p2 := protocolPorts{
		"tcp": []network.PortRange{{80, 100, "tcp"}, {443, 443, "tcp"}, {80, 80, "tcp"}},
	}
	result := p1.remove(p2)
	c.Assert(result, jc.DeepEquals, protocolPorts{
		"tcp": []network.PortRange{{8000, 8099, "tcp"}, {79, 81, "tcp"}},
	})
}
