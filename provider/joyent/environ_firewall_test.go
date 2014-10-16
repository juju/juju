// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package joyent_test

import (
	"github.com/joyent/gosdc/cloudapi"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/joyent"
)

type FirewallSuite struct{}

var _ = gc.Suite(&FirewallSuite{})

func (s *FirewallSuite) TestGetPorts(c *gc.C) {
	testCases := []struct {
		about    string
		envName  string
		rules    []cloudapi.FirewallRule
		expected []network.PortRange
	}{
		{
			"single port environment rule",
			"env",
			[]cloudapi.FirewallRule{{
				"",
				true,
				"FROM tag env TO tag juju ALLOW tcp PORT 80",
			}},
			[]network.PortRange{{
				FromPort: 80,
				ToPort:   80,
				Protocol: "tcp",
			}},
		},
		{
			"port range environment rule",
			"env",
			[]cloudapi.FirewallRule{{
				"",
				true,
				"FROM tag env TO tag juju ALLOW tcp (PORT 80 AND PORT 81 AND PORT 82 AND PORT 83)",
			}},
			[]network.PortRange{{
				FromPort: 80,
				ToPort:   83,
				Protocol: "tcp",
			}},
		},
	}
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		c.Assert(joyent.GetPorts(t.envName, t.rules), gc.DeepEquals, t.expected)
	}

}

func (s *FirewallSuite) TestRuleCreation(c *gc.C) {
	testCases := []struct {
		about    string
		ports    network.PortRange
		expected string
	}{{
		"single port firewall rule",
		network.PortRange{80, 80, "tcp"},
		"FROM tag env TO tag juju ALLOW tcp PORT 80",
	}, {
		"multiple port firewall rule",
		network.PortRange{80, 81, "tcp"},
		"FROM tag env TO tag juju ALLOW tcp ( PORT 80 AND PORT 81 )",
	}}

	for i, t := range testCases {
		c.Logf("test case %d: %s", i, t.about)
		rule := joyent.CreateFirewallRuleAll("env", t.ports)
		c.Check(rule, gc.Equals, t.expected)
	}
}
