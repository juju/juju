// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/cloudconfig/cloudinit"
)

type progressSuite struct{}

var _ = tc.Suite(&progressSuite{})

func (*progressSuite) TestProgressCmds(c *tc.C) {
	initCmd := cloudinit.InitProgressCmd()
	c.Assert(initCmd, tc.Equals,
		`test -n "$JUJU_PROGRESS_FD" || `+
			`(exec {JUJU_PROGRESS_FD}>&2) 2>/dev/null && exec {JUJU_PROGRESS_FD}>&2 || `+
			`JUJU_PROGRESS_FD=2`)
	logCmd := cloudinit.LogProgressCmd("he'llo\"!")
	c.Assert(logCmd, tc.Equals, `echo 'he'"'"'llo"!' >&$JUJU_PROGRESS_FD`)
}
