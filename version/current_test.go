// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"os/exec"
	"runtime"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/version"
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
		default:
			c.Assert(s, gc.Equals, "n/a")
		}
	} else {
		c.Assert(string(out), gc.Equals, "Codename:\t"+s+"\n")
	}
}
