// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	//"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/initsystems"
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

	s.Files.Returns.Data = []byte("{}")
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

func (s *agentSuite) TestAgentServiceSpecToolsDir(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	tag, err := names.ParseTag(name[6:])
	c.Assert(err, jc.ErrorIsNil)
	spec, err := service.NewAgentServiceSpec(tag, s.Paths, service.InitSystemUpstart)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(spec.ToolsDir(), gc.Equals, "/var/lib/juju/tools/unit-wordpress-0")
}

func (s *agentSuite) TestAgentServiceSpecConf(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	tag, err := names.ParseTag(name[6:])
	c.Assert(err, jc.ErrorIsNil)
	spec, err := service.NewAgentServiceSpec(tag, s.Paths, service.InitSystemUpstart)
	c.Assert(err, jc.ErrorIsNil)
	conf := spec.Conf()

	c.Check(conf, jc.DeepEquals, service.Conf{Conf: initsystems.Conf{
		Desc: "juju agent for unit wordpress/0",
		Cmd:  `"/var/lib/juju/tools/unit-wordpress-0/jujud" unit --data-dir "/var/lib/juju" --unit-name "wordpress/0" --debug`,
		Out:  "/var/log/juju/unit-wordpress-0.log",
	}})
}

func (s *agentSuite) TestAgentServiceSpecConfWindows(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	tag, err := names.ParseTag(name[6:])
	c.Assert(err, jc.ErrorIsNil)
	spec, err := service.NewAgentServiceSpec(tag, s.Paths, service.InitSystemWindows)
	c.Assert(err, jc.ErrorIsNil)
	conf := spec.Conf()

	c.Check(conf, jc.DeepEquals, service.Conf{Conf: initsystems.Conf{
		Desc: "juju agent for unit wordpress/0",
		Cmd:  `"\var\lib\juju\tools\unit-wordpress-0\jujud.exe" unit --data-dir "\var\lib\juju" --unit-name "wordpress/0" --debug`,
	}})
}

func (s *agentSuite) TestNewAgentService(c *gc.C) {
	expName := "jujud-unit-wordpress-0"
	tag, err := names.ParseTag(expName[6:])
	c.Assert(err, jc.ErrorIsNil)
	svc, err := service.NewAgentService(tag, s.Paths, nil, s.services)
	c.Assert(err, jc.ErrorIsNil)
	name := svc.Name()
	conf := svc.Conf()

	c.Check(name, gc.Equals, expName)
	c.Check(conf, jc.DeepEquals, service.Conf{Conf: initsystems.Conf{
		Desc: "juju agent for unit wordpress/0",
		Cmd:  `"/var/lib/juju/tools/unit-wordpress-0/jujud" unit --data-dir "/var/lib/juju" --unit-name "wordpress/0" --debug`,
		Out:  "/var/log/juju/unit-wordpress-0.log",
	}})
}
