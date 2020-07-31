// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type k8sAgentSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&k8sAgentSuite{})

type mainWrapperTC struct {
	args []string
	code int
}

func (s *k8sAgentSuite) TestMainWrapper(c *gc.C) {
	factory := commandFactotry{
		k8sAgentCmd: func(ctx *cmd.Context, args []string) int {
			return 11
		},
		jujuRun: func(ctx *cmd.Context, args []string) int {
			return 12
		},
		jujuIntrospect: func(ctx *cmd.Context, args []string) int {
			return 14
		},
	}
	for _, tc := range []mainWrapperTC{
		{args: []string{"k8sagent"}, code: 11},
		{args: []string{"juju-run"}, code: 12},
		{args: []string{"juju-introspect"}, code: 14},
	} {
		c.Check(mainWrapper(factory, tc.args), gc.DeepEquals, tc.code)
	}
}

func (s *k8sAgentSuite) TestRegisteredSubCommandsForK8sAgentCommand(c *gc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	k8sagentCmd, err := k8sAgentCommand(ctx)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err = cmdtesting.RunCommand(c, k8sagentCmd, []string{"help", "commands"}...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
help  Show help on a command or other topic.
init  initialize k8sagent state
unit  starting a k8s agent
`[1:])
}
