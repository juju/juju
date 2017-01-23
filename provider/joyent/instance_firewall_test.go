// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package joyent_test

import (
	"github.com/joyent/gosdc/cloudapi"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/joyent"
)

type InstanceFirewallSuite struct{}

var _ = gc.Suite(&InstanceFirewallSuite{})

func (s *InstanceFirewallSuite) TestGetPorts(c *gc.C) {
	testCases := []struct {
		about    string
		envName  string
		rules    []cloudapi.FirewallRule
		expected []network.IngressRule
	}{
		{
			"single port instance rule",
			"switch",
			[]cloudapi.FirewallRule{{
				"",
				true,
				"FROM tag switch TO vm machine ALLOW tcp PORT 80",
			}},
			[]network.IngressRule{network.MustNewIngressRule("tcp", 80, 80)},
		},
		{
			"port range instance rule",
			"switch",
			[]cloudapi.FirewallRule{{
				"",
				true,
				"FROM tag switch TO vm machine ALLOW tcp (PORT 80 AND PORT 81 AND PORT 82 AND PORT 83)",
			}},
			[]network.IngressRule{network.MustNewIngressRule("tcp", 80, 83)},
		},
	}
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		ports, err := joyent.GetPorts(t.envName, t.rules)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ports, jc.DeepEquals, t.expected)
	}

}

func (s *InstanceFirewallSuite) TestRuleCreation(c *gc.C) {
	testCases := []struct {
		about    string
		ports    network.IngressRule
		expected string
	}{{
		"single port firewall rule",
		network.MustNewIngressRule("tcp", 80, 80),
		"FROM tag switch TO vm machine ALLOW tcp PORT 80",
	}, {
		"multiple port firewall rule",
		network.MustNewIngressRule("tcp", 80, 81),
		"FROM tag switch TO vm machine ALLOW tcp ( PORT 80 AND PORT 81 )",
	}}

	for i, t := range testCases {
		c.Logf("test case %d: %s", i, t.about)
		rule := joyent.CreateFirewallRuleVm("switch", "machine", t.ports)
		c.Check(rule, gc.Equals, t.expected)
	}
}
