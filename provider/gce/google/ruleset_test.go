// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type RuleSetSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&RuleSetSuite{})

func makeRuleSet() ruleSet {
	rule1 := network.MustNewIngressRule("tcp", 8000, 8099)
	rule2 := network.MustNewIngressRule("tcp", 80, 80)
	rule3 := network.MustNewIngressRule("tcp", 79, 81)
	rule4 := network.MustNewIngressRule("udp", 5123, 8099, "192.168.1.0/24")
	rule5 := network.MustNewIngressRule("icmp", -1, -1)
	return newRuleSetFromRules(rule1, rule2, rule3, rule4, rule5)
}

func (s *RuleSetSuite) TestNewRuleSetFromRules(c *gc.C) {
	rs := makeRuleSet()
	c.Assert(rs, jc.DeepEquals, ruleSet{
		"b42e18366a": &firewall{
			SourceCIDRs: []string{"0.0.0.0/0"},
			AllowedPorts: protocolPorts{
				"tcp":  []corenetwork.PortRange{{8000, 8099, "tcp"}, {80, 80, "tcp"}, {79, 81, "tcp"}},
				"icmp": []corenetwork.PortRange{{-1, -1, "icmp"}},
			},
		},
		"d01a825c13": &firewall{
			SourceCIDRs: []string{"192.168.1.0/24"},
			AllowedPorts: protocolPorts{
				"udp": []corenetwork.PortRange{{5123, 8099, "udp"}},
			},
		},
	})
}

func newFirewall(name, target string, sourceRanges []string, ports map[string][]string) *compute.Firewall {
	allowed := make([]*compute.FirewallAllowed, len(ports))
	i := 0
	for protocol, ranges := range ports {
		allowed[i] = &compute.FirewallAllowed{
			IPProtocol: protocol,
			Ports:      ranges,
		}
		i++
	}
	return &compute.Firewall{
		Name:         name,
		TargetTags:   []string{target},
		SourceRanges: sourceRanges,
		Allowed:      allowed,
	}
}

func (s *RuleSetSuite) TestNewRuleSetFromFirewalls(c *gc.C) {
	ports := map[string][]string{
		"tcp": {"80", "443", "17070-17073"},
		"udp": {"123"},
	}
	fw1 := newFirewall("weeps", "target", []string{"1.2.3.0/24", "2.3.4.0/24"}, ports)
	fw2 := newFirewall("blackbird", "somewhere", nil, ports)
	ruleset, err := newRuleSetFromFirewalls(fw1, fw2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ruleset, gc.DeepEquals, ruleSet{
		"b42e18366a": &firewall{
			Name:        "blackbird",
			Target:      "somewhere",
			SourceCIDRs: []string{"0.0.0.0/0"},
			AllowedPorts: protocolPorts{
				"tcp": []corenetwork.PortRange{{80, 80, "tcp"}, {443, 443, "tcp"}, {17070, 17073, "tcp"}},
				"udp": []corenetwork.PortRange{{123, 123, "udp"}},
			},
		},
		"0e3c16a771": &firewall{
			Name:        "weeps",
			Target:      "target",
			SourceCIDRs: []string{"1.2.3.0/24", "2.3.4.0/24"},
			AllowedPorts: protocolPorts{
				"tcp": []corenetwork.PortRange{{80, 80, "tcp"}, {443, 443, "tcp"}, {17070, 17073, "tcp"}},
				"udp": []corenetwork.PortRange{{123, 123, "udp"}},
			},
		},
	})
}

func (s *RuleSetSuite) TestProtocolPortsUnion(c *gc.C) {
	p1 := protocolPorts{"tcp": []corenetwork.PortRange{{8000, 8099, "tcp"}, {80, 80, "tcp"}, {79, 81, "tcp"}}}
	p2 := protocolPorts{
		"tcp": []corenetwork.PortRange{{80, 80, "tcp"}, {80, 100, "tcp"}, {443, 443, "tcp"}},
		"udp": []corenetwork.PortRange{{67, 67, "udp"}},
	}
	result := p1.union(p2)
	c.Assert(result, jc.DeepEquals, protocolPorts{
		"tcp": []corenetwork.PortRange{{8000, 8099, "tcp"}, {80, 80, "tcp"}, {79, 81, "tcp"}, {80, 100, "tcp"}, {443, 443, "tcp"}},
		"udp": []corenetwork.PortRange{{67, 67, "udp"}},
	})
}

func (s *RuleSetSuite) TestProtocolPortsRemove(c *gc.C) {
	p1 := protocolPorts{"tcp": []corenetwork.PortRange{{8000, 8099, "tcp"}, {80, 80, "tcp"}, {79, 81, "tcp"}}}
	p2 := protocolPorts{
		"tcp": []corenetwork.PortRange{{80, 100, "tcp"}, {443, 443, "tcp"}, {80, 80, "tcp"}},
	}
	result := p1.remove(p2)
	c.Assert(result, jc.DeepEquals, protocolPorts{
		"tcp": []corenetwork.PortRange{{8000, 8099, "tcp"}, {79, 81, "tcp"}},
	})
}

func (s *RuleSetSuite) TestRuleSetToIngressRules(c *gc.C) {
	ports := map[string][]string{
		"tcp": {"80", "443", "17070-17073"},
		"udp": {"123"},
	}
	fw1 := newFirewall("weeps", "target", []string{"1.2.3.0/24", "2.3.4.0/24"}, ports)
	fw2 := newFirewall("blackbird", "somewhere", nil, ports)
	ruleset, err := newRuleSetFromFirewalls(fw1, fw2)
	c.Assert(err, jc.ErrorIsNil)
	rules, err := ruleset.toIngressRules()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.DeepEquals, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 80, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 80, 80, "1.2.3.0/24", "2.3.4.0/24"),
		network.MustNewIngressRule("tcp", 443, 443, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 443, 443, "1.2.3.0/24", "2.3.4.0/24"),
		network.MustNewIngressRule("tcp", 17070, 17073, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 17070, 17073, "1.2.3.0/24", "2.3.4.0/24"),
		network.MustNewIngressRule("udp", 123, 123, "0.0.0.0/0"),
		network.MustNewIngressRule("udp", 123, 123, "1.2.3.0/24", "2.3.4.0/24"),
	})
}

func (s *RuleSetSuite) TestMatchPorts(c *gc.C) {
	ruleset := makeRuleSet()
	fw, ok := ruleset.matchProtocolPorts(protocolPorts{
		"udp": {{5123, 8099, "udp"}},
	})
	c.Assert(ok, jc.IsTrue)
	c.Assert(fw, gc.DeepEquals, &firewall{
		AllowedPorts: protocolPorts{
			"udp": {{5123, 8099, "udp"}},
		},
		SourceCIDRs: []string{"192.168.1.0/24"},
	})
	// No partial matches.
	fw, ok = ruleset.matchProtocolPorts(protocolPorts{
		"tcp": {{80, 80, "tcp"}},
	})
	c.Assert(ok, jc.IsFalse)
	c.Assert(fw, gc.IsNil)
}

func (s *RuleSetSuite) TestMatchSourceCIDRs(c *gc.C) {
	ruleset := makeRuleSet()
	c.Logf("%#v", ruleset)
	c.Logf("%s", sourcecidrs([]string{"0.0.0.0/0"}).key())
	fw, ok := ruleset.matchSourceCIDRs([]string{"0.0.0.0/0"})
	c.Assert(ok, jc.IsTrue)
	c.Assert(fw, gc.DeepEquals, &firewall{
		SourceCIDRs: []string{"0.0.0.0/0"},
		AllowedPorts: protocolPorts{
			"tcp":  []corenetwork.PortRange{{8000, 8099, "tcp"}, {80, 80, "tcp"}, {79, 81, "tcp"}},
			"icmp": []corenetwork.PortRange{{-1, -1, "icmp"}},
		},
	})
	fw, ok = ruleset.matchSourceCIDRs([]string{"1.2.3.0/24"})
	c.Assert(ok, jc.IsFalse)
	c.Assert(fw, gc.IsNil)
}

func (s *RuleSetSuite) TestAllNames(c *gc.C) {
	ports := map[string][]string{"tcp": {"80"}}
	fw1 := newFirewall("weeps", "target", []string{"1.2.3.0/24", "2.3.4.0/24"}, ports)
	fw2 := newFirewall("blackbird", "somewhere", nil, ports)
	ruleset, err := newRuleSetFromFirewalls(fw1, fw2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ruleset.allNames(), gc.DeepEquals, set.NewStrings("weeps", "blackbird"))
}

func (s *RuleSetSuite) TestDifferentOrdersForCIDRs(c *gc.C) {
	rule1 := network.MustNewIngressRule("tcp", 8000, 8099, "1.2.3.0/24", "4.3.2.0/24")
	rule2 := network.MustNewIngressRule("tcp", 80, 80, "4.3.2.0/24", "1.2.3.0/24")
	ruleset := NewRuleSetFromRules(rule1, rule2)
	// They should be combined into one firewall rule.
	c.Assert(ruleset, gc.HasLen, 1)
	for _, fw := range ruleset {
		c.Assert(fw, gc.DeepEquals, &firewall{
			SourceCIDRs: []string{"1.2.3.0/24", "4.3.2.0/24"},
			AllowedPorts: protocolPorts{
				"tcp": []corenetwork.PortRange{{8000, 8099, "tcp"}, {80, 80, "tcp"}},
			},
		})
	}
}
