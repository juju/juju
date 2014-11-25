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

type infoSuite struct {
	BaseBackupsSuite
	subcommand *backups.InfoCommand
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = &backups.InfoCommand{}
}

func (s *infoSuite) TestHelp(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "info", "--help")
	c.Assert(err, jc.ErrorIsNil)

	info := s.subcommand.Info()
	expected := "(?sm)usage: juju backups info [options] " + info.Args + "$.*"
	expected = strings.Replace(expected, "[", `\[`, -1)
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + info.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + info.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
}

func (s *infoSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	out := MetaResultString
	s.checkStd(c, ctx, out, "")
}

func (s *infoSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
