// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type kernelVersionSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&kernelVersionSuite{})

func sysctlMacOS10dot9dot2() (string, error) {
	// My 10.9.2 Mac gives "13.1.0" as the kernel version
	return "13.1.0", nil
}

func sysctlError() (string, error) {
	return "", fmt.Errorf("no such syscall")
}

func (*kernelVersionSuite) TestKernelToMajorVersion(c *gc.C) {
	majorVersion, err := version.KernelToMajor(sysctlMacOS10dot9dot2)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
		{version: 14, series: "yosemite"},
		{version: 15, series: "elcapitan"},
		{version: 16, series: "unknown", err: `unknown series ""`},
		{version: 4, series: "unknown", err: `unknown series ""`},
		{version: 0, series: "unknown", err: `unknown series ""`},
	}
	for _, test := range tests {
		series, err := version.MacOSXSeriesFromMajorVersion(test.version)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
		c.Check(series, gc.Equals, test.series)
	}
}
