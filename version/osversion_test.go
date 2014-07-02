// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type readSeriesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&readSeriesSuite{})

type kernelVersionSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&kernelVersionSuite{})

var readSeriesTests = []struct {
	contents string
	series   string
}{{
	`DISTRIB_ID=Ubuntu
DISTRIB_RELEASE=12.04
DISTRIB_CODENAME=precise
DISTRIB_DESCRIPTION="Ubuntu 12.04 LTS"`,
	"precise",
}, {
	"DISTRIB_CODENAME= \tprecise\t",
	"precise",
}, {
	`DISTRIB_CODENAME="precise"`,
	"precise",
}, {
	"DISTRIB_CODENAME='precise'",
	"precise",
}, {
	`DISTRIB_ID=Ubuntu
DISTRIB_RELEASE=12.10
DISTRIB_CODENAME=quantal
DISTRIB_DESCRIPTION="Ubuntu 12.10"`,
	"quantal",
}, {
	"",
	"unknown",
},
}

func (*readSeriesSuite) TestReadSeries(c *gc.C) {
	d := c.MkDir()
	f := filepath.Join(d, "foo")
	for i, t := range readSeriesTests {
		c.Logf("test %d", i)
		err := ioutil.WriteFile(f, []byte(t.contents), 0666)
		c.Assert(err, gc.IsNil)
		series, err := version.ReadSeries(f)
		c.Assert(err, gc.IsNil)
		c.Assert(series, gc.Equals, t.series)
	}
}

func sysctlMacOS10dot9dot2() (string, error) {
	// My 10.9.2 Mac gives "13.1.0" as the kernel version
	return "13.1.0", nil
}

func sysctlError() (string, error) {
	return "", fmt.Errorf("no such syscall")
}

func (*kernelVersionSuite) TestKernelToMajorVersion(c *gc.C) {
	majorVersion, err := version.KernelToMajor(sysctlMacOS10dot9dot2)
	c.Assert(err, gc.IsNil)
	c.Check(majorVersion, gc.Equals, 13)
}

func (*kernelVersionSuite) TestKernelToMajorVersionError(c *gc.C) {
	majorVersion, err := version.KernelToMajor(sysctlError)
	c.Assert(err, gc.ErrorMatches, "no such syscall")
	c.Check(majorVersion, gc.Equals, 0)
}

func (*kernelVersionSuite) TestKernelToMajorVersionNoDots(c *gc.C) {
	majorVersion, err := version.KernelToMajor(func() (string, error) {
		return "1234", nil
	})
	c.Assert(err, gc.IsNil)
	c.Check(majorVersion, gc.Equals, 1234)
}

func (*kernelVersionSuite) TestKernelToMajorVersionNotInt(c *gc.C) {
	majorVersion, err := version.KernelToMajor(func() (string, error) {
		return "a.b.c", nil
	})
	c.Assert(err, gc.ErrorMatches, `strconv.ParseInt: parsing "a": invalid syntax`)
	c.Check(majorVersion, gc.Equals, 0)
}

func (*kernelVersionSuite) TestKernelToMajorVersionEmpty(c *gc.C) {
	majorVersion, err := version.KernelToMajor(func() (string, error) {
		return "", nil
	})
	c.Assert(err, gc.ErrorMatches, `strconv.ParseInt: parsing "": invalid syntax`)
	c.Check(majorVersion, gc.Equals, 0)
}

func (*kernelVersionSuite) TestMacOSXSeriesFromKernelVersion(c *gc.C) {
	series, err := version.MacOSXSeriesFromKernelVersion(sysctlMacOS10dot9dot2)
	c.Assert(err, gc.IsNil)
	c.Check(series, gc.Equals, "mavericks")
}

func (*kernelVersionSuite) TestMacOSXSeriesFromKernelVersionError(c *gc.C) {
	// We suppress the actual error in favor of returning "unknown", but we
	// do log the error
	series, err := version.MacOSXSeriesFromKernelVersion(sysctlError)
	c.Assert(err, gc.ErrorMatches, "no such syscall")
	c.Assert(series, gc.Equals, "unknown")
	c.Check(c.GetTestLog(), gc.Matches, ".* juju.version unable to determine OS version: no such syscall\n")
}

func (*kernelVersionSuite) TestMacOSXSeries(c *gc.C) {
	tests := []struct {
		version int
		series  string
		err     string
	}{
		{version: 13, series: "mavericks"},
		{version: 12, series: "mountainlion"},
		{version: 14, series: "unknown", err: `unknown series ""`},
		{version: 4, series: "unknown", err: `unknown series ""`},
		{version: 0, series: "unknown", err: `unknown series ""`},
	}
	for _, test := range tests {
		series, err := version.MacOSXSeriesFromMajorVersion(test.version)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, gc.IsNil)
		}
		c.Check(series, gc.Equals, test.series)
	}
}
