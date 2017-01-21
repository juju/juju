// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type FirewallSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&FirewallSuite{})

func (*FirewallSuite) TestStrings(c *gc.C) {
	rule, err := network.NewIngressRule("tcp", 80, 80)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rule.String(),
		gc.Equals,
		"80/tcp",
	)
	c.Assert(
		rule.GoString(),
		gc.Equals,
		"80/tcp",
	)

	rule, err = network.NewIngressRule("tcp", 80, 100)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rule.String(),
		gc.Equals,
		"80-100/tcp",
	)
	c.Assert(
		rule.GoString(),
		gc.Equals,
		"80-100/tcp",
	)

	rule, err = network.NewIngressRule("tcp", 80, 100, "0.0.0.0/0", "192.168.1.0/24")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rule.String(),
		gc.Equals,
		"80-100/tcp from 0.0.0.0/0,192.168.1.0/24",
	)
	c.Assert(
		rule.GoString(),
		gc.Equals,
		"80-100/tcp from 0.0.0.0/0,192.168.1.0/24",
	)
}

func (*FirewallSuite) TestSortIngressRules(c *gc.C) {
	rule1, err := network.NewIngressRule("udp", 10, 100, "0.0.0.0/0", "192.168.1.0/24")
	c.Assert(err, jc.ErrorIsNil)
	rule2, err := network.NewIngressRule("tcp", 80, 90, "0.0.0.0/0", "192.168.1.0/24")
	c.Assert(err, jc.ErrorIsNil)
	rule3, err := network.NewIngressRule("tcp", 80, 80, "0.0.0.0/0", "192.168.1.0/24")
	c.Assert(err, jc.ErrorIsNil)
	rule4, err := network.NewIngressRule("tcp", 80, 80, "0.0.0.0/0")
	c.Assert(err, jc.ErrorIsNil)

	ranges := []network.IngressRule{rule1, rule2, rule3, rule4}
	expected := []network.IngressRule{rule4, rule3, rule2, rule1}
	network.SortIngressRules(ranges)
	c.Assert(ranges, gc.DeepEquals, expected)
}

func (*FirewallSuite) TestOpenIngressRule(c *gc.C) {
	rule := network.NewOpenIngressRule("tcp", 80, 100)
	c.Assert(rule.Protocol, gc.Equals, "tcp")
	c.Assert(rule.FromPort, gc.Equals, 80)
	c.Assert(rule.ToPort, gc.Equals, 100)
	c.Assert(rule.SourceCIDRs, gc.IsNil)
}

func (*FirewallSuite) TestNewIngressRule(c *gc.C) {
	rule, err := network.NewIngressRule("tcp", 80, 100, "0.0.0.0/0", "192.168.1.0/24")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rule.Protocol, gc.Equals, "tcp")
	c.Assert(rule.FromPort, gc.Equals, 80)
	c.Assert(rule.ToPort, gc.Equals, 100)
	c.Assert(rule.SourceCIDRs, jc.DeepEquals, []string{"0.0.0.0/0", "192.168.1.0/24"})
}

func (*FirewallSuite) TestNewIngressRuleBadCIDR(c *gc.C) {
	_, err := network.NewIngressRule("tcp", 80, 100, "0.0.0.0/0", "192.168.0/24")
	c.Assert(err, gc.ErrorMatches, "invalid CIDR address: 192.168.0/24")
}
