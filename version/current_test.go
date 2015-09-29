// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"os/exec"
	"runtime"

	"github.com/juju/utils/os"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"
)

type CurrentSuite struct{}

var _ = gc.Suite(&CurrentSuite{})

func (*CurrentSuite) TestCurrentSeries(c *gc.C) {
	s := series.HostSeries()
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
			current_os, err := series.GetOSFromSeries(s)
			c.Assert(err, gc.IsNil)
			if s != "n/a" {
				// There is no lsb_release command on CentOS.
				if current_os == os.CentOS {
					c.Check(s, gc.Matches, `centos7`)
				}
			}
		}
	} else {
		c.Assert(string(out), gc.Equals, "Codename:\t"+s+"\n")
	}
}
