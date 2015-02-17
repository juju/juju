// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/machine"
	// Bring in the dummy provider definition.
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
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
	namesFound := testing.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, gc.DeepEquals, expectedCommmandNames)
}
