// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/tc"

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
	status := s.Instance.Status(c.Context()).Message

	c.Check(status, tc.Equals, google.StatusRunning)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestAddresses(c *tc.C) {
	addresses, err := s.Instance.Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(addresses, tc.DeepEquals, s.Addresses)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestOpenPortsAPI(c *tc.C) {
	err := s.Instance.OpenPorts(c.Context(), "42", s.Rules)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "OpenPorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, tc.Equals, s.InstName)
	c.Check(s.FakeConn.Calls[0].Rules, tc.DeepEquals, s.Rules)
}

func (s *instanceSuite) TestClosePortsAPI(c *tc.C) {
	err := s.Instance.ClosePorts(c.Context(), "42", s.Rules)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ClosePorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, tc.Equals, s.InstName)
	c.Check(s.FakeConn.Calls[0].Rules, tc.DeepEquals, s.Rules)
}

func (s *instanceSuite) TestPorts(c *tc.C) {
	s.FakeConn.Rules = s.Rules

	ports, err := s.Instance.IngressRules(c.Context(), "42")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ports, tc.DeepEquals, s.Rules)
}

func (s *instanceSuite) TestPortsAPI(c *tc.C) {
	_, err := s.Instance.IngressRules(c.Context(), "42")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "Ports")
	c.Check(s.FakeConn.Calls[0].FirewallName, tc.Equals, s.InstName)
}
