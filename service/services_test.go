// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
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

	s.Files.Returns.Data = []byte("{}")
}

func (s *servicesSuite) setPostReadErrors(errs ...error) {
	confDirReadErrors := []error{nil, nil, nil} // ReadFile, ReadFile, ListDir
	s.Stub.SetErrors(append(confDirReadErrors, errs...)...)
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

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
		"Start",
	)
}

func (s *servicesSuite) TestStartAlreadyStarted(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true
	failure := errors.AlreadyExistsf(name)
	s.setPostReadErrors(nil, failure) // IsEnabled, Start

	err := s.services.Start(name)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
		"Start",
	)
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
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
	)
}

func (s *servicesSuite) TestStartNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = false
	failure := errors.NotFoundf(name)
	s.setPostReadErrors(nil, failure) // IsEnabled, Start

	err := s.services.Start(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Start",
	)
}

func (s *servicesSuite) TestStop(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true

	err := s.services.Stop(name)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
		"Stop",
	)
}

func (s *servicesSuite) TestStopNotRunning(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Files.Returns.Data = []byte("{}")
	s.Init.Returns.Enabled = false
	failure := errors.NotFoundf(name)
	s.setPostReadErrors(nil, failure) // IsEnabled, Stop

	err := s.services.Stop(name)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Stop",
	)
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
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
	)
}

func (s *servicesSuite) TestStopNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = false
	failure := errors.NotFoundf(name)
	s.setPostReadErrors(nil, failure) // IsEnabled, Stop

	err := s.services.Stop(name)
	c.Check(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Stop",
	)
}

func (s *servicesSuite) TestIsRunningTrue(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true
	s.Init.Returns.Info = initsystems.ServiceInfo{
		Status: initsystems.StatusRunning,
	}

	running, err := s.services.IsRunning(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(running, jc.IsTrue)
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
		"Info",
	)
}

func (s *servicesSuite) TestIsRunningFalse(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true
	s.Init.Returns.Info = initsystems.ServiceInfo{
		Status: initsystems.StatusStopped,
	}

	running, err := s.services.IsRunning(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(running, jc.IsFalse)
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
		"Info",
	)
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
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
	)
}

func (s *servicesSuite) TestIsRunningNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = false
	failure := errors.NotFoundf(name)
	s.setPostReadErrors(nil, failure) // IsEnabled, IsRunning

	running, err := s.services.IsRunning(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(running, jc.IsFalse)
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Info",
	)
}

func (s *servicesSuite) TestEnable(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = false

	err := s.services.Enable(name)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"Enable",
	)
}

func (s *servicesSuite) TestEnableNotManaged(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	err := s.services.Enable(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.Stub.CheckCalls(c, nil)
}

func (s *servicesSuite) TestEnableAlreadyEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.CheckPassed = true
	failure := errors.AlreadyExistsf(name)
	s.setPostReadErrors(failure) // Enable

	err := s.services.Enable(name)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"Enable",
		"Check",
	)
}

func (s *servicesSuite) TestEnableCheckFailed(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.CheckPassed = false
	failure := errors.AlreadyExistsf(name)
	s.setPostReadErrors(failure) // Enable

	err := s.services.Enable(name)

	c.Check(errors.Cause(err), gc.Equals, service.ErrNotManaged)
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"Enable",
		"Check",
	)
}

func (s *servicesSuite) TestDisable(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true

	err := s.services.Disable(name)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
		"Disable",
	)
}

func (s *servicesSuite) TestDisableNotManaged(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	err := s.services.Disable(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.Stub.CheckCalls(c, nil)
}

func (s *servicesSuite) TestDisableNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = false
	failure := errors.NotFoundf(name)
	s.setPostReadErrors(nil, failure) // IsEnabled, Disable

	err := s.services.Disable(name)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Disable",
	)
}

func (s *servicesSuite) TestDisableCheckFailed(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = false

	err := s.services.Disable(name)

	c.Check(errors.Cause(err), gc.Equals, service.ErrNotManaged)
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
	)
}

func (s *servicesSuite) TestIsEnabledTrue(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true

	enabled, err := s.services.IsEnabled(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(enabled, jc.IsTrue)
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
		"IsEnabled",
	)
}

func (s *servicesSuite) TestIsEnabledFalse(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = false

	enabled, err := s.services.IsEnabled(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(enabled, jc.IsFalse)
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"IsEnabled",
	)
}

func (s *servicesSuite) TestIsEnabledNotManaged(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	enabled, err := s.services.IsEnabled(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(enabled, jc.IsFalse)
	s.Stub.CheckCalls(c, nil)
}

func (s *servicesSuite) TestIsEnabledCheckFailed(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = false

	_, err := s.services.IsEnabled(name)

	c.Check(errors.Cause(err), gc.Equals, service.ErrNotManaged)
	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
	)
}

func (s *servicesSuite) TestManage(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	err := s.services.Manage(name, s.Conf)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ManageCalls,
	)
}

func (s *servicesSuite) TestManageAlreadyManaged(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)

	err := s.services.Manage(name, s.Conf)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	s.Stub.CheckCalls(c, nil)
}

func (s *servicesSuite) TestRemove(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)

	err := s.services.Remove(name)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"RemoveAll",
	)
}

func (s *servicesSuite) TestRemoveEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = true

	err := s.services.Remove(name)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
		"RemoveAll",
		"Stop",
		"Disable",
	)
}

func (s *servicesSuite) TestRemoveCheckFailed(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)
	s.Init.Returns.Enabled = true
	s.Init.Returns.CheckPassed = false

	err := s.services.Remove(name)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		service.ConfDirReadCalls,
		"IsEnabled",
		"Check",
		"RemoveAll",
	)
}

func (s *servicesSuite) TestIsManagedTrue(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.SetManaged(name, s.services)

	managed := s.services.IsManaged(name)

	c.Check(managed, jc.IsTrue)
	s.Stub.CheckCalls(c, nil)
}

func (s *servicesSuite) TestIsManagedFalse(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	managed := s.services.IsManaged(name)

	c.Check(managed, jc.IsFalse)
	s.Stub.CheckCalls(c, nil)
}

func (s *servicesSuite) TestInstall(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.Init.Returns.Enabled = false

	err := s.services.Install(name, s.Conf)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c,
		"IsEnabled",
		service.ManageCalls,
		service.ConfDirReadCalls,
		"Enable",
		"Start",
	)
}

func (s *servicesSuite) TestCheckSame(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.Init.Returns.Conf = s.Conf.Conf

	ok, err := s.services.Check(name, s.Conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ok, jc.IsTrue)
}

func (s *servicesSuite) TestCheckDifferent(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.Init.Returns.Conf = s.Conf.Conf

	conf := service.Conf{Conf: initsystems.Conf{
		Desc: "another service",
		Cmd:  "<a command>",
	}}
	ok, err := s.services.Check(name, conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ok, jc.IsFalse)
}

func (s *servicesSuite) TestNewService(c *gc.C) {
	expName := "jujud-unit-wordpress-0"
	svc := s.services.NewService(expName, s.Conf)
	name := svc.Name()
	conf := svc.Conf()

	c.Check(name, gc.Equals, expName)
	c.Check(conf, jc.DeepEquals, s.Conf)
}

func (s *servicesSuite) TestNewAgentService(c *gc.C) {
	tagStr := "unit-wordpress-0"
	tag, _ := names.ParseTag(tagStr)
	svc, err := s.services.NewAgentService(tag, s.Paths, nil)
	c.Assert(err, jc.ErrorIsNil)
	name := svc.Name()
	conf := svc.Conf()

	c.Check(name, gc.Equals, "jujud-"+tagStr)
	c.Check(conf, jc.DeepEquals, service.Conf{Conf: initsystems.Conf{
		Desc: "juju agent for unit wordpress/0",
		Cmd:  `"/var/lib/juju/tools/unit-wordpress-0/jujud" unit --data-dir "/var/lib/juju" --unit-name "wordpress/0" --debug`,
		Out:  "/var/log/juju/unit-wordpress-0.log",
	}})
}
