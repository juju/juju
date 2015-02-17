// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/initsystems"
)

var _ = gc.Suite(&servicesSuite{})

type servicesSuite struct {
	service.BaseSuite

	init     *initsystems.Stub
	services *service.Services
}

func (s *servicesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.init = initsystems.NewStub()
	s.services = service.NewServices(c.MkDir(), s.init)
}

// TODO(ericsnow) Write the tests.

func (s *servicesSuite) TestDiscoverServices(c *gc.C) {
	// TODO(ericsnow) Write the test?
}

func (s *servicesSuite) TestInitSystem(c *gc.C) {
	// Our choice of init system name here is not significant.
	expected := service.InitSystemUpstart
	s.init.Returns.Name = expected

	name := s.services.InitSystem()

	c.Check(name, gc.Equals, expected)
}

func (s *servicesSuite) TestList(c *gc.C) {
}

func (s *servicesSuite) TestListEnabled(c *gc.C) {
}

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
