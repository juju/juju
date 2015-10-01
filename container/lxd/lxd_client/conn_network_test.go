// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client_test

import (
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

func (s *connSuite) TestConnectionPorts(c *gc.C) {
	s.FakeConn.Firewall = &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81"},
		}},
	}

	ports, err := s.Conn.Ports("spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ports, jc.DeepEquals, []network.PortRange{{
		FromPort: 80,
		ToPort:   81,
		Protocol: "tcp",
	}})
}

func (s *connSuite) TestConnectionPortsAPI(c *gc.C) {
	s.FakeConn.Firewall = &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81"},
		}},
	}

	_, err := s.Conn.Ports("eggs")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "GetFirewall")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].Name, gc.Equals, "eggs")
}

func (s *connSuite) TestConnectionOpenPortsAdd(c *gc.C) {
	s.FakeConn.Err = errors.NotFoundf("spam")

	ports := network.PortRange{
		FromPort: 80,
		ToPort:   81,
		Protocol: "tcp",
	}
	err := s.Conn.OpenPorts("spam", ports)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "GetFirewall")
	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "AddFirewall")
	sort.Strings(s.FakeConn.Calls[1].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[1].Firewall, jc.DeepEquals, &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80", "81"},
		}},
	})
}

func (s *connSuite) TestConnectionOpenPortsUpdate(c *gc.C) {
	s.FakeConn.Firewall = &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81"},
		}},
	}

	ports := network.PortRange{
		FromPort: 443,
		ToPort:   443,
		Protocol: "tcp",
	}
	err := s.Conn.OpenPorts("spam", ports)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "GetFirewall")
	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "UpdateFirewall")
	sort.Strings(s.FakeConn.Calls[1].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[1].Firewall, jc.DeepEquals, &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"443", "80", "81"},
		}},
	})
}

func (s *connSuite) TestConnectionClosePortsRemove(c *gc.C) {
	s.FakeConn.Firewall = &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"443"},
		}},
	}

	ports := network.PortRange{
		FromPort: 443,
		ToPort:   443,
		Protocol: "tcp",
	}
	err := s.Conn.ClosePorts("spam", ports)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "GetFirewall")
	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[1].Name, gc.Equals, "spam")
}

func (s *connSuite) TestConnectionClosePortsUpdate(c *gc.C) {
	s.FakeConn.Firewall = &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80-81", "443"},
		}},
	}

	ports := network.PortRange{
		FromPort: 443,
		ToPort:   443,
		Protocol: "tcp",
	}
	err := s.Conn.ClosePorts("spam", ports)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "GetFirewall")
	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "UpdateFirewall")
	sort.Strings(s.FakeConn.Calls[1].Firewall.Allowed[0].Ports)
	c.Check(s.FakeConn.Calls[1].Firewall, jc.DeepEquals, &compute.Firewall{
		Name:         "spam",
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      []string{"80", "81"},
		}},
	})
}
