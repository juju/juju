// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"strings"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type removeSuite struct {
	BaseBackupsSuite
	subcommand *backups.RemoveCommand
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = &backups.RemoveCommand{}
}

func (s *removeSuite) TestHelp(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "remove", "--help")
	c.Assert(err, jc.ErrorIsNil)

	info := s.subcommand.Info()
	expected := "(?sm)usage: juju backups remove [options] " + info.Args + "$.*"
	expected = strings.Replace(expected, "[", `\[`, -1)
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + info.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + info.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
}

func (s *removeSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	s.subcommand.ID = "spam"
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	out := "successfully removed: spam\n"
	s.checkStd(c, ctx, out, "")
}

func (s *removeSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
