// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/tools/lxdclient"
)

type instanceSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) TestNewInstance(c *gc.C) {
	inst := lxd.NewInstance(s.RawInstance, s.Env)

	c.Check(lxd.ExposeInstRaw(inst), gc.Equals, s.RawInstance)
	c.Check(lxd.ExposeInstEnv(inst), gc.Equals, s.Env)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestID(c *gc.C) {
	id := s.Instance.Id()

	c.Check(id, gc.Equals, instance.Id("spam"))
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestStatus(c *gc.C) {
	status := s.Instance.Status()

	c.Check(status, gc.Equals, lxdclient.StatusRunning)
	s.CheckNoAPI(c)
}

func (s *instanceSuite) TestAddresses(c *gc.C) {
	addresses, err := s.Instance.Addresses()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(addresses, jc.DeepEquals, s.Addresses)
}

func (s *instanceSuite) TestOpenPortsAPI(c *gc.C) {
	err := s.Instance.OpenPorts("spam", s.Ports)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "OpenPorts",
		Args: []interface{}{
			s.InstName,
			s.Ports,
		},
	}})
}

func (s *instanceSuite) TestClosePortsAPI(c *gc.C) {
	err := s.Instance.ClosePorts("spam", s.Ports)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "ClosePorts",
		Args: []interface{}{
			s.InstName,
			s.Ports,
		},
	}})
}

func (s *instanceSuite) TestPortsOkay(c *gc.C) {
	s.Firewaller.PortRanges = s.Ports

	ports, err := s.Instance.Ports("spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ports, jc.DeepEquals, s.Ports)
}

func (s *instanceSuite) TestPortsAPI(c *gc.C) {
	_, err := s.Instance.Ports("spam")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "Ports",
		Args: []interface{}{
			s.InstName,
		},
	}})
}
