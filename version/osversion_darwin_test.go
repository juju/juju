// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build darwin

package version

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type darwinVersionSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&darwinVersionSuite{})

func sysctlMacOS10dot9dot2() (string, error) {
	// My 10.9.2 Mac gives "13.1.0" as the kernel version
	return "13.1.0", nil
}

func sysctlError() (string, error) {
	return "", fmt.Errorf("no such syscall")
}

func (*darwinVersionSuite) TestGetSysctlVersionPlatform(c *gc.C) {
	releaseVersion, err := getSysctlVersion()
	c.Assert(err, gc.IsNil)
	c.Check(releaseVersion, gc.Matches, `\d+\..*`)
}

func (*darwinVersionSuite) TestGetMajorVersionPlatform(c *gc.C) {
	// Test that we actually get a value on this platform
	majorVersion, err := getMajorVersion()
	c.Assert(err, gc.IsNil)
	c.Check(majorVersion, jc.GreaterThan, 0)
	c.Check(majorVersion, jc.LessThan, 100)
}

func (s *darwinVersionSuite) TestGetMajorVersion(c *gc.C) {
	s.PatchValue(&getSysctlVersion, sysctlMacOS10dot9dot2)
	majorVersion, err := getMajorVersion()
	c.Assert(err, gc.IsNil)
	c.Check(majorVersion, gc.Equals, 13)
}

func (s *darwinVersionSuite) TestGetMajorVersionError(c *gc.C) {
	s.PatchValue(&getSysctlVersion, sysctlError)
	majorVersion, err := getMajorVersion()
	c.Assert(err, gc.ErrorMatches, "no such syscall")
	c.Check(majorVersion, gc.Equals, 0)
}

func (s *darwinVersionSuite) TestGetMajorVersionNoDots(c *gc.C) {
	s.PatchValue(&getSysctlVersion, func() (string, error) {
		return "1234", nil
	})
	majorVersion, err := getMajorVersion()
	c.Assert(err, gc.IsNil)
	c.Check(majorVersion, gc.Equals, 1234)
}

func (s *darwinVersionSuite) TestGetMajorVersionNotInt(c *gc.C) {
	s.PatchValue(&getSysctlVersion, func() (string, error) {
		return "a.b.c", nil
	})
	majorVersion, err := getMajorVersion()
	c.Assert(err, gc.ErrorMatches, `strconv.ParseInt: parsing "a": invalid syntax`)
	c.Check(majorVersion, gc.Equals, 0)
}

func (s *darwinVersionSuite) TestGetMajorVersionEmpty(c *gc.C) {
	s.PatchValue(&getSysctlVersion, func() (string, error) {
		return "", nil
	})
	majorVersion, err := getMajorVersion()
	c.Assert(err, gc.ErrorMatches, `strconv.ParseInt: parsing "": invalid syntax`)
	c.Check(majorVersion, gc.Equals, 0)
}

func (s *darwinVersionSuite) TestGetOSVersion(c *gc.C) {
	s.PatchValue(&getSysctlVersion, sysctlMacOS10dot9dot2)
	c.Check(osVersion(), gc.Equals, "darwin13")
}

func (s *darwinVersionSuite) TestGetOSVersionError(c *gc.C) {
	// We suppress the actual error in favor of returning "unknown", but we
	// do at least log the error
	s.PatchValue(&getSysctlVersion, sysctlError)
	c.Check(osVersion(), gc.Equals, "unknown")
	c.Check(c.GetTestLog(), gc.Matches, ".* juju.version unable to determine OS version: no such syscall\n")
}
