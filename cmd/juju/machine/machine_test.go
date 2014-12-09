// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	_ "os"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/machine"
	_ "github.com/juju/juju/environs/configstore"
	_ "github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"

	// Bring in the dummy provider definition.
	_ "github.com/juju/juju/provider/dummy"
)

type MachineCommandSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&MachineCommandSuite{})

var expectedCommmandNames = []string{
	"add",
	"help",
	"remove",
}

func (s *MachineCommandSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := testing.RunCommand(c, machine.NewSuperCommand(), "--help")
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
