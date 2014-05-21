// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
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
		c.Assert(version.ReadSeries(f), gc.Equals, t.series)
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
	c.Check(version.MacOSXSeriesFromKernelVersion(sysctlMacOS10dot9dot2), gc.Equals, "mavericks")
}

func (*kernelVersionSuite) TestMacOSXSeriesFromKernelVersionError(c *gc.C) {
	// We suppress the actual error in favor of returning "unknown", but we
	// do log the error
	c.Check(version.MacOSXSeriesFromKernelVersion(sysctlError), gc.Equals, "unknown")
	c.Check(c.GetTestLog(), gc.Matches, ".* juju.version unable to determine OS version: no such syscall\n")
}

func (*kernelVersionSuite) TestMacOSXSeries(c *gc.C) {
	tests := []struct {
		version int
		series  string
	}{
		{13, "mavericks"},
		{12, "mountainlion"},
		{14, "unknown"},
		{4, "unknown"},
		{0, "unknown"},
	}
	for _, test := range tests {
		series := version.MacOSXSeriesFromMajorVersion(test.version)
		c.Check(series, gc.Equals, test.series)
	}
}
