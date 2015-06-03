// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/testing"

	// Bring in the dummy provider definition.
	_ "github.com/juju/juju/provider/dummy"
)

type SystemCommandSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&SystemCommandSuite{})

var expectedCommmandNames = []string{
	"create-environment",
	"environments",
	"help",
	"list",
	"login",
	"use-env", // alias for use-environment
	"use-environment",
}

func (s *SystemCommandSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := testing.RunCommand(c, system.NewSuperCommand(), "--help")
	c.Assert(err, jc.ErrorIsNil)
	namesFound := testing.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, gc.DeepEquals, expectedCommmandNames)
}
