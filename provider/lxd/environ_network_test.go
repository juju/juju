// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce"
)

type environNetSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environNetSuite{})

func (s *environNetSuite) TestGlobalFirewallName(c *gc.C) {
	uuid, _ := s.Config.UUID()
	fwname := gce.GlobalFirewallName(s.Env)

	c.Check(fwname, gc.Equals, "juju-"+uuid)
}

func (s *environNetSuite) TestOpenPorts(c *gc.C) {
	err := s.Env.OpenPorts(s.Ports)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environNetSuite) TestOpenPortsAPI(c *gc.C) {
	fwname := gce.GlobalFirewallName(s.Env)
	err := s.Env.OpenPorts(s.Ports)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "OpenPorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, fwname)
	c.Check(s.FakeConn.Calls[0].PortRanges, jc.DeepEquals, s.Ports)
}

func (s *environNetSuite) TestClosePorts(c *gc.C) {
	err := s.Env.ClosePorts(s.Ports)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environNetSuite) TestClosePortsAPI(c *gc.C) {
	fwname := gce.GlobalFirewallName(s.Env)
	err := s.Env.ClosePorts(s.Ports)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ClosePorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, fwname)
	c.Check(s.FakeConn.Calls[0].PortRanges, jc.DeepEquals, s.Ports)
}

func (s *environNetSuite) TestPorts(c *gc.C) {
	s.FakeConn.PortRanges = s.Ports

	ports, err := s.Env.Ports()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ports, jc.DeepEquals, s.Ports)
}

func (s *environNetSuite) TestPortsAPI(c *gc.C) {
	fwname := gce.GlobalFirewallName(s.Env)
	_, err := s.Env.Ports()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "Ports")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, fwname)
}
