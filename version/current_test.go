// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"os/exec"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	osbase "github.com/juju/juju/core/os/base"
)

type CurrentSuite struct{}

var _ = gc.Suite(&CurrentSuite{})

func (*CurrentSuite) TestCurrentSeries(c *gc.C) {
	b, err := osbase.HostBase()
	if err != nil {
		c.Fatal(err)
	}
	out, err := exec.Command("lsb_release", "-r").CombinedOutput()

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, "Release:\t"+b.Channel.Track+"\n")
}
