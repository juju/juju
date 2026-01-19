// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreos "github.com/juju/juju/core/os"
)

type SupportedSeriesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SupportedSeriesSuite{})

func (s *SupportedSeriesSuite) TestSeriesForTypes(c *gc.C) {
	info, err := seriesForTypes("", "")
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := controllerSeries(info)
	c.Assert(ctrlSeries, jc.DeepEquals, []string{"noble", "jammy", "focal", "bionic", "xenial", "trusty"})

	wrkSeries := workloadSeries(info, false)
	c.Assert(wrkSeries, jc.DeepEquals, []string{"noble", "jammy", "focal", "bionic", "xenial", "trusty", "centos7", "centos8", "centos9", "genericlinux", "kubernetes", "opensuseleap", "win10", "win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2016", "win2016hv", "win2016nano", "win2019", "win7", "win8", "win81"})
}

func (s *SupportedSeriesSuite) TestSeriesForTypesUsingImageStream(c *gc.C) {
	info, err := seriesForTypes("focal", "daily")
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := controllerSeries(info)
	c.Assert(ctrlSeries, jc.DeepEquals, []string{"noble", "jammy", "focal", "bionic", "xenial", "trusty"})

	wrkSeries := workloadSeries(info, false)
	c.Assert(wrkSeries, jc.DeepEquals, []string{"noble", "jammy", "focal", "bionic", "xenial", "trusty", "centos7", "centos8", "centos9", "genericlinux", "kubernetes", "opensuseleap", "win10", "win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2016", "win2016hv", "win2016nano", "win2019", "win7", "win8", "win81"})
}

func (s *SupportedSeriesSuite) TestSeriesForTypesUsingInvalidImageStream(c *gc.C) {
	info, err := seriesForTypes("focal", "turtle")
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := controllerSeries(info)
	c.Assert(ctrlSeries, jc.DeepEquals, []string{"noble", "jammy", "focal", "bionic", "xenial", "trusty"})

	wrkSeries := workloadSeries(info, false)
	c.Assert(wrkSeries, jc.DeepEquals, []string{"noble", "jammy", "focal", "bionic", "xenial", "trusty", "centos7", "centos8", "centos9", "genericlinux", "kubernetes", "opensuseleap", "win10", "win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2016", "win2016hv", "win2016nano", "win2019", "win7", "win8", "win81"})
}

func (s *SupportedSeriesSuite) TestSeriesForTypesUsingInvalidSeries(c *gc.C) {
	info, err := seriesForTypes("firewolf", "daily")
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := controllerSeries(info)
	c.Assert(ctrlSeries, jc.DeepEquals, []string{"noble", "jammy", "focal", "bionic", "xenial", "trusty"})

	wrkSeries := workloadSeries(info, false)
	c.Assert(wrkSeries, jc.DeepEquals, []string{"noble", "jammy", "focal", "bionic", "xenial", "trusty", "centos7", "centos8", "centos9", "genericlinux", "kubernetes", "opensuseleap", "win10", "win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2016", "win2016hv", "win2016nano", "win2019", "win7", "win8", "win81"})
}

var getOSFromSeriesTests = []struct {
	series string
	want   coreos.OSType
	err    string
}{{
	series: "precise",
	want:   coreos.Ubuntu,
}, {
	series: "win2012r2",
	want:   coreos.Windows,
}, {
	series: "win2016nano",
	want:   coreos.Windows,
}, {
	series: "mountainlion",
	want:   coreos.OSX,
}, {
	series: "centos7",
	want:   coreos.CentOS,
}, {
	series: "opensuseleap",
	want:   coreos.OpenSUSE,
}, {
	series: "kubernetes",
	want:   coreos.Kubernetes,
}, {
	series: "genericlinux",
	want:   coreos.GenericLinux,
}, {
	series: "",
	err:    "series \"\" not valid",
},
}

func (s *SupportedSeriesSuite) TestGetOSFromSeries(c *gc.C) {
	for _, t := range getOSFromSeriesTests {
		got, err := GetOSFromSeries(t.series)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Assert(got, gc.Equals, t.want)
		}
	}
}

func (s *SupportedSeriesSuite) TestUnknownOSFromSeries(c *gc.C) {
	_, err := GetOSFromSeries("Xuanhuaceratops")
	c.Assert(err, jc.Satisfies, IsUnknownOSForSeriesError)
	c.Assert(err, gc.ErrorMatches, `unknown OS for series: "Xuanhuaceratops"`)
}

func (s *SupportedSeriesSuite) TestVersionSeriesValid(c *gc.C) {
	seriesResult, err := VersionSeries("14.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert("trusty", gc.DeepEquals, seriesResult)
}

func (s *SupportedSeriesSuite) TestVersionSeriesEmpty(c *gc.C) {
	_, err := VersionSeries("")
	c.Assert(err, gc.ErrorMatches, `.*unknown series for version: "".*`)
}

func (s *SupportedSeriesSuite) TestVersionSeriesInvalid(c *gc.C) {
	_, err := VersionSeries("73655")
	c.Assert(err, gc.ErrorMatches, `.*unknown series for version: "73655".*`)
}

func (s *SupportedSeriesSuite) TestSeriesVersionEmpty(c *gc.C) {
	_, err := SeriesVersion("")
	c.Assert(err, gc.ErrorMatches, `.*unknown version for series: "".*`)
}

func (s *SupportedSeriesSuite) TestGetOSVersionFromSeries(c *gc.C) {
	vers, err := GetBaseFromSeries("jammy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers, jc.DeepEquals, MakeDefaultBase("ubuntu", "22.04"))
	_, err = GetBaseFromSeries("unknown")
	c.Assert(err, gc.ErrorMatches, `series "unknown" not valid`)
	vers, err = GetBaseFromSeries("centos7")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers, jc.DeepEquals, MakeDefaultBase("centos", "centos7"))
}

func (s *SupportedSeriesSuite) TestGetSeriesFromOSVersion(c *gc.C) {
	series, err := GetSeriesFromChannel("ubuntu", "22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "jammy")
	_, err = GetSeriesFromChannel("bad", "22.04")
	c.Assert(err, gc.ErrorMatches, `os "bad" version "22.04" not found`)
	series, err = GetSeriesFromChannel("centos", "centos7")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "centos7")
}

func (s *SupportedSeriesSuite) TestUbuntuSeriesVersionEmpty(c *gc.C) {
	_, err := UbuntuSeriesVersion("")
	c.Assert(err, gc.ErrorMatches, `.*unknown version for series: "".*`)
}

func (s *SupportedSeriesSuite) TestUbuntuSeriesVersion(c *gc.C) {
	isUbuntuTests := []struct {
		series   string
		expected string
	}{
		{"precise", "12.04"},
		{"raring", "13.04"},
		{"bionic", "18.04"},
		{"eoan", "19.10"},
		{"focal", "20.04"},
		{"jammy", "22.04"},
	}
	for _, v := range isUbuntuTests {
		ver, err := UbuntuSeriesVersion(v.series)
		c.Assert(err, gc.IsNil)
		c.Assert(ver, gc.Equals, v.expected)
	}
}

func (s *SupportedSeriesSuite) TestUbuntuInvalidSeriesVersion(c *gc.C) {
	_, err := UbuntuSeriesVersion("firewolf")
	c.Assert(err, gc.ErrorMatches, `.*unknown version for series: "firewolf".*`)
}

func (s *SupportedSeriesSuite) TestWorkloadSeries(c *gc.C) {
	series, err := WorkloadSeries("", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series.SortedValues(), gc.DeepEquals, []string{
		"bionic", "centos7", "centos8", "centos9", "focal", "genericlinux", "jammy", "kubernetes",
		"opensuseleap", "trusty", "win10", "win2008r2", "win2012", "win2012hv",
		"win2012hvr2", "win2012r2", "win2016", "win2016hv", "win2016nano", "win2019",
		"win7", "win8", "win81", "xenial"})
}
