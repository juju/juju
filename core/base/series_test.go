// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/os/ostype"
)

type SeriesSuite struct {
	testing.IsolationSuite
}

func (s *SeriesSuite) TestGetBaseFromSeries(c *gc.C) {
	vers, err := GetBaseFromSeries("jammy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers, jc.DeepEquals, MakeDefaultBase("ubuntu", "22.04"))
	_, err = GetBaseFromSeries("unknown")
	c.Assert(err, gc.ErrorMatches, `series "unknown" not valid`)
	vers, err = GetBaseFromSeries("centos7")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers, jc.DeepEquals, MakeDefaultBase("centos", "7"))
}

func (s *SeriesSuite) TestGetSeriesFromBase(c *gc.C) {
	series, err := GetSeriesFromBase(MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "jammy")
}

var getOSFromSeriesTests = []struct {
	series string
	want   ostype.OSType
	err    string
}{{
	series: "precise",
	want:   ostype.Ubuntu,
}, {
	series: "centos7",
	want:   ostype.CentOS,
}, {
	series: "genericlinux",
	want:   ostype.GenericLinux,
}, {
	series: "",
	err:    "series \"\" not valid",
},
}

func (s *SeriesSuite) TestGetOSFromSeries(c *gc.C) {
	for _, t := range getOSFromSeriesTests {
		got, err := getOSFromSeries(t.series)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Assert(got, gc.Equals, t.want)
		}
	}
}

func (s *SeriesSuite) TestUnknownOSFromSeries(c *gc.C) {
	_, err := getOSFromSeries("Xuanhuaceratops")
	c.Assert(err, jc.Satisfies, IsUnknownOSForSeriesError)
	c.Assert(err, gc.ErrorMatches, `unknown OS for series: "Xuanhuaceratops"`)
}

func (s *SeriesSuite) TestSeriesVersionEmpty(c *gc.C) {
	_, err := getSeriesVersion("")
	c.Assert(err, gc.ErrorMatches, `.*unknown version for series: "".*`)
}
