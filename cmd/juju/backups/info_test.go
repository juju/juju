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

type infoSuite struct {
	BaseBackupsSuite
	subcommand cmd.Command
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = backups.NewInfoCommand()
}

func (s *infoSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, s.subcommand)
}

func (s *infoSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	ctx, err := testing.RunCommand(c, s.subcommand, s.metaresult.ID)
	c.Check(err, jc.ErrorIsNil)

	out := MetaResultString
	s.checkStd(c, ctx, out, "")
}

func (s *infoSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	_, err := testing.RunCommand(c, s.subcommand, s.metaresult.ID)
	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
