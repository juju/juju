// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package windows_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/windows"
	coretesting "github.com/juju/juju/testing"
)

var (
	_ = gc.Suite(&ServicePasswordChangerSuite{})
	_ = gc.Suite(&EnsurePasswordSuite{})
)

type myAmazingServiceManager struct {
	windows.SvcManager
	svcNames []string
	pwd      string
}

func (mgr *myAmazingServiceManager) ChangeServicePassword(name, newPassword string) error {
	mgr.svcNames = append(mgr.svcNames, name)
	mgr.pwd = newPassword
	if name == "failme" {
		return errors.New("wubwub")
	}
	return nil
}

type ServicePasswordChangerSuite struct {
	coretesting.BaseSuite
	c   *windows.PasswordChanger
	mgr *myAmazingServiceManager
}

func (s *ServicePasswordChangerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.c = &windows.PasswordChanger{}
	s.mgr = &myAmazingServiceManager{}
}

func listServices() ([]string, error) {
	return []string{"boom", "pow"}, nil
}

func listServicesFailingService() ([]string, error) {
	return []string{"boom", "failme", "pow"}, nil
}

func brokenListServices() ([]string, error) {
	return nil, errors.New("ludicrous")
}

func (s *ServicePasswordChangerSuite) TestChangeServicePasswordListSucceeds(c *gc.C) {
	err := s.c.ChangeJujudServicesPassword("newPass", s.mgr, listServices)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mgr.svcNames, gc.DeepEquals, []string{"boom", "pow"})
	c.Assert(s.mgr.pwd, gc.Equals, "newPass")
}

func (s *ServicePasswordChangerSuite) TestChangeServicePasswordListFails(c *gc.C) {
	err := s.c.ChangeJujudServicesPassword("newPass", s.mgr, brokenListServices)
	c.Assert(err, gc.ErrorMatches, "ludicrous")
}

func (s *ServicePasswordChangerSuite) TestChangePasswordFails(c *gc.C) {
	err := s.c.ChangeJujudServicesPassword("newPass", s.mgr, listServicesFailingService)
	c.Assert(err, gc.ErrorMatches, "wubwub")
	c.Assert(s.mgr.svcNames, gc.DeepEquals, []string{"boom", "failme"})
	c.Assert(s.mgr.pwd, gc.Equals, "newPass")
}

type helpersStub struct {
	failLocalhost   bool
	failServices    bool
	localhostCalled bool
	serviceCalled   bool
}

func (s *helpersStub) reset() {
	s.localhostCalled = false
	s.serviceCalled = false
}

func (s *helpersStub) ChangeUserPasswordLocalhost(newPassword string) error {
	s.localhostCalled = true
	if s.failLocalhost {
		return errors.New("zzz")
	}
	return nil
}

func (s *helpersStub) ChangeJujudServicesPassword(newPassword string, mgr windows.ServiceManager, listServices func() ([]string, error)) error {
	s.serviceCalled = true
	if s.failServices {
		return errors.New("splat")
	}
	return nil
}

type EnsurePasswordSuite struct {
	coretesting.BaseSuite
	username    string
	newPassword string
}

func (s *EnsurePasswordSuite) SetUpSuite(c *gc.C) {
	s.username = "jujud"
	s.newPassword = "pass"
}

func (s *EnsurePasswordSuite) TestBothCalledAndSucceed(c *gc.C) {
	stub := helpersStub{}
	err := windows.EnsureJujudPasswordHelper(s.username, s.newPassword, nil, &stub)
	c.Assert(stub.localhostCalled, jc.IsTrue)
	c.Assert(stub.serviceCalled, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnsurePasswordSuite) TestChangePasswordFails(c *gc.C) {
	stub := helpersStub{failLocalhost: true}
	err := windows.EnsureJujudPasswordHelper(s.username, s.newPassword, nil, &stub)
	c.Assert(err, gc.ErrorMatches, "could not change user password: zzz")
	c.Assert(errors.Cause(err), gc.ErrorMatches, "zzz")
	c.Assert(stub.localhostCalled, jc.IsTrue)
	c.Assert(stub.serviceCalled, jc.IsFalse)
}

func (s *EnsurePasswordSuite) TestChangeServicesFails(c *gc.C) {
	stub := helpersStub{failServices: true}
	err := windows.EnsureJujudPasswordHelper(s.username, s.newPassword, nil, &stub)
	c.Assert(err, gc.ErrorMatches, "could not change password for all jujud services: splat")
	c.Assert(errors.Cause(err), gc.ErrorMatches, "splat")
	c.Assert(stub.localhostCalled, jc.IsTrue)
	c.Assert(stub.serviceCalled, jc.IsTrue)
}
