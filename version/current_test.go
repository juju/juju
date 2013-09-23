// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"io/ioutil"
	"os/exec"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/version"
)

type CurrentSuite struct{}

var _ = gc.Suite(&CurrentSuite{})

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

func (*CurrentSuite) TestReadSeries(c *gc.C) {
	d := c.MkDir()
	f := filepath.Join(d, "foo")
	for i, t := range readSeriesTests {
		c.Logf("test %d", i)
		err := ioutil.WriteFile(f, []byte(t.contents), 0666)
		c.Assert(err, gc.IsNil)
		c.Assert(version.ReadSeries(f), gc.Equals, t.series)
	}
}

func (*CurrentSuite) TestCurrentSeries(c *gc.C) {
	s := version.Current.Series
	if s == "unknown" {
		s = "n/a"
	}
	out, err := exec.Command("lsb_release", "-c").CombinedOutput()
	if err != nil {
		// If the command fails (for instance if we're running on some other
		// platform) then CurrentSeries should be unknown.
		c.Assert(s, gc.Equals, "n/a")
	} else {
		c.Assert(string(out), gc.Equals, "Codename:\t"+s+"\n")
	}
}
