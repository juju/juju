// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"sort"

	"github.com/juju/juju/version"
	gc "launchpad.net/gocheck"
)

func (s *simplestreamsSuite) TestSeriesVersion(c *gc.C) {
	vers, err := version.SeriesVersion("precise")
	if err != nil && err.Error() == `invalid series "precise"` {
		c.Fatalf(`Unable to lookup series "precise", you may need to: apt-get install distro-info`)
	}
	c.Assert(err, gc.IsNil)
	c.Assert(vers, gc.Equals, "12.04")
}

func (s *simplestreamsSuite) TestSupportedSeries(c *gc.C) {
	series := version.SupportedSeries()
	sort.Strings(series)
	c.Assert(series, gc.DeepEquals, []string{"precise", "quantal", "raring", "saucy", "trusty", "utopic"})
}
