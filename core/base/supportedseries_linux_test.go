// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"time"

	jujuos "github.com/juju/os/v2"
	jujuseries "github.com/juju/os/v2/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type SupportedSeriesLinuxSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SupportedSeriesLinuxSuite{})

func (s *SupportedSeriesLinuxSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchValue(&LocalSeriesVersionInfo, func() (jujuos.OSType, map[string]jujuseries.SeriesVersionInfo, error) {
		return jujuos.Ubuntu, map[string]jujuseries.SeriesVersionInfo{
			"hairy": {},
		}, nil
	})
}

func (s *SupportedSeriesLinuxSuite) TestLatestLts(c *gc.C) {
	table := []struct {
		latest, want string
	}{
		{"testseries", "testseries"},
		{"", "jammy"},
	}
	for _, test := range table {
		SetLatestLtsForTesting(test.latest)
		got := LatestLTS()
		c.Assert(got, gc.Equals, test.want)
	}
}

func (s *SupportedSeriesLinuxSuite) TestUbuntuSeriesVersionEmpty(c *gc.C) {
	_, err := UbuntuSeriesVersion("")
	c.Assert(err, gc.ErrorMatches, `.*unknown version for series: "".*`)
}

func (s *SupportedSeriesLinuxSuite) TestUbuntuSeriesVersion(c *gc.C) {
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
		{"noble", "24.04"},
	}
	for _, v := range isUbuntuTests {
		ver, err := UbuntuSeriesVersion(v.series)
		c.Assert(err, gc.IsNil)
		c.Assert(ver, gc.Equals, v.expected)
	}
}

func (s *SupportedSeriesLinuxSuite) TestUbuntuInvalidSeriesVersion(c *gc.C) {
	_, err := UbuntuSeriesVersion("firewolf")
	c.Assert(err, gc.ErrorMatches, `.*unknown version for series: "firewolf".*`)
}

func (s *SupportedSeriesLinuxSuite) TestWorkloadSeries(c *gc.C) {
	tmpFile, close := makeTempFile(c, distroInfoContents)
	defer close()

	s.PatchValue(&UbuntuDistroInfo, tmpFile.Name())

	series, err := WorkloadSeries(time.Time{}, "", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series.SortedValues(), gc.DeepEquals, []string{
		"centos7", "centos9", "focal", "genericlinux", "jammy", "noble",
	})
}
