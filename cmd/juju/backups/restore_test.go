// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type restoreSuite struct {
	BaseBackupsSuite
	subcommand *backups.RestoreCommand
}

var _ = gc.Suite(&restoreSuite{})

func (s *restoreSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = &backups.RestoreCommand{}
}

func (s *restoreSuite) TestRestoreArgs(c *gc.C) {
	_, err := testing.RunCommand(c, s.command, "restore")
	c.Assert(err, gc.ErrorMatches, "you must specify either a file or a backup id.")

	_, err = testing.RunCommand(c, s.command, "restore", "--id", "anid", "--file", "afile")
	c.Assert(err, gc.ErrorMatches, "you must specify either a file or a backup id but not both.")

	_, err = testing.RunCommand(c, s.command, "restore", "--id", "anid", "-b")
	c.Assert(err, gc.ErrorMatches, "it is not possible to rebootstrap and restore from an id.")
}
