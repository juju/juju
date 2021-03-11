// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"io/ioutil"
	"os"
	"sort"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreos "github.com/juju/juju/core/os"
)

const distroInfoContents = `version,codename,series,created,release,eol,eol-server
10.04,Firefox,firefox,2009-10-13,2010-04-26,2016-04-26
12.04 LTS,Precise Pangolin,precise,2011-10-13,2012-04-26,2017-04-26
99.04,Focal,focal,2020-04-25,2020-10-17,2365-07-17
`

type SupportedSeriesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SupportedSeriesSuite{})

func (s *SupportedSeriesSuite) TestSeriesForTypes(c *gc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	now := time.Date(2020, 3, 16, 0, 0, 0, 0, time.UTC)

	info, err := seriesForTypes(tmpFile.Name(), now, "", "")
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.ControllerSeries()
	sort.Strings(ctrlSeries)
	c.Assert(ctrlSeries, gc.DeepEquals, []string{"bionic", "trusty", "xenial"})

	wrkSeries := info.WorkloadSeries()
	sort.Strings(wrkSeries)
	c.Assert(wrkSeries, gc.DeepEquals, []string{"bionic", "centos7", "centos8", "genericlinux", "kubernetes", "opensuseleap", "trusty", "win10", "win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2016", "win2016hv", "win2016nano", "win2019", "win7", "win8", "win81", "xenial"})
}

func (s *SupportedSeriesSuite) TestSeriesForTypesUsingImageStream(c *gc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	now := time.Date(2020, 3, 16, 0, 0, 0, 0, time.UTC)

	info, err := seriesForTypes(tmpFile.Name(), now, "focal", "daily")
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.ControllerSeries()
	sort.Strings(ctrlSeries)
	c.Assert(ctrlSeries, gc.DeepEquals, []string{"bionic", "focal", "trusty", "xenial"})

	wrkSeries := info.WorkloadSeries()
	sort.Strings(wrkSeries)
	c.Assert(wrkSeries, gc.DeepEquals, []string{"bionic", "centos7", "centos8", "focal", "genericlinux", "kubernetes", "opensuseleap", "trusty", "win10", "win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2016", "win2016hv", "win2016nano", "win2019", "win7", "win8", "win81", "xenial"})
}

func (s *SupportedSeriesSuite) TestSeriesForTypesUsingInvalidImageStream(c *gc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	now := time.Date(2020, 3, 16, 0, 0, 0, 0, time.UTC)

	info, err := seriesForTypes(tmpFile.Name(), now, "focal", "turtle")
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.ControllerSeries()
	sort.Strings(ctrlSeries)
	c.Assert(ctrlSeries, gc.DeepEquals, []string{"bionic", "trusty", "xenial"})

	wrkSeries := info.WorkloadSeries()
	sort.Strings(wrkSeries)
	c.Assert(wrkSeries, gc.DeepEquals, []string{"bionic", "centos7", "centos8", "genericlinux", "kubernetes", "opensuseleap", "trusty", "win10", "win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2016", "win2016hv", "win2016nano", "win2019", "win7", "win8", "win81", "xenial"})
}

func (s *SupportedSeriesSuite) TestSeriesForTypesUsingInvalidSeries(c *gc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	now := time.Date(2020, 3, 16, 0, 0, 0, 0, time.UTC)

	info, err := seriesForTypes(tmpFile.Name(), now, "firewolf", "daily")
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.ControllerSeries()
	sort.Strings(ctrlSeries)
	c.Assert(ctrlSeries, gc.DeepEquals, []string{"bionic", "trusty", "xenial"})

	wrkSeries := info.WorkloadSeries()
	sort.Strings(wrkSeries)
	c.Assert(wrkSeries, gc.DeepEquals, []string{"bionic", "centos7", "centos8", "genericlinux", "kubernetes", "opensuseleap", "trusty", "win10", "win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2016", "win2016hv", "win2016nano", "win2019", "win7", "win8", "win81", "xenial"})
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

func setSeriesTestData() func() {
	value := map[SeriesName]seriesVersion{
		"trusty":       {Version: "14.04"},
		"utopic":       {Version: "14.10"},
		"win7":         {Version: "win7"},
		"win81":        {Version: "win81"},
		"win2016nano":  {Version: "1win2016nano"},
		"centos7":      {Version: "centos7"},
		"opensuseleap": {Version: "opensuseleap"},
		"genericlinux": {Version: "genericlinux"},
	}

	origVersions := allSeriesVersions
	origUpdated := updatedseriesVersions
	allSeriesVersions = value
	updateVersionSeries()
	updatedseriesVersions = len(value) != 0
	return func() {
		allSeriesVersions = origVersions
		updateVersionSeries()
		updatedseriesVersions = origUpdated
	}
}

func (s *SupportedSeriesSuite) TestOSSupportedSeries(c *gc.C) {
	reset := setSeriesTestData()
	defer reset()
	supported := OSSupportedSeries(coreos.Ubuntu)
	c.Assert(supported, jc.SameContents, []string{"trusty", "utopic"})
	supported = OSSupportedSeries(coreos.Windows)
	c.Assert(supported, jc.SameContents, []string{"win7", "win81", "win2016nano"})
	supported = OSSupportedSeries(coreos.CentOS)
	c.Assert(supported, jc.SameContents, []string{"centos7"})
	supported = OSSupportedSeries(coreos.OpenSUSE)
	c.Assert(supported, jc.SameContents, []string{"opensuseleap"})
	supported = OSSupportedSeries(coreos.GenericLinux)
	c.Assert(supported, jc.SameContents, []string{"genericlinux"})
}

func (s *SupportedSeriesSuite) TestVersionSeriesValid(c *gc.C) {
	reset := setSeriesTestData()
	defer reset()
	seriesResult, err := VersionSeries("14.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert("trusty", gc.DeepEquals, seriesResult)
}

func (s *SupportedSeriesSuite) TestVersionSeriesEmpty(c *gc.C) {
	reset := setSeriesTestData()
	defer reset()
	_, err := VersionSeries("")
	c.Assert(err, gc.ErrorMatches, `.*unknown series for version: "".*`)
}

func (s *SupportedSeriesSuite) TestVersionSeriesInvalid(c *gc.C) {
	reset := setSeriesTestData()
	defer reset()
	_, err := VersionSeries("73655")
	c.Assert(err, gc.ErrorMatches, `.*unknown series for version: "73655".*`)
}

func (s *SupportedSeriesSuite) TestSeriesVersionEmpty(c *gc.C) {
	reset := setSeriesTestData()
	defer reset()
	_, err := SeriesVersion("")
	c.Assert(err, gc.ErrorMatches, `.*unknown version for series: "".*`)
}

func makeTempFile(c *gc.C, content string) (*os.File, func()) {
	tmpfile, err := ioutil.TempFile("", "distroinfo")
	if err != nil {
		c.Assert(err, jc.ErrorIsNil)
	}

	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, jc.ErrorIsNil)

	// Reset the file for reading.
	_, err = tmpfile.Seek(0, 0)
	c.Assert(err, jc.ErrorIsNil)

	return tmpfile, func() {
		err := os.Remove(tmpfile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}
}
