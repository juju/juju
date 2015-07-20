// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package password

import (
	"fmt"
	"regexp"

	// https://bugs.launchpad.net/juju-core/+bug/1470820
	"github.com/gabriel-samfira/sys/windows/registry"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
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
	c   *passwordChanger
	mgr *myAmazingServiceManager
}

func (s *ServicePasswordChangerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.c = &passwordChanger{}
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
	err := s.c.changeJujudServicesPassword("newPass", s.mgr, listServices)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mgr.svcNames, gc.DeepEquals, []string{"boom", "pow"})
	c.Assert(s.mgr.pwd, gc.Equals, "newPass")
}

func (s *ServicePasswordChangerSuite) TestChangeServicePasswordListFails(c *gc.C) {
	err := s.c.changeJujudServicesPassword("newPass", s.mgr, brokenListServices)
	c.Assert(err, gc.ErrorMatches, "ludicrous")
}

func (s *ServicePasswordChangerSuite) TestChangePasswordFails(c *gc.C) {
	err := s.c.changeJujudServicesPassword("newPass", s.mgr, listServicesFailingService)
	c.Assert(err, gc.ErrorMatches, "wubwub")
	c.Assert(s.mgr.svcNames, gc.DeepEquals, []string{"boom", "failme"})
	c.Assert(s.mgr.pwd, gc.Equals, "newPass")
}

type helpersStub struct {
	k               registry.Key
	deleteRegEntry  bool
	regEntry        string
	failLocalhost   bool
	failServices    bool
	localhostCalled bool
	serviceCalled   bool
}

func (s *helpersStub) reset() {
	s.localhostCalled = false
	s.serviceCalled = false
}

func (s *helpersStub) changeUserPasswordLocalhost(newPassword string) error {
	s.localhostCalled = true
	if s.deleteRegEntry {
		s.k.DeleteValue(s.regEntry)
	}
	if s.failLocalhost {
		return errors.New("zzz")
	}
	return nil
}

func (s *helpersStub) changeJujudServicesPassword(newPassword string, mgr windows.ServiceManager, listServices func() ([]string, error)) error {
	s.serviceCalled = true
	if s.deleteRegEntry {
		s.k.DeleteValue(s.regEntry)
	}
	if s.failServices {
		return errors.New("splat")
	}
	return nil
}

type EnsurePasswordSuite struct {
	coretesting.BaseSuite
	tempRegKey   string
	username     string
	newPassword  string
	tempRegEntry string
	k            registry.Key
}

func (s *EnsurePasswordSuite) SetUpSuite(c *gc.C) {
	s.username = "jujud"
	s.newPassword = "pass"
	s.tempRegKey = `HKLM:\SOFTWARE\juju-1348930201394`
	s.tempRegEntry = `password`
}

func (s *EnsurePasswordSuite) TestKeyNonexistent(c *gc.C) {
	stub := helpersStub{}
	err := ensureJujudPasswordHelper(s.username, s.newPassword, s.tempRegKey, s.tempRegEntry, nil, &stub)
	c.Assert(err, gc.ErrorMatches, "failed to open juju registry key: .*")
	c.Assert(errors.Cause(err), gc.ErrorMatches, utils.NoSuchFileErrRegexp)
}

func (s *EnsurePasswordSuite) setUpKey(c *gc.C) {
	k, exist, err := registry.CreateKey(registry.LOCAL_MACHINE, s.tempRegKey[6:], registry.ALL_ACCESS)
	// If we get in here it means cleanup failed at some point earlier
	if exist {
		err = registry.DeleteKey(registry.LOCAL_MACHINE, s.tempRegKey[6:])
		k, exist, err = registry.CreateKey(registry.LOCAL_MACHINE, s.tempRegKey[6:], registry.ALL_ACCESS)
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exist, jc.IsFalse)
	s.k = k
}

func (s *EnsurePasswordSuite) tearDownKey(c *gc.C, shouldBeDeleted bool) {
	err := s.k.DeleteValue(s.tempRegEntry)
	if shouldBeDeleted {
		c.Assert(err, gc.ErrorMatches, utils.NoSuchFileErrRegexp)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
	s.k.Close()
	err = registry.DeleteKey(registry.LOCAL_MACHINE, s.tempRegKey[6:])
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnsurePasswordSuite) TestKeyExists(c *gc.C) {
	s.setUpKey(c)
	defer s.tearDownKey(c, false)
	stub := helpersStub{}
	err := ensureJujudPasswordHelper(s.username, s.newPassword, s.tempRegKey, s.tempRegEntry, nil, &stub)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnsurePasswordSuite) TestKeyExistsValueWritten(c *gc.C) {
	s.setUpKey(c)
	defer s.tearDownKey(c, false)
	stub := helpersStub{}
	err := ensureJujudPasswordHelper(s.username, s.newPassword, s.tempRegKey, s.tempRegEntry, nil, &stub)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stub.localhostCalled, jc.IsTrue)
	c.Assert(stub.serviceCalled, jc.IsTrue)

	stub.reset()

	err = ensureJujudPasswordHelper(s.username, s.newPassword, s.tempRegKey, s.tempRegEntry, nil, &stub)
	c.Assert(err, gc.Equals, ERR_REGKEY_EXIST)
	c.Assert(stub.localhostCalled, jc.IsFalse)
	c.Assert(stub.serviceCalled, jc.IsFalse)
}

func (s *EnsurePasswordSuite) TestChangeServicesFails(c *gc.C) {
	s.setUpKey(c)
	defer s.tearDownKey(c, true)
	stub := helpersStub{failServices: true}
	err := ensureJujudPasswordHelper(s.username, s.newPassword, s.tempRegKey, s.tempRegEntry, nil, &stub)
	c.Assert(err, gc.ErrorMatches, "could not change password for all jujud services: splat")
	c.Assert(errors.Cause(err), gc.ErrorMatches, "splat")
	c.Assert(stub.localhostCalled, jc.IsTrue)
	c.Assert(stub.serviceCalled, jc.IsTrue)
}

func (s *EnsurePasswordSuite) TestChangeServicesFailsDeleteFails(c *gc.C) {
	s.setUpKey(c)
	defer s.tearDownKey(c, true)
	stub := helpersStub{failServices: true, deleteRegEntry: true, regEntry: s.tempRegEntry, k: s.k}
	err := ensureJujudPasswordHelper(s.username, s.newPassword, s.tempRegKey, s.tempRegEntry, nil, &stub)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("could not change password for all jujud services; could not erase entry %s at %s: splat", regexp.QuoteMeta(s.tempRegEntry), regexp.QuoteMeta(s.tempRegKey)))
	c.Assert(errors.Cause(err), gc.ErrorMatches, "splat")
	c.Assert(stub.localhostCalled, jc.IsTrue)
	c.Assert(stub.serviceCalled, jc.IsTrue)
}
