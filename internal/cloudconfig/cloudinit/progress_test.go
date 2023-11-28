// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/cloudconfig/cloudinit"
)

type progressSuite struct{}

var _ = gc.Suite(&progressSuite{})

func (*progressSuite) TestProgressCmds(c *gc.C) {
	initCmd := cloudinit.InitProgressCmd()
	c.Assert(initCmd, gc.Equals,
		`test -n "$JUJU_PROGRESS_FD" || `+
			`(exec {JUJU_PROGRESS_FD}>&2) 2>/dev/null && exec {JUJU_PROGRESS_FD}>&2 || `+
			`JUJU_PROGRESS_FD=2`)
	logCmd := cloudinit.LogProgressCmd("he'llo\"!")
	c.Assert(logCmd, gc.Equals, `echo 'he'"'"'llo"!' >&$JUJU_PROGRESS_FD`)
}
