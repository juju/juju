// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
)

type removeSuite struct {
	BaseBackupsSuite
	command cmd.Command
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.command = backups.NewRemoveCommandForTest()
}

func (s *removeSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	ctx, err := cmdtesting.RunCommand(c, s.command, "spam")
	c.Check(err, jc.ErrorIsNil)

	out := "successfully removed: spam\n"
	s.checkStd(c, ctx, out, "")
}

func (s *removeSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	_, err := cmdtesting.RunCommand(c, s.command, "spam")
	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
