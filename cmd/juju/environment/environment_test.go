// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/environment"
	// Bring in the dummy provider definition.
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type EnvironmentCommandSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&EnvironmentCommandSuite{})

var expectedCommmandNames = []string{
	"get",
	"get-constraints",
	"help",
	"jenv",
	"retry-provisioning",
	"set",
	"set-constraints",
	"unset",
}

func (s *EnvironmentCommandSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := testing.RunCommand(c, environment.NewSuperCommand(), "--help")
	c.Assert(err, jc.ErrorIsNil)
	namesFound := testing.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, gc.DeepEquals, expectedCommmandNames)
}
