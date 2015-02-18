// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	//"github.com/juju/errors"
	//"github.com/juju/names"
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
)

var _ = gc.Suite(&agentSuite{})

type agentSuite struct {
	service.BaseSuite

	services *service.Services
}

func (s *agentSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.services = service.NewServices(c.MkDir(), s.Init)
	s.Stub.Calls = nil
}

func (s *agentSuite) TestListAgents(c *gc.C) {
}

func (s *agentSuite) TestNewAgentServiceSpec(c *gc.C) {
}

func (s *agentSuite) TestDiscoverAgentServiceSpec(c *gc.C) {
}

func (s *agentSuite) TestAgentServiceSpecName(c *gc.C) {
}

func (s *agentSuite) TestAgentServiceSpecToolsDir(c *gc.C) {
}

func (s *agentSuite) TestAgentServiceSpecConf(c *gc.C) {
}

func (s *agentSuite) TestNewAgentService(c *gc.C) {
}
