// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/testing"

	// Bring in the dummy provider definition.
	_ "github.com/juju/juju/provider/dummy"
)

type EnvironmentCommandSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&EnvironmentCommandSuite{})

var expectedCommmandNames = []string{
	"get",
	"help",
	"set",
	"unset",
}

func (s *EnvironmentCommandSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := testing.RunCommand(c, environment.NewSuperCommand(), "--help")
	c.Assert(err, jc.ErrorIsNil)

	// Check that we have registered all the sub commands by
	// inspecting the help output.
	var namesFound []string
	commandHelp := strings.SplitAfter(testing.Stdout(ctx), "commands:")[1]
	commandHelp = strings.TrimSpace(commandHelp)
	for _, line := range strings.Split(commandHelp, "\n") {
		namesFound = append(namesFound, strings.TrimSpace(strings.Split(line, " - ")[0]))
	}
	c.Assert(namesFound, gc.DeepEquals, expectedCommmandNames)
}
