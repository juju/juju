// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
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
