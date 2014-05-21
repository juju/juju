// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ubuntu_test

import (
	"sort"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version/ubuntu"
)

type simplestreamsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&simplestreamsSuite{})

func (s *simplestreamsSuite) TestSeriesVersion(c *gc.C) {
	cleanup := ubuntu.SetSeriesVersions(make(map[string]string))
	defer cleanup()
	vers, err := ubuntu.SeriesVersion("precise")
	if err != nil && err.Error() == `invalid series "precise"` {
		c.Fatalf(`Unable to lookup series "precise", you may need to: apt-get install distro-info`)
	}
	c.Assert(err, gc.IsNil)
	c.Assert(vers, gc.Equals, "12.04")
}

func (s *simplestreamsSuite) TestSupportedSeries(c *gc.C) {
	cleanup := ubuntu.SetSeriesVersions(make(map[string]string))
	defer cleanup()
	series := ubuntu.SupportedSeries()
	sort.Strings(series)
	c.Assert(series, gc.DeepEquals, []string{"precise", "quantal", "raring", "saucy", "trusty", "utopic"})
}
