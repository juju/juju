// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build darwin

package version

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/set"
)

type macOSXVersionSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&macOSXVersionSuite{})

func (*macOSXVersionSuite) TestGetSysctlVersionPlatform(c *gc.C) {
	// Test that getSysctlVersion returns something that looks like a dotted revision number
	releaseVersion, err := getSysctlVersion()
	c.Assert(err, gc.IsNil)
	c.Check(releaseVersion, gc.Matches, `\d+\..*`)
}

func (s *macOSXVersionSuite) TestOSVersion(c *gc.C) {
	knownSeries := set.Strings{}
	for _, series := range macOSXSeries {
		knownSeries.Add(series)
	}
	c.Check(osVersion(), jc.Satisfies, knownSeries.Contains)
}
