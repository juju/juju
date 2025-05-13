// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"os/exec"

	"github.com/juju/tc"

	coreos "github.com/juju/juju/core/os"
)

type CurrentSuite struct{}

var _ = tc.Suite(&CurrentSuite{})

func (*CurrentSuite) TestCurrentSeries(c *tc.C) {
	b, err := coreos.HostBase()
	if err != nil {
		c.Fatal(err)
	}
	out, err := exec.Command("lsb_release", "-r").CombinedOutput()

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, "Release:\t"+b.Channel.Track+"\n")
}
