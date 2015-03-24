// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type recoverSuite struct {
	BaseBackupsSuite
	subcommand *backups.RecoverCommand
}

var _ = gc.Suite(&recoverSuite{})

func (s *recoverSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = &backups.RecoverCommand{}
}

func (s *recoverSuite) TestRecoverArgs(c *gc.C) {
	_, err := testing.RunCommand(c, s.command, "recover")
	c.Assert(err, gc.ErrorMatches, "you must specify either a file or a backup id.")

	_, err = testing.RunCommand(c, s.command, "recover", "--id", "anid", "--file", "afile")
	c.Assert(err, gc.ErrorMatches, "you must specify either a file or a backup id but not both.")

	_, err = testing.RunCommand(c, s.command, "recover", "--id", "anid", "-b")
	c.Assert(err, gc.ErrorMatches, "it is not possible to rebootstrap and recover from an id.")
}
