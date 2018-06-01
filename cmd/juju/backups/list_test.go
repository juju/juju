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
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type listSuite struct {
	BaseBackupsSuite
	subcommand cmd.Command
}

var _ = gc.Suite(&listSuite{})

func (s *listSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = backups.NewListCommandForTest(jujuclienttesting.MinimalStore())
}

func (s *listSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	ctx, err := cmdtesting.RunCommand(c, s.subcommand, []string{"--verbose"}...)
	c.Assert(err, jc.ErrorIsNil)

	out := MetaResultString
	s.checkStd(c, ctx, out, "")
}

func (s *listSuite) TestBrief(c *gc.C) {
	s.setSuccess()
	ctx, err := cmdtesting.RunCommand(c, s.subcommand)
	c.Assert(err, jc.ErrorIsNil)
	out := s.metaresult.ID + "\n"
	s.checkStd(c, ctx, out, "")
}

func (s *listSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	_, err := cmdtesting.RunCommand(c, s.subcommand)
	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
