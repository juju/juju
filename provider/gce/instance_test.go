// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/provider/gce/google"
)

type instanceSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) TestNewInstance(c *gc.C) {
	inst := gce.NewInstance(s.BaseInstance, s.Env)

	c.Check(gce.ExposeInstBase(inst), gc.Equals, s.BaseInstance)
	c.Check(gce.ExposeInstEnv(inst), gc.Equals, s.Env)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestID(c *gc.C) {
	id := s.Instance.Id()

	c.Check(id, gc.Equals, instance.Id("spam"))
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestStatus(c *gc.C) {
	status := s.Instance.Status()

	c.Check(status, gc.Equals, google.StatusRunning)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestRefreshAPI(c *gc.C) {
	s.FakeConn.Inst = s.BaseInstance

	err := s.Instance.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "Instance")
	c.Check(s.FakeConn.Calls[0].ID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "home-zone")
}

func (s *instanceSuite) TestAddresses(c *gc.C) {
	addresses, err := s.Instance.Addresses()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(addresses, jc.DeepEquals, s.Addresses)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestOpenPortsAPI(c *gc.C) {
	err := s.Instance.OpenPorts("spam", s.Ports)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "OpenPorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, s.InstName)
	c.Check(s.FakeConn.Calls[0].PortRanges, jc.DeepEquals, s.Ports)
}

func (s *instanceSuite) TestClosePortsAPI(c *gc.C) {
	err := s.Instance.ClosePorts("spam", s.Ports)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ClosePorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, s.InstName)
	c.Check(s.FakeConn.Calls[0].PortRanges, jc.DeepEquals, s.Ports)
}

func (s *instanceSuite) TestPorts(c *gc.C) {
	s.FakeConn.PortRanges = s.Ports

	ports, err := s.Instance.Ports("spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ports, jc.DeepEquals, s.Ports)
}

func (s *instanceSuite) TestPortsAPI(c *gc.C) {
	_, err := s.Instance.Ports("spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "Ports")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, s.InstName)
}
