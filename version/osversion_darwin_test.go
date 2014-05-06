// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build darwin

package version

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type darwinVersionSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&darwinVersionSuite{})

func (*darwinVersionSuite) TestGetSysctlVersionPlatform(c *gc.C) {
	// Test that getSysctlVersion returns something that looks like a dotted revision number
	releaseVersion, err := getSysctlVersion()
	c.Assert(err, gc.IsNil)
	c.Check(releaseVersion, gc.Matches, `\d+\..*`)
}

func (s *darwinVersionSuite) TestOSVersion(c *gc.C) {
	c.Check(osVersion(), gc.Matches, `darwin\d+`)
}
