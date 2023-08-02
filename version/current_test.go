// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"os/exec"
	"runtime"

	osseries "github.com/juju/os/v2/series"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/os"
)

type CurrentSuite struct{}

var _ = gc.Suite(&CurrentSuite{})

func (*CurrentSuite) TestCurrentSeries(c *gc.C) {
	s, err := osseries.HostSeries()
	if err != nil || s == "unknown" {
		s = "n/a"
	}
	out, err := exec.Command("lsb_release", "-c").CombinedOutput()

	if err != nil {
		// If the command fails (for instance if we're running on some other
		// platform) then CurrentSeries should be unknown.
		switch runtime.GOOS {
		case "darwin":
			c.Check(s, gc.Matches, `mavericks|mountainlion|lion|snowleopard`)
		default:
			currentOS, err := corebase.GetOSFromSeries(s)
			c.Assert(err, gc.IsNil)
			if s != "n/a" {
				// There is no lsb_release command on CentOS.
				if currentOS == os.CentOS {
					c.Check(s, gc.Matches, `centos\d+`)
				}
			}
		}
	} else {
		//OpenSUSE lsb-release returns n/a
		currentOS, err := corebase.GetOSFromSeries(s)
		c.Assert(err, gc.IsNil)
		if string(out) == "n/a" && currentOS == os.OpenSUSE {
			c.Check(s, gc.Matches, "opensuseleap")
		} else {
			c.Assert(string(out), gc.Equals, "Codename:\t"+s+"\n")
		}
	}
}
