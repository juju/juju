// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/provider/gce/google"
)

type instanceSuite struct {
	gce.BaseSuite
}

var _ = tc.Suite(&instanceSuite{})

func (s *instanceSuite) TestNewInstance(c *tc.C) {
	inst := gce.NewInstance(s.BaseInstance, s.Env)

	c.Check(gce.ExposeInstBase(inst), tc.Equals, s.BaseInstance)
	c.Check(gce.ExposeInstEnv(inst), tc.Equals, s.Env)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestID(c *tc.C) {
	id := s.Instance.Id()

	c.Check(id, tc.Equals, instance.Id("spam"))
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestStatus(c *tc.C) {
	status := s.Instance.Status(context.Background()).Message

	c.Check(status, tc.Equals, google.StatusRunning)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestAddresses(c *tc.C) {
	addresses, err := s.Instance.Addresses(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(addresses, jc.DeepEquals, s.Addresses)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestOpenPortsAPI(c *tc.C) {
	err := s.Instance.OpenPorts(context.Background(), "42", s.Rules)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "OpenPorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, tc.Equals, s.InstName)
	c.Check(s.FakeConn.Calls[0].Rules, jc.DeepEquals, s.Rules)
}

func (s *instanceSuite) TestClosePortsAPI(c *tc.C) {
	err := s.Instance.ClosePorts(context.Background(), "42", s.Rules)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ClosePorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, tc.Equals, s.InstName)
	c.Check(s.FakeConn.Calls[0].Rules, jc.DeepEquals, s.Rules)
}

func (s *instanceSuite) TestPorts(c *tc.C) {
	s.FakeConn.Rules = s.Rules

	ports, err := s.Instance.IngressRules(context.Background(), "42")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ports, jc.DeepEquals, s.Rules)
}

func (s *instanceSuite) TestPortsAPI(c *tc.C) {
	_, err := s.Instance.IngressRules(context.Background(), "42")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "Ports")
	c.Check(s.FakeConn.Calls[0].FirewallName, tc.Equals, s.InstName)
}
