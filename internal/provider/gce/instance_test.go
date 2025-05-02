// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/provider/gce/google"
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
	status := s.Instance.Status(context.Background()).Message

	c.Check(status, gc.Equals, google.StatusRunning)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestAddresses(c *gc.C) {
	addresses, err := s.Instance.Addresses(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(addresses, jc.DeepEquals, s.Addresses)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestOpenPortsAPI(c *gc.C) {
	err := s.Instance.OpenPorts(context.Background(), "42", s.Rules)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "OpenPorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, s.InstName)
	c.Check(s.FakeConn.Calls[0].Rules, jc.DeepEquals, s.Rules)
}

func (s *instanceSuite) TestClosePortsAPI(c *gc.C) {
	err := s.Instance.ClosePorts(context.Background(), "42", s.Rules)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ClosePorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, s.InstName)
	c.Check(s.FakeConn.Calls[0].Rules, jc.DeepEquals, s.Rules)
}

func (s *instanceSuite) TestPorts(c *gc.C) {
	s.FakeConn.Rules = s.Rules

	ports, err := s.Instance.IngressRules(context.Background(), "42")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ports, jc.DeepEquals, s.Rules)
}

func (s *instanceSuite) TestPortsAPI(c *gc.C) {
	_, err := s.Instance.IngressRules(context.Background(), "42")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "Ports")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, s.InstName)
}
