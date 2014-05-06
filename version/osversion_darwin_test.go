// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build darwin

package version

import (
	"fmt"

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
	// Test that getSysctlVersion returns something that looks like a dotted revision number
	releaseVersion, err := getSysctlVersion()
	c.Assert(err, gc.IsNil)
	c.Check(releaseVersion, gc.Matches, `\d+\..*`)
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
