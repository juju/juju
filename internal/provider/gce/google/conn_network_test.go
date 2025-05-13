// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"regexp"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/core/network"
	corefirewall "github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/internal/provider/gce/google"
)

func (s *connSuite) TestConnectionIngressRules(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Allowed: []*compute.FirewallAllowed{
			{
				IPProtocol: "tcp",
				Ports:      []string{"80-81", "92"},
			}, {
				IPProtocol: "udp",
				Ports:      []string{"443", "100-120"},
			},
		},
	}}

	ports, err := s.Conn.IngressRules("spam")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		ports, tc.DeepEquals,
		corefirewall.IngressRules{
			corefirewall.NewIngressRule(network.MustParsePortRange("80-81/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
			corefirewall.NewIngressRule(network.MustParsePortRange("92/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
			corefirewall.NewIngressRule(network.MustParsePortRange("100-120/udp"), "10.0.0.0/24", "192.168.1.0/24"),
			corefirewall.NewIngressRule(network.MustParsePortRange("443/udp"), "10.0.0.0/24", "192.168.1.0/24"),
		},
	)
}

func (s *connSuite) TestConnectionIngressRulesCollapse(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"81"},
		}, {
			IPProtocol: "tcp",
			Ports:      []string{"82"},
		}, {
			IPProtocol: "tcp",
			Ports:      []string{"80"},
		}, {
			IPProtocol: "tcp",
			Ports:      []string{"83"},
		}, {
			IPProtocol: "tcp",
			Ports:      []string{"92"},
		}},
	}}

	ports, err := s.Conn.IngressRules("spam")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		ports, tc.DeepEquals,
		corefirewall.IngressRules{
			corefirewall.NewIngressRule(network.MustParsePortRange("80-83/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
			corefirewall.NewIngressRule(network.MustParsePortRange("92/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		},
	)
}

func (s *connSuite) TestConnectionIngressRulesDefaultCIDR(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:       "spam",
		TargetTags: []string{"spam"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81", "92"},
		}},
	}}

	ports, err := s.Conn.IngressRules("spam")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		ports, tc.DeepEquals,
		corefirewall.IngressRules{
			corefirewall.NewIngressRule(network.MustParsePortRange("80-81/tcp"), corefirewall.AllNetworksIPV4CIDR),
			corefirewall.NewIngressRule(network.MustParsePortRange("92/tcp"), corefirewall.AllNetworksIPV4CIDR),
		},
	)
}

func (s *connSuite) TestConnectionPortsAPI(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81"},
		}},
	}}

	_, err := s.Conn.IngressRules("eggs")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetFirewalls")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].Name, tc.Equals, "eggs")
}

func (s *connSuite) TestConnectionOpenPortsAdd(c *tc.C) {
	s.FakeConn.Err = errors.NotFoundf("spam")

	rules := corefirewall.IngressRules{
		corefirewall.NewIngressRule(network.MustParsePortRange("80-81/tcp")), // leave out CIDR to check default
		corefirewall.NewIngressRule(network.MustParsePortRange("80-81/udp"), corefirewall.AllNetworksIPV4CIDR),
		corefirewall.NewIngressRule(network.MustParsePortRange("100-120/tcp"), "192.168.1.0/24", "10.0.0.0/24"),
		corefirewall.NewIngressRule(network.MustParsePortRange("67/udp"), "10.0.0.0/24"),
	}
	err := s.Conn.OpenPortsWithNamer("spam", google.HashSuffixNamer, rules)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 4)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetFirewalls")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "AddFirewall")
	c.Check(s.FakeConn.Calls[1].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "spam-4eebe8d7a9",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"100-120"},
		}},
	})
	c.Check(s.FakeConn.Calls[2].FuncName, tc.Equals, "AddFirewall")
	c.Check(s.FakeConn.Calls[2].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "spam-a34d80f7b6",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"10.0.0.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "udp",
			Ports:      []string{"67"},
		}},
	})
	c.Check(s.FakeConn.Calls[3].FuncName, tc.Equals, "AddFirewall")
	c.Check(s.FakeConn.Calls[3].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81"},
		}, {
			IPProtocol: "udp",
			Ports:      []string{"80-81"},
		}},
	})
}

func (s *connSuite) TestConnectionOpenPortsUpdateSameCIDR(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "spam-ad7554",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"192.168.1.0/24", "10.0.0.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81"},
		}},
	}}

	rules := corefirewall.IngressRules{
		corefirewall.NewIngressRule(network.MustParsePortRange("443/tcp"), "192.168.1.0/24", "10.0.0.0/24"),
	}
	err := s.Conn.OpenPortsWithNamer("spam", google.HashSuffixNamer, rules)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetFirewalls")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "UpdateFirewall")
	sort.Strings(s.FakeConn.Calls[1].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[1].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "spam-ad7554",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"443", "80-81"},
		}},
	})
}

func (s *connSuite) TestConnectionOpenPortsUpdateAddCIDR(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "spam-arbitrary-name",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"192.168.1.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81"},
		}},
	}}

	rules := corefirewall.IngressRules{
		corefirewall.NewIngressRule(network.MustParsePortRange("80-81/tcp"), "10.0.0.0/24"),
	}
	err := s.Conn.OpenPortsWithNamer("spam", google.HashSuffixNamer, rules)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetFirewalls")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "UpdateFirewall")
	sort.Strings(s.FakeConn.Calls[1].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[1].Name, tc.Equals, "spam-arbitrary-name")
	c.Check(s.FakeConn.Calls[1].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "spam-arbitrary-name",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81"},
		}},
	})
}

func (s *connSuite) TestConnectionOpenPortsUpdateAndAdd(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "spam-d01a82",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"192.168.1.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81"},
		}},
	}, {
		Name:         "spam-8e65efabcd",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"172.0.0.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"100-120", "443"},
		}},
	}}

	rules := corefirewall.IngressRules{
		corefirewall.NewIngressRule(network.MustParsePortRange("443/tcp"), "192.168.1.0/24"),
		corefirewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), "10.0.0.0/24"),
		corefirewall.NewIngressRule(network.MustParsePortRange("443/tcp"), "10.0.0.0/24"),
		corefirewall.NewIngressRule(network.MustParsePortRange("67/udp"), "172.0.0.0/24"),
	}
	err := s.Conn.OpenPortsWithNamer("spam", google.HashSuffixNamer, rules)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 4)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetFirewalls")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "UpdateFirewall")
	sort.Strings(s.FakeConn.Calls[1].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[1].Name, tc.Equals, "spam-8e65efabcd")
	c.Check(s.FakeConn.Calls[1].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "spam-8e65efabcd",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"172.0.0.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"100-120", "443"},
		}, {
			IPProtocol: "udp",
			Ports:      []string{"67"},
		}},
	})
	c.Check(s.FakeConn.Calls[2].FuncName, tc.Equals, "AddFirewall")
	sort.Strings(s.FakeConn.Calls[2].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[2].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "spam-a34d80f7b6",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"10.0.0.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"443", "80-100"},
		}},
	})
	c.Check(s.FakeConn.Calls[3].FuncName, tc.Equals, "UpdateFirewall")
	sort.Strings(s.FakeConn.Calls[3].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[3].Name, tc.Equals, "spam-d01a82")
	c.Check(s.FakeConn.Calls[3].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "spam-d01a82",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"192.168.1.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"443", "80-81"},
		}},
	})
}

func (s *connSuite) TestConnectionClosePortsRemove(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"443"},
		}},
	}}

	rules := corefirewall.IngressRules{
		corefirewall.NewIngressRule(network.MustParsePortRange("443/tcp")),
	}
	err := s.Conn.ClosePorts("spam", rules)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetFirewalls")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[1].Name, tc.Equals, "spam")
}

func (s *connSuite) TestConnectionClosePortsUpdate(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81", "443"},
		}},
	}}

	rules := corefirewall.IngressRules{
		corefirewall.NewIngressRule(network.MustParsePortRange("443/tcp")),
	}
	err := s.Conn.ClosePorts("spam", rules)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetFirewalls")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "UpdateFirewall")
	sort.Strings(s.FakeConn.Calls[1].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[1].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81"},
		}},
	})
}

func (s *connSuite) TestConnectionClosePortsCollapseUpdate(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-80", "100-120", "81-81", "82-82"},
		}},
	}}

	rules := corefirewall.IngressRules{
		corefirewall.NewIngressRule(network.MustParsePortRange("80-82/tcp")),
	}
	err := s.Conn.ClosePorts("spam", rules)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetFirewalls")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "UpdateFirewall")
	sort.Strings(s.FakeConn.Calls[1].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[1].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"100-120"},
		}},
	})
}

func (s *connSuite) TestConnectionClosePortsRemoveCIDR(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "glass-onion",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"192.168.1.0/24", "10.0.0.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81", "443"},
		}},
	}}

	rules := corefirewall.IngressRules{
		corefirewall.NewIngressRule(network.MustParsePortRange("443/tcp"), "192.168.1.0/24"),
		corefirewall.NewIngressRule(network.MustParsePortRange("80-81/tcp"), "192.168.1.0/24"),
	}
	err := s.Conn.ClosePorts("spam", rules)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetFirewalls")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "UpdateFirewall")
	sort.Strings(s.FakeConn.Calls[1].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[1].Firewall, tc.DeepEquals, &compute.Firewall{
		Name:         "glass-onion",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"10.0.0.0/24"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"443", "80-81"},
		}},
	})
}

func (s *connSuite) TestRemoveFirewall(c *tc.C) {
	err := s.Conn.RemoveFirewall("glass-onion")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].Name, tc.Equals, "glass-onion")
}

func (s *connSuite) TestConnectionCloseMoMatches(c *tc.C) {
	s.FakeConn.Firewalls = []*compute.Firewall{{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81", "443"},
		}},
	}}

	rules := corefirewall.IngressRules{
		corefirewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), "192.168.0.1/24"),
	}
	err := s.Conn.ClosePorts("spam", rules)
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`closing port(s) [100-110/tcp from 192.168.0.1/24] over non-matching rules not supported`))

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetFirewalls")
}

func (s *connSuite) TestNetworks(c *tc.C) {
	s.FakeConn.Networks = []*compute.Network{{
		Name: "kamar-taj",
	}}
	results, err := s.Conn.Networks()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert((*results[0]).Name, tc.Equals, "kamar-taj")

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListNetworks")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
}

func (s *connSuite) TestSubnetworks(c *tc.C) {
	s.FakeConn.Subnetworks = []*compute.Subnetwork{{
		Name: "heptapod",
	}}
	results, err := s.Conn.Subnetworks("us-central1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert((*results[0]).Name, tc.Equals, "heptapod")

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListSubnetworks")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].Region, tc.Equals, "us-central1")
}

func (s *connSuite) TestRandomSuffixNamer(c *tc.C) {
	ruleset := google.NewRuleSetFromRules(corefirewall.IngressRules{
		corefirewall.NewIngressRule(network.MustParsePortRange("80-90/tcp")),
		corefirewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), "10.0.10.0/24"),
	})
	i := 0
	for _, firewall := range ruleset {
		i++
		c.Logf("%#v", *firewall)
		name, err := google.RandomSuffixNamer(firewall, "mischief", set.NewStrings())
		c.Assert(err, tc.ErrorIsNil)
		if firewall.SourceCIDRs[0] == "0.0.0.0/0" {
			c.Assert(name, tc.Equals, "mischief")
		} else {
			c.Assert(name, tc.Matches, "mischief-[0-9a-f]{8}")
		}
	}
	c.Assert(i, tc.Equals, 2)
}
