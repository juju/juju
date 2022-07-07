// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type containerAgentSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&containerAgentSuite{})

type mainWrapperTC struct {
	args []string
	code int
}

func (s *containerAgentSuite) TestMainWrapper(c *gc.C) {
	factory := commandFactory{
		containerAgentCmd: func(ctx *cmd.Context, args []string) int {
			return 11
		},
		jujuExec: func(ctx *cmd.Context, args []string) int {
			return 12
		},
		jujuIntrospect: func(ctx *cmd.Context, args []string) int {
			return 14
		},
	}
	for _, tc := range []mainWrapperTC{
		{args: []string{"containeragent"}, code: 11},
		{args: []string{"juju-exec"}, code: 12},
		{args: []string{"juju-introspect"}, code: 14},
	} {
		c.Check(mainWrapper(factory, tc.args), gc.DeepEquals, tc.code)
	}
}

func (s *containerAgentSuite) TestRegisteredSubCommandsForContainerAgentCommand(c *gc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	containerAgentCmd, err := containerAgentCommand(ctx)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err = cmdtesting.RunCommand(c, containerAgentCmd, []string{"help", "commands"}...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
help  Show help on a command or other topic.
init  Initialize containeragent local state.
unit  Start containeragent.
`[1:])
}
