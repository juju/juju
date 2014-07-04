// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type supportedSeriesSuite struct {
	testing.BaseSuite
	cleanup func()
}

var _ = gc.Suite(&supportedSeriesSuite{})

func (s *supportedSeriesSuite) SetUpTest(c *gc.C) {
	s.cleanup = version.SetSeriesVersions(make(map[string]string))
}

func (s *supportedSeriesSuite) TearDownTest(c *gc.C) {
	s.cleanup()
}

var getOSFromSeriesTests = []struct {
	series string
	want   version.OSType
	err    string
}{{
	series: "precise",
	want:   version.Ubuntu,
}, {
	series: "win2012r2",
	want:   version.Windows,
}, {
	// GetOSFromSeries only supports Ubuntu and Windows.
	series: "mountainlion",
	err:    `invalid series "mountainlion"`,
}}

func (s *supportedSeriesSuite) TestGetOSFromSeries(c *gc.C) {
	for _, t := range getOSFromSeriesTests {
		got, err := version.GetOSFromSeries(t.series)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Check(err, gc.IsNil)
			c.Assert(got, gc.Equals, t.want)
		}
	}
}
