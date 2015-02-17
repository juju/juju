// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
)

var _ = gc.Suite(&servicesSuite{})

type servicesSuite struct {
	service.BaseSuite
}

func (s *servicesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

// TODO(ericsnow) Write the tests.

func (s *servicesSuite) TestDiscoverServices(c *gc.C) {
}

func (s *servicesSuite) TestInitSystem(c *gc.C) {
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
