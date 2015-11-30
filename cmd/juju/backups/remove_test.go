// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type removeSuite struct {
	BaseBackupsSuite
	command cmd.Command
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.command = backups.NewRemoveCommand()
}

func (s *removeSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, s.command)
}

func (s *removeSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	ctx, err := testing.RunCommand(c, s.command, "spam")
	c.Check(err, jc.ErrorIsNil)

	out := "successfully removed: spam\n"
	s.checkStd(c, ctx, out, "")
}

func (s *removeSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	_, err := testing.RunCommand(c, s.command, "spam")
	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
