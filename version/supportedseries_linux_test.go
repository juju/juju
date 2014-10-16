// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"sort"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/version"
)

func (s *supportedSeriesSuite) TestSeriesVersion(c *gc.C) {
	vers, err := version.SeriesVersion("precise")
	if err != nil && err.Error() == `invalid series "precise"` {
		c.Fatalf(`Unable to lookup series "precise", you may need to: apt-get install distro-info`)
	}
	c.Assert(err, gc.IsNil)
	c.Assert(vers, gc.Equals, "12.04")
}

func (s *supportedSeriesSuite) TestSupportedSeries(c *gc.C) {
	expectedSeries := []string{"precise", "quantal", "raring", "saucy", "trusty", "utopic"}
	series := version.SupportedSeries()
	sort.Strings(series)
	c.Assert(series, gc.DeepEquals, expectedSeries)
}
