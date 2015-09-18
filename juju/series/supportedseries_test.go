// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/os"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/series"
	"github.com/juju/juju/testing"
)

type supportedSeriesSuite struct {
	testing.BaseSuite
	cleanup func()
}

var _ = gc.Suite(&supportedSeriesSuite{})

func (s *supportedSeriesSuite) SetUpTest(c *gc.C) {
	s.cleanup = series.SetSeriesVersions(make(map[string]string))
}

func (s *supportedSeriesSuite) TearDownTest(c *gc.C) {
	s.cleanup()
}

var getOSFromSeriesTests = []struct {
	series string
	want   os.OSType
	err    string
}{{
	series: "precise",
	want:   os.Ubuntu,
}, {
	series: "win2012r2",
	want:   os.Windows,
}, {
	series: "mountainlion",
	want:   os.OSX,
}, {
	series: "centos7",
	want:   os.CentOS,
}, {
	series: "arch",
	want:   os.Arch,
}, {
	series: "",
	err:    "series \"\" not valid",
},
}

func (s *supportedSeriesSuite) TestGetOSFromSeries(c *gc.C) {
	for _, t := range getOSFromSeriesTests {
		got, err := series.GetOSFromSeries(t.series)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Assert(got, gc.Equals, t.want)
		}
	}
}

func (s *supportedSeriesSuite) TestUnknownOSFromSeries(c *gc.C) {
	_, err := series.GetOSFromSeries("Xuanhuaceratops")
	c.Assert(err, jc.Satisfies, series.IsUnknownOSForSeriesError)
	c.Assert(err, gc.ErrorMatches, `unknown OS for series: "Xuanhuaceratops"`)
}

func (s *supportedSeriesSuite) TestOSSupportedSeries(c *gc.C) {
	series.SetSeriesVersions(map[string]string{
		"trusty":  "14.04",
		"utopic":  "14.10",
		"win7":    "win7",
		"win81":   "win81",
		"centos7": "centos7",
		"arch":    "rolling",
	})
	supported := series.OSSupportedSeries(os.Ubuntu)
	c.Assert(supported, jc.SameContents, []string{"trusty", "utopic"})
	supported = series.OSSupportedSeries(os.Windows)
	c.Assert(supported, jc.SameContents, []string{"win7", "win81"})
	supported = series.OSSupportedSeries(os.CentOS)
	c.Assert(supported, jc.SameContents, []string{"centos7"})
	supported = series.OSSupportedSeries(os.Arch)
	c.Assert(supported, jc.SameContents, []string{"arch"})
}
