// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
)

var _ = gc.Suite(&servicesSuite{})

type servicesSuite struct {
	service.BaseSuite

	services *service.Services
}

func (s *servicesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.services = service.NewServices(c.MkDir(), s.Init)
}

func (s *servicesSuite) TestDiscoverServices(c *gc.C) {
	// TODO(ericsnow) Write the test?
}

func (s *servicesSuite) TestInitSystem(c *gc.C) {
	// Our choice of init system name here is not significant.
	expected := service.InitSystemUpstart
	s.Init.Returns.Name = expected

	name := s.services.InitSystem()

	c.Check(name, gc.Equals, expected)
}

func (s *servicesSuite) TestList(c *gc.C) {
	s.SetManaged("jujud-machine-0", s.services)
	s.SetManaged("juju-mongod", s.services)

	names, err := s.services.List()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{
		"jujud-machine-0",
		"juju-mongod",
	})
}

func (s *servicesSuite) TestListEmpty(c *gc.C) {
	names, err := s.services.List()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, gc.HasLen, 0)
}

func (s *servicesSuite) TestListEnabled(c *gc.C) {
	s.SetManaged("jujud-machine-0", s.services)
	s.SetManaged("juju-mongod", s.services)
	s.Init.Returns.Names = []string{
		"jujud-machine-0",
		"juju-mongod",
	}
	s.Init.Returns.CheckPassed = true

	names, err := s.services.ListEnabled()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{
		"jujud-machine-0",
		"juju-mongod",
	})
}

func (s *servicesSuite) TestListEnabledEmpty(c *gc.C) {
	names, err := s.services.List()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, gc.HasLen, 0)
}

// TODO(ericsnow) Write the tests.

func (s *servicesSuite) TestStart(c *gc.C) {
}

func (s *servicesSuite) TestStop(c *gc.C) {
}

func (s *servicesSuite) TestIsRunning(c *gc.C) {
}

func (s *servicesSuite) TestEnable(c *gc.C) {
}

func (s *servicesSuite) TestDisable(c *gc.C) {
}

func (s *servicesSuite) TestIsEnabled(c *gc.C) {
}

func (s *servicesSuite) TestManage(c *gc.C) {
}

func (s *servicesSuite) TestRemove(c *gc.C) {
}

func (s *servicesSuite) TestInstall(c *gc.C) {
}

func (s *servicesSuite) TestCheck(c *gc.C) {
}

func (s *servicesSuite) TestIsManaged(c *gc.C) {
}

func (s *servicesSuite) TestNewService(c *gc.C) {
}

func (s *servicesSuite) TestNewAgentService(c *gc.C) {
}
