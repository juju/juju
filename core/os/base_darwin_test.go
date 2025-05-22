// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	stdtesting "testing"

	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/testhelpers"
)

type macOSXSeriesSuite struct {
	testhelpers.CleanupSuite
}

func TestMacOSXSeriesSuite(t *stdtesting.T) {
	tc.Run(t, &macOSXSeriesSuite{})
}

func (*macOSXSeriesSuite) TestGetSysctlVersionPlatform(c *tc.C) {
	// Test that sysctlVersion returns something that looks like a dotted revision number
	releaseVersion, err := sysctlVersion()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(releaseVersion, tc.Matches, `\d+\..*`)
}

func (s *macOSXSeriesSuite) TestOSVersion(c *tc.C) {
	s.PatchValue(&sysctlVersion, func() (string, error) { return "23.1.0", nil })
	b, err := readBase()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(b, tc.Equals, corebase.MustParseBaseFromString("osx@23.1.0"))
}
