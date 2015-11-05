// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/juju/osenv"
	// Bring in the dummy provider definition.
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type EnvironmentCommandSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&EnvironmentCommandSuite{})

var expectedCommmandNames = []string{
	"destroy",
	"get",
	"get-constraints",
	"help",
	"jenv",
	"retry-provisioning",
	"set",
	"set-constraints",
	"share",
	"unset",
	"unshare",
	"users",
}

func (s *EnvironmentCommandSuite) TestHelpCommands(c *gc.C) {
	defer osenv.SetJujuHome(osenv.SetJujuHome(c.MkDir()))

	// Check that we have correctly registered all the commands
	// by checking the help output.
	c.Assert(getHelpCommandNames(c), jc.SameContents, expectedCommmandNames)
}

func getHelpCommandNames(c *gc.C) []string {
	ctx, err := testing.RunCommand(c, environment.NewSuperCommand(), "--help")
	c.Assert(err, jc.ErrorIsNil)
	return testing.ExtractCommandsFromHelpOutput(ctx)
}
