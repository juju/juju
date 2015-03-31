// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

var subcommandNames = []string{
	"create",
	"remove",
	"update",
	"help",
}

type SpaceCommandSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&SpaceCommandSuite{})

func (s *SpaceCommandSuite) TestHelpSubcommands(c *gc.C) {
	ctx, err := coretesting.RunCommand(c, s.superCmd, "--help")
	c.Assert(err, jc.ErrorIsNil)

	namesFound := coretesting.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, jc.SameContents, subcommandNames)
}
