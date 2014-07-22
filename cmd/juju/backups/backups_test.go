// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	gc "launchpad.net/gocheck"

	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/testing"
)

const ExpectedHelp = `
usage: juju backups [options] <command> ...
purpose: create, manage, and restore backups of juju's state

options:
--description  (= false)
    
-h, --help  (= false)
    show help on a command or other topic

"juju backups" is used to manage backups of the state of a juju environment.

commands:
    help - show help on a command or other topic
`

type BackupCommandSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&BackupCommandSuite{})

func (s *BackupCommandSuite) TestBackupsHelp(c *gc.C) {

	// Run the command, ensuring it is actually there.
	args := []string{"juju", "backups", "--help"}
	out := cmdtesting.BadRun(c, 0, args...)

	c.Check(out, gc.Equals, ExpectedHelp[1:])
}
