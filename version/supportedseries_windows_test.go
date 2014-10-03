// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"sort"

	"github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/version"
)

type supportedSeriesWindowsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&supportedSeriesWindowsSuite{})

func (s *supportedSeriesWindowsSuite) TestSeriesVersion(c *gc.C) {
	vers, err := version.SeriesVersion("win8")
	if err != nil {
		c.Assert(err, gc.Not(gc.ErrorMatches), `invalid series "win8"`, gc.Commentf(`unable to lookup series "win8"`))
	} else {
		c.Assert(err, gc.IsNil)
	}
	c.Assert(err, gc.IsNil)
	c.Assert(vers, gc.Equals, "win8")
}

func (s *supportedSeriesWindowsSuite) TestSupportedSeries(c *gc.C) {
	expectedSeries := []string{"precise", "quantal", "raring", "saucy", "trusty", "utopic", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win7", "win8", "win81"}
	series := version.SupportedSeries()
	sort.Strings(series)
	c.Assert(series, gc.DeepEquals, expectedSeries)
}
