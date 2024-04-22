// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
)

type macOSXSeriesSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&macOSXSeriesSuite{})

func (*macOSXSeriesSuite) TestGetSysctlVersionPlatform(c *gc.C) {
	// Test that sysctlVersion returns something that looks like a dotted revision number
	releaseVersion, err := sysctlVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(releaseVersion, gc.Matches, `\d+\..*`)
}

func (s *macOSXSeriesSuite) TestOSVersion(c *gc.C) {
	s.PatchValue(&sysctlVersion, func() (string, error) { return "23.1.0", nil })
	b, err := readBase()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(b, gc.Equals, corebase.MustParseBaseFromString("osx@23.1.0"))
}
