// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	//"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
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
	svcNames := []string{
		"jujud-machine-0",
		"juju-mongod",
		"jujud-unit-wordpress-0",
	}
	for _, name := range svcNames {
		s.SetManaged(name, s.services)
		s.Init.Returns.Names = append(s.Init.Returns.Names, name)
	}
	s.Init.Returns.CheckPassed = true

	tags, err := service.ListAgents(s.services)
	c.Assert(err, jc.ErrorIsNil)

	var expected []names.Tag
	for _, name := range []string{"machine-0", "unit-wordpress-0"} {
		tag, err := names.ParseTag(name)
		c.Assert(err, jc.ErrorIsNil)
		expected = append(expected, tag)
	}
	c.Check(tags, jc.SameContents, expected)
}

func (s *agentSuite) TestNewAgentServiceSpec(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	tag, err := names.ParseTag(name[6:])
	c.Assert(err, jc.ErrorIsNil)
	spec, err := service.NewAgentServiceSpec(tag, s.Paths, service.InitSystemUpstart)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(spec.Name(), gc.Equals, name)
}

func (s *agentSuite) TestAgentServiceSpecName(c *gc.C) {
}

func (s *agentSuite) TestAgentServiceSpecToolsDir(c *gc.C) {
}

func (s *agentSuite) TestAgentServiceSpecConf(c *gc.C) {
}

func (s *agentSuite) TestNewAgentService(c *gc.C) {
}
