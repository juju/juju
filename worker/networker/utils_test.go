// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	"runtime"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/networker"
)

type utilsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&utilsSuite{})

func (s *utilsSuite) TestExecuteCommands(c *gc.C) {
	//TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: test uses bash scripts, will fix later on windows")
	}
	commands := []string{
		"echo start",
		"sh -c 'echo STDOUT; echo STDERR >&2; exit 123'",
		"echo end",
		"exit 111",
	}
	err := networker.ExecuteCommands(commands)
	expected := "command \"sh -c 'echo STDOUT; echo STDERR >&2; exit 123'\" failed " +
		"(code: 123, stdout: STDOUT\n, stderr: STDERR\n)"
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, expected)
}
