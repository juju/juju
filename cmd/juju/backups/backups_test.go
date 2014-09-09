// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	gc "launchpad.net/gocheck"
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
    create - create a backup
    help   - show help on a command or other topic
`

type backupsSuite struct {
	BackupsSuite
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, "", ExpectedHelp[1:])
}
