// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

// MachineSuite tests the connectivity of all the machine subcommands. These
// tests go from the command line, api client, api server, db. The db changes
// are then checked.  Only one test for each command is done here to check
// connectivity.  Exhaustive unit tests are at each layer.
type MachineSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&MachineSuite{})

func (s *MachineSuite) RunCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	context := cmdtesting.Context(c)
	juju := NewJujuCommand(context, "")
	if err := cmdtesting.InitCommand(juju, args); err != nil {
		return context, err
	}
	return context, juju.Run(context)
}

func (s *MachineSuite) TestMachineAdd(c *gc.C) {
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	count := len(machines)

	ctx, err := s.RunCommand(c, "add-machine")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `created machine`)

	machines, err = s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, count+1)
}

func (s *MachineSuite) TestMachineRemove(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)

	ctx, err := s.RunCommand(c, "remove-machine", machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")

	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(machine.Life(), gc.Equals, state.Dying)
}
