// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/initsystems"
)

var _ = gc.Suite(&servicesSuite{})

type servicesSuite struct {
	service.BaseSuite

	services *service.Services
}

func (s *servicesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.services = service.NewServices(c.MkDir(), s.Init)
	s.Stub.Calls = nil
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

func (s *servicesSuite) TestStart(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true

	err := s.services.Start(name)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "IsEnabled", "Check", "Start")
}

func (s *servicesSuite) TestStartAlreadyStarted(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true
	failure := errors.AlreadyExistsf(name)
	s.Stub.SetErrors(nil, failure) // IsEnabled, Start

	err := s.services.Start(name)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "IsEnabled", "Check", "Start")
}

func (s *servicesSuite) TestStartNotManaged(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	err := s.services.Start(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.Stub.CheckCalls(c, nil)
}

func (s *servicesSuite) TestStartCheckFailed(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true

	err := s.services.Start(name)

	c.Check(errors.Cause(err), gc.Equals, service.ErrNotManaged)
	s.Stub.CheckCallNames(c, "IsEnabled", "Check")
}

func (s *servicesSuite) TestStartNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = false
	failure := errors.NotFoundf(name)
	s.Stub.SetErrors(nil, failure) // IsEnabled, Start

	err := s.services.Start(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.Stub.CheckCallNames(c, "IsEnabled", "Start")
}

func (s *servicesSuite) TestStop(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true

	err := s.services.Stop(name)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "IsEnabled", "Check", "Stop")
}

func (s *servicesSuite) TestStopNotRunning(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = false
	failure := errors.NotFoundf(name)
	s.Stub.SetErrors(nil, failure) // IsEnabled, Stop

	err := s.services.Stop(name)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "IsEnabled", "Stop")
}

func (s *servicesSuite) TestStopNotManaged(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	err := s.services.Stop(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.Stub.CheckCalls(c, nil)
}

func (s *servicesSuite) TestStopCheckFailed(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true

	err := s.services.Stop(name)

	c.Check(errors.Cause(err), gc.Equals, service.ErrNotManaged)
	s.Stub.CheckCallNames(c, "IsEnabled", "Check")
}

func (s *servicesSuite) TestStopNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = false
	failure := errors.NotFoundf(name)
	s.Stub.SetErrors(nil, failure) // IsEnabled, Stop

	err := s.services.Stop(name)
	c.Check(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "IsEnabled", "Stop")
}

func (s *servicesSuite) TestIsRunningTrue(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true
	s.Init.Returns.Info = &initsystems.ServiceInfo{
		Status: initsystems.StatusRunning,
	}

	running, err := s.services.IsRunning(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(running, jc.IsTrue)
	s.Stub.CheckCallNames(c, "IsEnabled", "Check", "Info")
}

func (s *servicesSuite) TestIsRunningFalse(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true
	s.Init.Returns.Info = &initsystems.ServiceInfo{
		Status: initsystems.StatusStopped,
	}

	running, err := s.services.IsRunning(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(running, jc.IsFalse)
	s.Stub.CheckCallNames(c, "IsEnabled", "Check", "Info")
}

func (s *servicesSuite) TestIsRunningNotManaged(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	running, err := s.services.IsRunning(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(running, jc.IsFalse)
	s.Stub.CheckCalls(c, nil)
}

func (s *servicesSuite) TestIsRunningCheckFailed(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true

	_, err := s.services.IsRunning(name)

	c.Check(errors.Cause(err), gc.Equals, service.ErrNotManaged)
	s.Stub.CheckCallNames(c, "IsEnabled", "Check")
}

func (s *servicesSuite) TestIsRunningNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = false
	failure := errors.NotFoundf(name)
	s.Stub.SetErrors(nil, failure) // IsEnabled, IsRunning

	running, err := s.services.IsRunning(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(running, jc.IsFalse)
	s.Stub.CheckCallNames(c, "IsEnabled", "Info")
}

// TODO(ericsnow) Write the tests.

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
