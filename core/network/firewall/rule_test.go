// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/network"
)

var _ = gc.Suite(&IngressRuleSuite{})

type IngressRuleSuite struct {
	testing.IsolationSuite
}

func (IngressRuleSuite) TestRuleFormatting(c *gc.C) {
	pr := network.MustParsePortRange("8080-9090/tcp")
	r1 := NewIngressRule(pr)
	c.Assert(r1.PortRange, gc.Equals, pr)
	c.Assert(r1.SourceCIDRs, gc.HasLen, 0)
	c.Assert(r1.String(), gc.Equals, "8080-9090/tcp")

	r2 := NewIngressRule(pr, "10.0.0.0/24", "0.0.0.0/0", "0.0.0.0/0")
	c.Assert(r2.PortRange, gc.Equals, pr)
	c.Assert(r2.SourceCIDRs, gc.HasLen, 2, gc.Commentf("expected ingress rule not to contain duplicate CIDRs"))
	c.Assert(r2.String(), gc.Equals, "8080-9090/tcp from 0.0.0.0/0,10.0.0.0/24")
}

func (IngressRuleSuite) TestRuleValidation(c *gc.C) {
	bogus := network.PortRange{
		Protocol: "gopher",
		FromPort: 1,
		ToPort:   1,
	}
	r1 := NewIngressRule(bogus)
	c.Assert(r1.Validate(), gc.ErrorMatches, `.*invalid protocol "gopher", expected "tcp", "udp", or "icmp"`)

	pr := network.MustParsePortRange("8080-9090/tcp")
	r2 := NewIngressRule(pr, "bogus")
	c.Assert(r2.Validate(), gc.ErrorMatches, ".*invalid CIDR address: bogus")

	r3 := NewIngressRule(pr, "100.0.0.0/8")
	c.Assert(r3.Validate(), jc.ErrorIsNil)
}

func (IngressRuleSuite) TestRuleEquality(c *gc.C) {
	specs := []struct {
		descr        string
		ruleA, ruleB IngressRule
		exp          bool
	}{
		{
			descr: "same port and CIDRs",
			ruleA: NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24", "192.168.0.0/24"),
			ruleB: NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24", "10.0.0.0/24"),
			exp:   true,
		},
		{
			descr: "same port different CIDRs",
			ruleA: NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24", "192.168.42.0/24"),
			ruleB: NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24", "10.0.0.0/24"),
			exp:   false,
		},
		{
			descr: "different port same CIDRs",
			ruleA: NewIngressRule(network.MustParsePortRange("90/tcp"), "10.0.0.0/24", "192.168.0.0/24"),
			ruleB: NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24", "10.0.0.0/24"),
			exp:   false,
		},
	}

	for specIndex, spec := range specs {
		c.Logf("%d) %s", specIndex, spec.descr)
		got := spec.ruleA.EqualTo(spec.ruleB)
		c.Assert(got, gc.Equals, spec.exp)

		got = spec.ruleB.EqualTo(spec.ruleA)
		c.Assert(got, gc.Equals, spec.exp)
	}
}

func (IngressRuleSuite) TestRuleSorting(c *gc.C) {
	rules := IngressRules{
		NewIngressRule(network.MustParsePortRange("10-100/udp"), "0.0.0.0/0", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "0.0.0.0/0", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/udp"), "0.0.0.0/0", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80/tcp"), "0.0.0.0/0", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80/tcp"), "0.0.0.0/0"),
	}
	rules.Sort()

	exp := IngressRules{
		NewIngressRule(network.MustParsePortRange("80/tcp"), "0.0.0.0/0"),
		NewIngressRule(network.MustParsePortRange("80/tcp"), "0.0.0.0/0", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "0.0.0.0/0", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("10-100/udp"), "0.0.0.0/0", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/udp"), "0.0.0.0/0", "192.168.1.0/24"),
	}

	c.Assert(rules, gc.DeepEquals, exp)
}

func (IngressRuleSuite) TestRulesEquality(c *gc.C) {
	setA := IngressRules{
		NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24", "192.168.0.0/24"),
		NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24", "10.0.0.0/24"),
	}
	setB := IngressRules{
		NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24", "10.0.0.0/24"),
		NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24", "192.168.0.0/24"),
	}
	setC := IngressRules{
		NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24", "10.0.0.0/24"),
		NewIngressRule(network.MustParsePortRange("90/tcp"), "10.0.0.0/24", "192.168.0.0/24"),
	}

	c.Assert(setA.EqualTo(setB), jc.IsTrue)
	c.Assert(setA.EqualTo(setC), jc.IsFalse)
	c.Assert(setB.EqualTo(setC), jc.IsFalse)
}

func (IngressRuleSuite) TestUniqueRules(c *gc.C) {
	in := IngressRules{
		NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24", "192.168.0.0/24"),
		NewIngressRule(network.MustParsePortRange("123/tcp"), "192.168.0.0/24", "10.0.0.0/24"),
		NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24", "192.168.0.0/24"),
	}

	exp := IngressRules{
		NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24", "192.168.0.0/24"),
		NewIngressRule(network.MustParsePortRange("123/tcp"), "192.168.0.0/24", "10.0.0.0/24"),
	}

	c.Assert(in.UniqueRules(), gc.DeepEquals, exp)
}

func (IngressRuleSuite) TestDiffOpenAll(c *gc.C) {
	wanted := IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "0.0.0.0/0"),
		NewIngressRule(network.MustParsePortRange("443/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/udp"), "0.0.0.0/0"),
	}
	wanted.Sort()

	toOpen, toClose := IngressRules{}.Diff(wanted)
	c.Assert(toClose, gc.HasLen, 0)
	c.Assert(toOpen, jc.DeepEquals, wanted)
}

func (IngressRuleSuite) TestDiffCloseAll(c *gc.C) {
	current := IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "0.0.0.0/0"),
		NewIngressRule(network.MustParsePortRange("443/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/udp"), "0.0.0.0/0"),
	}
	current.Sort()

	toOpen, toClose := current.Diff(nil)
	c.Assert(toOpen, gc.HasLen, 0)
	c.Assert(toClose, jc.DeepEquals, current)
}

func (IngressRuleSuite) TestDiffNoPortRangeOverlap(c *gc.C) {
	current := IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "0.0.0.0/0"),
		NewIngressRule(network.MustParsePortRange("443/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/udp"), "0.0.0.0/0"),
	}
	extra := IngressRules{
		NewIngressRule(network.MustParsePortRange("100-110/tcp"), "0.0.0.0/0"),
		NewIngressRule(network.MustParsePortRange("8080/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("67/udp"), "0.0.0.0/0"),
	}

	wanted := append(current, extra...)
	toOpen, toClose := current.Diff(wanted)
	c.Assert(toClose, gc.HasLen, 0)

	extra.Sort()
	c.Assert(toOpen, jc.DeepEquals, extra)
}

func (IngressRuleSuite) TestPortRangeOverlapToOpen(c *gc.C) {
	current := IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "10.0.0.0/24"),
		NewIngressRule(network.MustParsePortRange("443/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/udp"), "0.0.0.0/0"),
	}
	extra := IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "192.168.1.0/24", "10.0.0.0/24"),
		NewIngressRule(network.MustParsePortRange("8080/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("67/udp"), "0.0.0.0/0"),
	}
	wanted := append(current, extra...)
	toOpen, toClose := current.Diff(wanted)
	c.Assert(toClose, gc.HasLen, 0)

	c.Assert(toOpen, jc.DeepEquals, IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("8080/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("67/udp"), "0.0.0.0/0"),
	})
}

func (IngressRuleSuite) TestPortRangeOverlapToClose(c *gc.C) {
	current := IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("443/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/udp"), "0.0.0.0/0"),
	}
	wanted := IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "10.0.0.0/24"),
		NewIngressRule(network.MustParsePortRange("443/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/udp"), "0.0.0.0/0"),
	}

	toOpen, toClose := current.Diff(wanted)
	c.Assert(toOpen, gc.HasLen, 0)

	c.Assert(toClose, jc.DeepEquals, IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "192.168.1.0/24"),
	})
}

func (IngressRuleSuite) TestPortRangeOverlap(c *gc.C) {
	current := IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("443/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
	}
	wanted := IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "10.0.0.0/24"),
		NewIngressRule(network.MustParsePortRange("443/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		NewIngressRule(network.MustParsePortRange("80-90/udp"), "0.0.0.0/0"),
	}

	toOpen, toClose := current.Diff(wanted)
	c.Assert(toOpen, jc.DeepEquals, IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/udp"), "0.0.0.0/0"),
	})
	c.Assert(toClose, jc.DeepEquals, IngressRules{
		NewIngressRule(network.MustParsePortRange("80-90/tcp"), "192.168.1.0/24"),
	})
}

func (IngressRuleSuite) TestDiffRangesClosesPortsIfRulesAreDisjoint(c *gc.C) {
	current := IngressRules{
		NewIngressRule(network.MustParsePortRange("3306/tcp"), "35.187.158.35/32"),
	}
	wanted := IngressRules{
		NewIngressRule(network.MustParsePortRange("3306/tcp"), "35.187.152.241/32"),
	}

	toOpen, toClose := current.Diff(wanted)
	c.Assert(toOpen, gc.DeepEquals, wanted)
	c.Assert(toClose, gc.DeepEquals, current)
}

func (IngressRuleSuite) TestRemoveCIDRsMatchingAddressType(c *gc.C) {
	in := IngressRules{
		NewIngressRule(network.MustParsePortRange("80/tcp"), "35.187.158.35/32"),
		// We expect these rules to be de-dupped once the IPV6 CIDRs get removed
		NewIngressRule(network.MustParsePortRange("81/tcp"), "35.187.1.35/32", "::/0"),
		NewIngressRule(network.MustParsePortRange("81/tcp"), "35.187.1.35/32", "2002::1234:abcd:ffff:c0a8:101/64"),
	}

	out := in.RemoveCIDRsMatchingAddressType(network.IPv6Address)
	c.Assert(out, gc.DeepEquals, IngressRules{
		NewIngressRule(network.MustParsePortRange("80/tcp"), "35.187.158.35/32"),
		NewIngressRule(network.MustParsePortRange("81/tcp"), "35.187.1.35/32"),
	})
}
