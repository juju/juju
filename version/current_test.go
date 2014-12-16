// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"os/exec"
	"runtime"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/version"
)

type CurrentSuite struct{}

var _ = gc.Suite(&CurrentSuite{})

func (*CurrentSuite) TestCurrentSeries(c *gc.C) {
	s := version.Current.Series
	if s == "unknown" {
		s = "n/a"
	}
	out, err := exec.Command("lsb_release", "-c").CombinedOutput()
	if err != nil {
		// If the command fails (for instance if we're running on some other
		// platform) then CurrentSeries should be unknown.
		switch runtime.GOOS {
		case "darwin":
			c.Check(s, gc.Matches, `mavericks|mountainlion|lion|snowleopard`)
		case "windows":
			c.Check(s, gc.Matches, `win2012hvr2|win2012hv|win2012|win2012r2|win8|win81|win7`)
		default:
			c.Assert(s, gc.Equals, "n/a")
		}
	} else {
		os, err := version.GetOSFromSeries(s)
		c.Assert(err, gc.IsNil)
		// There is no lsb_release command on CentOS.
		switch os {
		case version.CentOS:
			c.Check(s, gc.Matches, `centos7`)
		default:
			c.Assert(string(out), gc.Equals, "Codename:\t"+s+"\n")
		}
	}
}
