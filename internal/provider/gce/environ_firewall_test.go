// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/provider/gce"
)

type environFirewallSuite struct {
	gce.BaseSuite
}

var _ = tc.Suite(&environFirewallSuite{})

func (s *environFirewallSuite) TestGlobalFirewallName(c *tc.C) {
	uuid := s.Config.UUID()
	fwname := gce.GlobalFirewallName(s.Env)

	c.Check(fwname, tc.Equals, "juju-"+uuid)
}

func (s *environFirewallSuite) TestOpenPortsInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	err := s.Env.OpenPorts(context.Background(), s.Rules)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environFirewallSuite) TestOpenPorts(c *tc.C) {
	err := s.Env.OpenPorts(context.Background(), s.Rules)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environFirewallSuite) TestOpenPortsAPI(c *tc.C) {
	fwname := gce.GlobalFirewallName(s.Env)
	err := s.Env.OpenPorts(context.Background(), s.Rules)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "OpenPorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, tc.Equals, fwname)
	c.Check(s.FakeConn.Calls[0].Rules, jc.DeepEquals, s.Rules)
}

func (s *environFirewallSuite) TestClosePorts(c *tc.C) {
	err := s.Env.ClosePorts(context.Background(), s.Rules)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environFirewallSuite) TestClosePortsInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	err := s.Env.ClosePorts(context.Background(), s.Rules)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environFirewallSuite) TestClosePortsAPI(c *tc.C) {
	fwname := gce.GlobalFirewallName(s.Env)
	err := s.Env.ClosePorts(context.Background(), s.Rules)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ClosePorts")
	c.Check(s.FakeConn.Calls[0].FirewallName, tc.Equals, fwname)
	c.Check(s.FakeConn.Calls[0].Rules, jc.DeepEquals, s.Rules)
}

func (s *environFirewallSuite) TestPorts(c *tc.C) {
	s.FakeConn.Rules = s.Rules

	ports, err := s.Env.IngressRules(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ports, jc.DeepEquals, s.Rules)
}

func (s *environFirewallSuite) TestIngressRulesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	_, err := s.Env.IngressRules(context.Background())
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environFirewallSuite) TestPortsAPI(c *tc.C) {
	fwname := gce.GlobalFirewallName(s.Env)
	_, err := s.Env.IngressRules(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "Ports")
	c.Check(s.FakeConn.Calls[0].FirewallName, tc.Equals, fwname)
}
