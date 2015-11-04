// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/controller"
	// Bring in the dummy provider definition.
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type ControllerCommandSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ControllerCommandSuite{})

var expectedCommmandNames = []string{
	"create-env", // alias for create-environment
	"create-environment",
	"destroy",
	"environments",
	"help",
	"kill",
	"list",
	"list-blocks",
	"login",
	"remove-blocks",
	"use-env", // alias for use-environment
	"use-environment",
}

func (s *ControllerCommandSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := testing.RunCommand(c, controller.NewSuperCommand(), "--help")
	c.Assert(err, jc.ErrorIsNil)
	namesFound := testing.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, gc.DeepEquals, expectedCommmandNames)
}
