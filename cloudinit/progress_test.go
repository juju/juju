// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"regexp"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cloudinit"
)

type progressSuite struct{}

var _ = gc.Suite(&progressSuite{})

func (*progressSuite) TestProgressCmds(c *gc.C) {
	initCmd := cloudinit.InitProgressCmd()
	re := regexp.MustCompile(`test -e /proc/self/fd/([0-9]+) \|\| exec ([0-9]+)>&2`)
	submatch := re.FindStringSubmatch(initCmd)
	c.Assert(submatch, gc.HasLen, 3)
	c.Assert(submatch[0], gc.Equals, initCmd)
	c.Assert(submatch[1], gc.Equals, submatch[2])
	logCmd := cloudinit.LogProgressCmd("he'llo\"!")
	c.Assert(logCmd, gc.Equals, `echo 'he'"'"'llo"!' >&`+submatch[1])
}
