// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"sort"

	jujuos "github.com/juju/os"
	jujuseries "github.com/juju/os/series"
	"github.com/juju/testing"
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

func (s *SupportedSeriesLinuxSuite) TestDefaultSupportedLTS(c *gc.C) {
	name := DefaultSupportedLTS()
	c.Assert(name, gc.Equals, "bionic")
}

func (s *SupportedSeriesLinuxSuite) TestLatestLts(c *gc.C) {
	table := []struct {
		latest, want string
	}{
		{"testseries", "testseries"},
		{"", "bionic"},
	}
	for _, test := range table {
		SetLatestLtsForTesting(test.latest)
		got := LatestLts()
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

func (s *SupportedSeriesLinuxSuite) TestSupportedSeries(c *gc.C) {
	series := SupportedSeries()
	sort.Strings(series)
	c.Assert(series, gc.DeepEquals, []string{
		"artful", "bionic", "catalina", "centos7", "centos8", "cosmic", "disco", "elcapitan", "eoan", "focal",
		"genericlinux", "groovy", "hairy", "highsierra", "hirsute", "jaguar", "kubernetes", "leopard", "lion",
		"mavericks", "mojave", "mountainlion", "opensuseleap", "panther", "precise", "puma", "quantal", "raring",
		"saucy", "sierra", "snowleopard", "tiger", "trusty", "utopic", "vivid", "wily", "win10", "win2008r2", "win2012",
		"win2012hv", "win2012hvr2", "win2012r2", "win2016", "win2016hv", "win2016nano", "win2019", "win7", "win8", "win81",
		"xenial", "yakkety", "yosemite", "zesty"})
}
