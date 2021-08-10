// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
)

type listSuite struct {
	BaseBackupsSuite
	subcommand cmd.Command
}

var _ = gc.Suite(&listSuite{})

func (s *listSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = backups.NewListCommandForTest(s.store)
}

func (s *listSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	s.subcommand = s.createCommandForGlobalOptionTesting(s.subcommand)
	ctx, err := cmdtesting.RunCommand(c, s.subcommand, "backups", "--verbose")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmdtesting.Stderr(ctx), gc.Equals, MetaResultString[:len(MetaResultString)-1])
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *listSuite) TestBrief(c *gc.C) {
	s.setSuccess()
	ctx, err := cmdtesting.RunCommand(c, s.subcommand)
	c.Assert(err, jc.ErrorIsNil)
	out := s.metaresult.ID + "\n"
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, out)
}

func (s *listSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	_, err := cmdtesting.RunCommand(c, s.subcommand)
	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
