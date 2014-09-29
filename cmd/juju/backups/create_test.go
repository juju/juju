// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"strings"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type createSuite struct {
	BaseBackupsSuite
	subcommand *backups.CreateCommand
}

var _ = gc.Suite(&createSuite{})

func (s *createSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = &backups.CreateCommand{}
}

func (s *createSuite) TestHelp(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "create", "--help")
	c.Assert(err, gc.IsNil)

	info := s.subcommand.Info()
	expected := "(?sm)usage: juju backups create [options] " + info.Args + "$.*"
	expected = strings.Replace(expected, "[", `\[`, -1)
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + info.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + info.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
}

func (s *createSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Check(err, gc.IsNil)

	out := MetaResultString + s.metaresult.ID + "\n"
	s.checkStd(c, ctx, out, "")
}

func (s *createSuite) TestQuiet(c *gc.C) {
	s.setSuccess()
	s.subcommand.Quiet = true
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Check(err, gc.IsNil)

	out := s.metaresult.ID + "\n"
	s.checkStd(c, ctx, out, "")
}

func (s *createSuite) TestNotes(c *gc.C) {
	client := s.setSuccess()
	_, err := testing.RunCommand(c, s.command, "create", "spam")
	c.Assert(err, gc.IsNil)

	c.Check(client.args, gc.DeepEquals, []string{"notes"})
	c.Check(client.notes, gc.Equals, "spam")
}

func (s *createSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
