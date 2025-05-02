// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/provider/gce"
)

type environFirewallSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environFirewallSuite{})

func (s *environFirewallSuite) TestGlobalFirewallName(c *gc.C) {
	uuid := s.Config.UUID()
	fwname := gce.GlobalFirewallName(s.Env)

	c.Check(fwname, gc.Equals, "juju-"+uuid)
}

func (s *environFirewallSuite) TestOpenPortsInvalidCredentialError(c *gc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	err := s.Env.OpenPorts(context.Background(), s.Rules)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environFirewallSuite) TestOpenPorts(c *gc.C) {
	err := s.Env.OpenPorts(context.Background(), s.Rules)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environFirewallSuite) TestOpenPortsAPI(c *gc.C) {
	fwname := gce.GlobalFirewallName(s.Env)
	err := s.Env.OpenPorts(context.Background(), s.Rules)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "OpenPorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, fwname)
	c.Check(s.FakeConn.Calls[0].Rules, jc.DeepEquals, s.Rules)
}

func (s *environFirewallSuite) TestClosePorts(c *gc.C) {
	err := s.Env.ClosePorts(context.Background(), s.Rules)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environFirewallSuite) TestClosePortsInvalidCredentialError(c *gc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	err := s.Env.ClosePorts(context.Background(), s.Rules)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environFirewallSuite) TestClosePortsAPI(c *gc.C) {
	fwname := gce.GlobalFirewallName(s.Env)
	err := s.Env.ClosePorts(context.Background(), s.Rules)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ClosePorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, fwname)
	c.Check(s.FakeConn.Calls[0].Rules, jc.DeepEquals, s.Rules)
}

func (s *environFirewallSuite) TestPorts(c *gc.C) {
	s.FakeConn.Rules = s.Rules

	ports, err := s.Env.IngressRules(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ports, jc.DeepEquals, s.Rules)
}

func (s *environFirewallSuite) TestIngressRulesInvalidCredentialError(c *gc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	_, err := s.Env.IngressRules(context.Background())
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environFirewallSuite) TestPortsAPI(c *gc.C) {
	fwname := gce.GlobalFirewallName(s.Env)
	_, err := s.Env.IngressRules(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "Ports")
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, fwname)
}
