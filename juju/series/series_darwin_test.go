// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
)

type macOSXSeriesSuite struct{}

var _ = gc.Suite(&macOSXSeriesSuite{})

func (*macOSXSeriesSuite) TestGetSysctlVersionPlatform(c *gc.C) {
	// Test that sysctlVersion returns something that looks like a dotted revision number
	releaseVersion, err := sysctlVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(releaseVersion, gc.Matches, `\d+\..*`)
}

func (s *macOSXSeriesSuite) TestOSVersion(c *gc.C) {
	knownSeries := make(set.Strings)
	for _, series := range macOSXSeries {
		knownSeries.Add(series)
	}
	version, err := readSeries()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, jc.Satisfies, knownSeries.Contains)
}
