// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	series: "mountainlion",
	want:   version.OSX,
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

func (s *supportedSeriesSuite) TestOSSupportedSeries(c *gc.C) {
	version.SetSeriesVersions(map[string]string{
		"trusty": "14.04",
		"utopic": "14.10",
		"win7":   "win7",
		"win81":  "win81",
	})
	series := version.OSSupportedSeries(version.Ubuntu)
	c.Assert(series, jc.SameContents, []string{"trusty", "utopic"})
	series = version.OSSupportedSeries(version.Windows)
	c.Assert(series, jc.SameContents, []string{"win7", "win81"})
}
